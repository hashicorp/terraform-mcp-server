// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/zclconf/go-cty/cty"
)

const (
	defaultImportResultLimit = 20
	maxImportResultLimit     = 100
	maxModuleContextBytes    = 512 * 1024
	maxProviderDocBytes      = 32 * 1024
	importPlanPollInterval   = 3 * time.Second
	importPlanTimeout        = 10 * time.Minute
)

type importCandidate struct {
	Address        string         `json:"address"`
	DisplayName    string         `json:"display_name"`
	ResourceType   string         `json:"resource_type"`
	Identity       map[string]any `json:"identity"`
	ResourceObject map[string]any `json:"resource_object"`
	Configuration  string         `json:"configuration"`
	ImportConfig   string         `json:"import_configuration"`
}

type importQueryLogRecord struct {
	Type              string           `json:"type"`
	ListResourceFound *importCandidate `json:"list_resource_found"`
}

type workspaceProvider struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type providerDocumentation struct {
	ProviderSource  string `json:"provider_source"`
	ProviderVersion string `json:"provider_version"`
	ResourceType    string `json:"resource_type"`
	Content         string `json:"content,omitempty"`
	Error           string `json:"error,omitempty"`
}

type moduleFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type moduleLayout struct {
	UploadRoot string
	ModuleDir  string
	Warning    string
}

type importBinding struct {
	Target   string
	Identity cty.Value
}

type terraformConfigurationShape struct {
	Resources map[string]struct{}
	Imports   []importBinding
}

// ImportQueryResults creates a two-phase tool for preparing and verifying bulk imports.
func ImportQueryResults(logger *log.Logger, mcpServer *server.MCPServer) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("import_query_results",
			mcp.WithDescription(importQueryResultsDescription),
			mcp.WithTitleAnnotation("Prepare and verify Terraform query imports"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("phase",
				mcp.Required(),
				mcp.Enum("prepare", "verify"),
				mcp.Description("Use prepare first. Use verify only after shaping the returned generated HCL to the target module."),
			),
			mcp.WithString("organization_name", mcp.Required(), mcp.Description("HCP Terraform organization name.")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("HCP Terraform workspace that will own the imported resources.")),
			mcp.WithString("query_run_id", mcp.Required(), mcp.Description("Finished query run whose discovered resources should be imported.")),
			mcp.WithString("configuration_path", mcp.Description("Absolute local path to the workspace configuration upload root. When omitted, the tool asks the user.")),
			mcp.WithString("query_configuration", mcp.Description("Exact query_configuration JSON previously passed to execute_query. When provided, its provider versions are used instead of requiring Explorer metadata from the target workspace.")),
			mcp.WithNumber("max_resources", mcp.Min(1), mcp.Max(maxImportResultLimit), mcp.Description("Maximum query results to prepare. Defaults to 20.")),
			mcp.WithString("generated_configuration", mcp.Description("For verify: shaped HCL containing resource and import blocks for the selected results.")),
			mcp.WithString("output_file", mcp.Description("For verify: new .tf filename in the target module. Defaults to imports.generated.tf.")),
			mcp.WithBoolean("confirm_speculative_run", mcp.Description("For verify: must be true to write the file, upload a provisional configuration, and create a plan-only run."), mcp.DefaultBool(false)),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return importQueryResultsHandler(ctx, request, logger, mcpServer)
		},
	}
}

func importQueryResultsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger, mcpServer *server.MCPServer) (*mcp.CallToolResult, error) {
	phase := strings.TrimSpace(request.GetString("phase", ""))
	organizationName := strings.TrimSpace(request.GetString("organization_name", ""))
	workspaceName := strings.TrimSpace(request.GetString("workspace_name", ""))
	queryRunID := strings.TrimSpace(request.GetString("query_run_id", ""))
	if phase == "" || organizationName == "" || workspaceName == "" || queryRunID == "" {
		return importQueryToolErrorf(logger, "phase, organization_name, workspace_name, and query_run_id are required")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return importQueryToolErrorf(logger, "failed to get Terraform client: %v", err)
	}
	workspace, err := tfeClient.Workspaces.Read(ctx, organizationName, workspaceName)
	if err != nil {
		return importQueryToolErrorf(logger, "workspace %q not found in organization %q: %v", workspaceName, organizationName, err)
	}

	configurationPath := strings.TrimSpace(request.GetString("configuration_path", ""))
	if configurationPath == "" {
		configurationPath, err = requestConfigurationPath(ctx, mcpServer, organizationName, workspaceName)
		if err != nil {
			return importQueryToolErrorf(logger, "%v", err)
		}
	}
	layout, err := resolveModuleLayout(configurationPath, workspace.WorkingDirectory)
	if err != nil {
		return importQueryToolErrorf(logger, "invalid configuration_path: %v", err)
	}

	queryRun, err := tfeClient.QueryRuns.Read(ctx, queryRunID)
	if err != nil {
		return importQueryToolErrorf(logger, "failed to read query run %q: %v", queryRunID, err)
	}
	if queryRun.Status != tfe.QueryRunFinished {
		return importQueryToolErrorf(logger, "query run %q must be finished; current status is %q", queryRunID, queryRun.Status)
	}
	if err := validateQueryRunWorkspace(queryRun, workspace); err != nil {
		return importQueryToolErrorf(logger, "query run %q: %v", queryRunID, err)
	}

	limit := request.GetInt("max_resources", defaultImportResultLimit)
	if limit < 1 || limit > maxImportResultLimit {
		return importQueryToolErrorf(logger, "max_resources must be between 1 and %d", maxImportResultLimit)
	}
	candidates, err := readImportCandidates(ctx, tfeClient, queryRunID, limit)
	if err != nil {
		return importQueryToolErrorf(logger, "failed to read import candidates: %v", err)
	}
	if len(candidates) == 0 {
		return importQueryToolErrorf(logger, "query run %q has no generated import candidates; execute the query with generate_config_out=true", queryRunID)
	}

	if phase == "verify" {
		if !request.GetBool("confirm_speculative_run", false) {
			return importQueryToolErrorf(logger, "verify requires confirm_speculative_run=true before requesting user confirmation")
		}
		confirmed, err := requestRunConfirmation(ctx, mcpServer, workspaceName, len(candidates))
		if err != nil {
			return importQueryToolErrorf(logger, "%v", err)
		}
		if !confirmed {
			return importQueryToolErrorf(logger, "speculative import run was not confirmed")
		}
		return verifyQueryImport(ctx, request, tfeClient, workspace, layout, candidates, logger)
	}
	if phase != "prepare" {
		return importQueryToolErrorf(logger, "phase must be prepare or verify")
	}

	moduleContext, err := readModuleContext(layout.ModuleDir)
	if err != nil {
		return importQueryToolErrorf(logger, "failed to read target module: %v", err)
	}
	providers, err := providersFromQueryConfiguration(request.GetString("query_configuration", ""))
	if err != nil {
		return importQueryToolErrorf(logger, "invalid query_configuration: %v", err)
	}
	providerSource := "query_configuration"
	if len(providers) == 0 {
		providers, err = readWorkspaceProviders(ctx, tfeClient, organizationName, workspaceName)
		if err != nil {
			return importQueryToolErrorf(logger, "failed to discover exact workspace provider versions: %v", err)
		}
		providerSource = "explorer"
	}
	if len(providers) == 0 {
		return importQueryToolErrorf(logger, "Explorer returned no providers for workspace %q; retry with the exact query_configuration passed to execute_query", workspaceName)
	}
	documentation := readProviderDocumentation(ctx, candidates, providers, logger)
	for _, doc := range documentation {
		if doc.Error != "" {
			return importQueryToolErrorf(logger, "failed to fetch provider documentation for %s: %s", doc.ResourceType, doc.Error)
		}
	}

	response := map[string]any{
		"phase":                       "prepare",
		"workspace_id":                workspace.ID,
		"workspace_working_directory": workspace.WorkingDirectory,
		"upload_root":                 layout.UploadRoot,
		"target_module":               layout.ModuleDir,
		"module_context":              moduleContext,
		"provider_versions":           providers,
		"provider_version_source":     providerSource,
		"provider_documentation":      documentation,
		"import_candidates":           candidates,
		"next_step":                   "Shape only these candidates into one HCL document. Keep required provider attributes, remove computed-only values, and prefer existing var.*, local.*, module.*, data.*, and resource references from module_context over repeated literals. Do not reference symbols from sibling or parent modules. Then call this tool with phase=verify, generated_configuration, and confirm_speculative_run=true.",
	}
	if layout.Warning != "" {
		response["configuration_path_warning"] = layout.Warning
	}
	return importQueryJSONResult(response, logger)
}

func validateQueryRunWorkspace(queryRun *tfe.QueryRun, workspace *tfe.Workspace) error {
	if queryRun.Workspace == nil || queryRun.Workspace.ID == "" {
		return fmt.Errorf("did not identify its workspace; refusing to import without proving workspace ownership")
	}
	if queryRun.Workspace.ID != workspace.ID {
		return fmt.Errorf("belongs to a different workspace")
	}
	return nil
}

func requestConfigurationPath(ctx context.Context, mcpServer *server.MCPServer, organizationName, workspaceName string) (string, error) {
	result, err := mcpServer.RequestElicitation(ctx, mcp.ElicitationRequest{Params: mcp.ElicitationParams{
		Message: fmt.Sprintf("Where does the local configuration for HCP Terraform workspace %s/%s live? Provide the absolute path to the configuration upload root.", organizationName, workspaceName),
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"configuration_path": map[string]any{"type": "string", "title": "Configuration path"},
			},
			"required": []string{"configuration_path"},
		},
	}})
	if err != nil {
		return "", fmt.Errorf("failed to ask for configuration_path: %w", err)
	}
	if result.Action != mcp.ElicitationResponseActionAccept {
		return "", fmt.Errorf("configuration path request was %s", result.Action)
	}
	content, ok := result.Content.(map[string]any)
	if !ok {
		return "", fmt.Errorf("configuration path response is invalid")
	}
	path, _ := content["configuration_path"].(string)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("configuration_path cannot be empty")
	}
	return strings.TrimSpace(path), nil
}

func requestRunConfirmation(ctx context.Context, mcpServer *server.MCPServer, workspaceName string, candidateCount int) (bool, error) {
	result, err := mcpServer.RequestElicitation(ctx, mcp.ElicitationRequest{Params: mcp.ElicitationParams{
		Message: fmt.Sprintf("Create a speculative plan-only run in workspace %s to verify up to %d imports? This writes a new local .tf file and uploads a provisional configuration, but cannot apply infrastructure changes.", workspaceName, candidateCount),
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm": map[string]any{"type": "boolean", "title": "Create speculative run"},
			},
			"required": []string{"confirm"},
		},
	}})
	if err != nil {
		return false, fmt.Errorf("failed to request speculative run confirmation: %w", err)
	}
	if result.Action != mcp.ElicitationResponseActionAccept {
		return false, nil
	}
	content, ok := result.Content.(map[string]any)
	if !ok {
		return false, fmt.Errorf("speculative run confirmation response is invalid")
	}
	confirmed, _ := content["confirm"].(bool)
	return confirmed, nil
}

func resolveModuleLayout(configurationPath, workingDirectory string) (moduleLayout, error) {
	absPath, err := filepath.Abs(configurationPath)
	if err != nil {
		return moduleLayout{}, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return moduleLayout{}, err
	}
	if !info.IsDir() {
		return moduleLayout{}, fmt.Errorf("%s is not a directory", absPath)
	}

	layout := moduleLayout{UploadRoot: absPath, ModuleDir: absPath}
	workingDirectory = filepath.Clean(strings.TrimSpace(workingDirectory))
	if workingDirectory == "." || workingDirectory == "" {
		return layout, nil
	}
	if filepath.IsAbs(workingDirectory) || workingDirectory == ".." || strings.HasPrefix(workingDirectory, ".."+string(filepath.Separator)) {
		return moduleLayout{}, fmt.Errorf("workspace working directory %q is invalid", workingDirectory)
	}
	moduleDir := filepath.Join(absPath, workingDirectory)
	if info, err := os.Stat(moduleDir); err == nil && info.IsDir() {
		layout.ModuleDir = moduleDir
		return layout, nil
	}

	layout.Warning = fmt.Sprintf("The workspace working directory is %q, but it does not exist below configuration_path. Preparation used configuration_path as the target module; verification will refuse to upload until configuration_path points to the repository root.", workingDirectory)
	return layout, nil
}

func readImportCandidates(ctx context.Context, tfeClient *tfe.Client, queryRunID string, limit int) ([]importCandidate, error) {
	reader, err := tfeClient.QueryRuns.Logs(ctx, queryRunID)
	if err != nil {
		return nil, err
	}
	return parseImportCandidates(reader, limit)
}

func parseImportCandidates(reader io.Reader, limit int) ([]importCandidate, error) {
	candidates := make([]importCandidate, 0, limit)
	lines := bufio.NewReader(reader)
	for len(candidates) < limit {
		line, err := lines.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.Trim(bytes.TrimSpace(line), "\x02\x03")
			var record importQueryLogRecord
			if json.Unmarshal(line, &record) == nil && record.Type == "list_resource_found" && record.ListResourceFound != nil {
				candidates = append(candidates, *record.ListResourceFound)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

func readModuleContext(moduleDir string) ([]moduleFile, error) {
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	files := make([]moduleFile, 0)
	total := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || (!strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tf.json") && name != ".terraform.lock.hcl") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(moduleDir, name))
		if err != nil {
			return nil, err
		}
		total += len(content)
		if total > maxModuleContextBytes {
			return nil, fmt.Errorf("module configuration exceeds %d bytes", maxModuleContextBytes)
		}
		files = append(files, moduleFile{Name: name, Content: string(content)})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Terraform configuration files found directly in %s", moduleDir)
	}
	return files, nil
}

func readWorkspaceProviders(ctx context.Context, tfeClient *tfe.Client, organizationName, workspaceName string) ([]workspaceProvider, error) {
	path := fmt.Sprintf("/api/v2/organizations/%s/explorer", url.PathEscape(organizationName))
	body, err := utils.MakeCustomGetRequestRaw(ctx, tfeClient, path, map[string][]string{
		"type":                               {"providers"},
		"fields":                             {"source,name,version,workspaces"},
		"page[size]":                         {"100"},
		"filter[0][workspaces][contains][0]": {workspaceName},
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	providers := make([]workspaceProvider, 0, len(response.Data))
	for _, item := range response.Data {
		workspaces, _ := item.Attributes["workspaces"].(string)
		if !commaSeparatedContains(workspaces, workspaceName) {
			continue
		}
		provider := workspaceProvider{
			Source:  stringAttribute(item.Attributes, "source"),
			Name:    stringAttribute(item.Attributes, "name"),
			Version: stringAttribute(item.Attributes, "version"),
		}
		if provider.Source == "" || provider.Name == "" || provider.Version == "" {
			return nil, fmt.Errorf("Explorer returned incomplete provider metadata for workspace %q", workspaceName)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func providersFromQueryConfiguration(raw string) ([]workspaceProvider, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	configuration, err := parseExecuteQueryConfiguration(raw)
	if err != nil {
		return nil, err
	}
	providers := make([]workspaceProvider, 0, len(configuration.Providers))
	for _, provider := range configuration.Providers {
		providers = append(providers, workspaceProvider{
			Source:  fmt.Sprintf("registry.terraform.io/%s/%s", provider.Namespace, provider.Name),
			Name:    provider.Name,
			Version: provider.Version,
		})
	}
	return providers, nil
}

func commaSeparatedContains(value, target string) bool {
	for part := range strings.SplitSeq(value, ",") {
		if strings.TrimSpace(part) == target {
			return true
		}
	}
	return false
}

func stringAttribute(attributes map[string]any, name string) string {
	value, _ := attributes[name].(string)
	return value
}

func readProviderDocumentation(ctx context.Context, candidates []importCandidate, providers []workspaceProvider, logger *log.Logger) []providerDocumentation {
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		return []providerDocumentation{{Error: fmt.Sprintf("failed to get registry client: %v", err)}}
	}
	resourceTypes := make(map[string]struct{})
	for _, candidate := range candidates {
		resourceTypes[candidate.ResourceType] = struct{}{}
	}
	docs := make([]providerDocumentation, 0, len(resourceTypes))
	for resourceType := range resourceTypes {
		provider, ok := providerForResource(resourceType, providers)
		if !ok {
			docs = append(docs, providerDocumentation{ResourceType: resourceType, Error: "could not match resource type to a workspace provider"})
			continue
		}
		doc := providerDocumentation{ProviderSource: provider.Source, ProviderVersion: provider.Version, ResourceType: resourceType}
		doc.Content, err = fetchResourceDocumentation(ctx, httpClient, provider, resourceType, logger)
		if err != nil {
			doc.Error = err.Error()
		}
		docs = append(docs, doc)
	}
	slices.SortFunc(docs, func(a, b providerDocumentation) int { return strings.Compare(a.ResourceType, b.ResourceType) })
	return docs
}

func providerForResource(resourceType string, providers []workspaceProvider) (workspaceProvider, bool) {
	for _, provider := range providers {
		if provider.Name != "" && strings.HasPrefix(resourceType, provider.Name+"_") {
			return provider, true
		}
	}
	return workspaceProvider{}, false
}

func fetchResourceDocumentation(ctx context.Context, httpClient *http.Client, provider workspaceProvider, resourceType string, logger *log.Logger) (string, error) {
	namespace, name, ok := publicProviderParts(provider.Source)
	if !ok {
		return "", fmt.Errorf("provider %q is not a public registry source", provider.Source)
	}
	body, err := client.SendRegistryCall(ctx, httpClient, http.MethodGet, fmt.Sprintf("providers/%s/%s/%s", namespace, name, provider.Version), logger)
	if err != nil {
		return "", err
	}
	var providerDocs client.ProviderDocs
	if err := json.Unmarshal(body, &providerDocs); err != nil {
		return "", err
	}
	for _, candidate := range providerDocs.Docs {
		if candidate.Language != "hcl" || candidate.Category != "resources" {
			continue
		}
		if candidate.Slug != resourceType && name+"_"+candidate.Slug != resourceType {
			continue
		}
		detailBody, err := client.SendRegistryCall(ctx, httpClient, http.MethodGet, "provider-docs/"+candidate.ID, logger, "v2")
		if err != nil {
			return "", err
		}
		var details client.ProviderResourceDetails
		if err := json.Unmarshal(detailBody, &details); err != nil {
			return "", err
		}
		content := details.Data.Attributes.Content
		if len(content) > maxProviderDocBytes {
			content = content[:maxProviderDocBytes] + "\n\n[documentation truncated]"
		}
		return content, nil
	}
	return "", fmt.Errorf("resource documentation not found for %s %s", resourceType, provider.Version)
}

func publicProviderParts(source string) (string, string, bool) {
	parts := strings.Split(strings.Trim(source, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	if strings.Contains(parts[0], ".") && parts[0] != "registry.terraform.io" {
		return "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1], true
}

func verifyQueryImport(ctx context.Context, request mcp.CallToolRequest, tfeClient *tfe.Client, workspace *tfe.Workspace, layout moduleLayout, candidates []importCandidate, logger *log.Logger) (*mcp.CallToolResult, error) {
	if !request.GetBool("confirm_speculative_run", false) {
		return importQueryToolErrorf(logger, "verify requires explicit confirmation with confirm_speculative_run=true")
	}
	if layout.Warning != "" {
		return importQueryToolErrorf(logger, "%s", layout.Warning)
	}
	configuration := strings.TrimSpace(request.GetString("generated_configuration", ""))
	if configuration == "" {
		return importQueryToolErrorf(logger, "generated_configuration is required for verify")
	}
	if err := validateGeneratedConfiguration(configuration, candidates); err != nil {
		return importQueryToolErrorf(logger, "generated_configuration is invalid: %v", err)
	}

	outputFile := strings.TrimSpace(request.GetString("output_file", "imports.generated.tf"))
	if filepath.Base(outputFile) != outputFile || !strings.HasSuffix(outputFile, ".tf") {
		return importQueryToolErrorf(logger, "output_file must be a .tf filename without directory components")
	}
	outputPath := filepath.Join(layout.ModuleDir, outputFile)
	if _, err := os.Stat(outputPath); err == nil {
		return importQueryToolErrorf(logger, "refusing to overwrite existing file %s", outputPath)
	} else if !os.IsNotExist(err) {
		return importQueryToolErrorf(logger, "failed to inspect output file: %v", err)
	}
	if err := os.WriteFile(outputPath, []byte(configuration+"\n"), 0o600); err != nil {
		return importQueryToolErrorf(logger, "failed to write generated configuration: %v", err)
	}

	provisional := true
	speculative := true
	autoQueue := false
	configurationVersion, err := tfeClient.ConfigurationVersions.Create(ctx, workspace.ID, tfe.ConfigurationVersionCreateOptions{
		AutoQueueRuns: &autoQueue,
		Speculative:   &speculative,
		Provisional:   &provisional,
	})
	if err != nil {
		return importQueryToolErrorf(logger, "wrote %s, but failed to create speculative configuration version: %v", outputPath, err)
	}
	if err := tfeClient.ConfigurationVersions.Upload(ctx, configurationVersion.UploadURL, layout.UploadRoot); err != nil {
		return importQueryToolErrorf(logger, "wrote %s, but failed to upload speculative configuration: %v", outputPath, err)
	}

	message := fmt.Sprintf("Verify bulk import from query %s via Terraform MCP Server", strings.TrimSpace(request.GetString("query_run_id", "")))
	run, err := tfeClient.Runs.Create(ctx, tfe.RunCreateOptions{
		Workspace:             workspace,
		ConfigurationVersion:  configurationVersion,
		PlanOnly:              tfe.Bool(true),
		AllowConfigGeneration: tfe.Bool(false),
		Message:               &message,
	})
	if err != nil {
		return importQueryToolErrorf(logger, "wrote %s and uploaded configuration version %s, but failed to create speculative run: %v", outputPath, configurationVersion.ID, err)
	}

	planCtx, cancel := context.WithTimeout(ctx, importPlanTimeout)
	defer cancel()
	plan, err := waitForImportPlan(planCtx, tfeClient, run.ID, importPlanPollInterval)
	if err != nil {
		return importQueryToolErrorf(logger, "speculative run %s did not produce a verifiable plan: %v", run.ID, err)
	}
	if err := validateImportPlan(plan, len(candidates)); err != nil {
		return importQueryToolErrorf(logger, "%v", err)
	}

	return importQueryJSONResult(map[string]any{
		"phase":                    "verify",
		"output_file":              outputPath,
		"configuration_version_id": configurationVersion.ID,
		"run_id":                   run.ID,
		"plan_id":                  plan.ID,
		"plan_only":                true,
		"verified_result":          fmt.Sprintf("Import-only plan with %d import actions and zero add, change, or destroy actions.", len(candidates)),
	}, logger)
}

func validateImportPlan(plan *tfe.Plan, expectedImports int) error {
	if plan.ResourceImports != expectedImports || plan.ResourceAdditions != 0 || plan.ResourceChanges != 0 || plan.ResourceDestructions != 0 {
		return fmt.Errorf(
			"speculative plan %s is not import-only: imports=%d (expected %d), additions=%d, changes=%d, destructions=%d",
			plan.ID, plan.ResourceImports, expectedImports, plan.ResourceAdditions, plan.ResourceChanges, plan.ResourceDestructions,
		)
	}
	return nil
}

func validateGeneratedConfiguration(configuration string, candidates []importCandidate) error {
	actual, err := parseTerraformConfigurationShape(configuration, "generated_configuration.tf")
	if err != nil {
		return err
	}
	if len(actual.Imports) != len(candidates) {
		return fmt.Errorf("contains %d import blocks; expected exactly %d", len(actual.Imports), len(candidates))
	}

	expectedImports := make(map[string]cty.Value, len(candidates))
	for index, candidate := range candidates {
		candidateHCL := strings.TrimSpace(candidate.Configuration) + "\n" + strings.TrimSpace(candidate.ImportConfig)
		expected, err := parseTerraformConfigurationShape(candidateHCL, fmt.Sprintf("query_candidate_%d.tf", index+1))
		if err != nil {
			return fmt.Errorf("query candidate %d contains invalid generated HCL: %w", index+1, err)
		}
		if len(expected.Resources) != 1 || len(expected.Imports) != 1 {
			return fmt.Errorf("query candidate %d must contain exactly one resource and one import block", index+1)
		}
		for address := range expected.Resources {
			if _, ok := actual.Resources[address]; !ok {
				return fmt.Errorf("missing resource block %s selected by query candidate %d", address, index+1)
			}
		}
		binding := expected.Imports[0]
		if _, duplicate := expectedImports[binding.Target]; duplicate {
			return fmt.Errorf("query results contain duplicate import target %s", binding.Target)
		}
		expectedImports[binding.Target] = binding.Identity
	}

	seen := make(map[string]struct{}, len(actual.Imports))
	for _, binding := range actual.Imports {
		expectedIdentity, ok := expectedImports[binding.Target]
		if !ok {
			return fmt.Errorf("import target %s was not selected by the query", binding.Target)
		}
		if _, duplicate := seen[binding.Target]; duplicate {
			return fmt.Errorf("duplicate import target %s", binding.Target)
		}
		if !binding.Identity.RawEquals(expectedIdentity) {
			return fmt.Errorf("import identity for %s does not match the query result", binding.Target)
		}
		seen[binding.Target] = struct{}{}
	}
	return nil
}

func parseTerraformConfigurationShape(configuration, filename string) (terraformConfigurationShape, error) {
	file, diagnostics := hclsyntax.ParseConfig([]byte(configuration), filename, hcl.Pos{Line: 1, Column: 1})
	if diagnostics.HasErrors() {
		return terraformConfigurationShape{}, fmt.Errorf("invalid Terraform HCL: %s", diagnostics.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return terraformConfigurationShape{}, fmt.Errorf("configuration did not parse as Terraform HCL")
	}
	shape := terraformConfigurationShape{Resources: make(map[string]struct{})}
	for _, block := range body.Blocks {
		switch block.Type {
		case "resource":
			if len(block.Labels) != 2 {
				return terraformConfigurationShape{}, fmt.Errorf("resource block must have type and name labels")
			}
			address := block.Labels[0] + "." + block.Labels[1]
			if _, duplicate := shape.Resources[address]; duplicate {
				return terraformConfigurationShape{}, fmt.Errorf("duplicate resource block %s", address)
			}
			shape.Resources[address] = struct{}{}
		case "import":
			binding, err := parseImportBinding(block)
			if err != nil {
				return terraformConfigurationShape{}, err
			}
			shape.Imports = append(shape.Imports, binding)
		}
	}
	return shape, nil
}

func parseImportBinding(block *hclsyntax.Block) (importBinding, error) {
	to, ok := block.Body.Attributes["to"]
	if !ok {
		return importBinding{}, fmt.Errorf("import block is missing to")
	}
	target, err := simpleResourceAddress(to.Expr)
	if err != nil {
		return importBinding{}, fmt.Errorf("invalid import target: %w", err)
	}

	id, hasID := block.Body.Attributes["id"]
	identity, hasIdentity := block.Body.Attributes["identity"]
	if hasID == hasIdentity {
		return importBinding{}, fmt.Errorf("import block for %s must contain exactly one of id or identity", target)
	}
	attribute := id
	if hasIdentity {
		attribute = identity
	}
	value, diagnostics := attribute.Expr.Value(nil)
	if diagnostics.HasErrors() || !value.IsWhollyKnown() {
		return importBinding{}, fmt.Errorf("import identity for %s must be a literal value", target)
	}
	return importBinding{Target: target, Identity: value}, nil
}

func simpleResourceAddress(expression hclsyntax.Expression) (string, error) {
	traversal, diagnostics := hcl.AbsTraversalForExpr(expression)
	if diagnostics.HasErrors() || len(traversal) != 2 {
		return "", fmt.Errorf("must be a direct resource address in the target module")
	}
	root, rootOK := traversal[0].(hcl.TraverseRoot)
	attribute, attributeOK := traversal[1].(hcl.TraverseAttr)
	if !rootOK || !attributeOK {
		return "", fmt.Errorf("must be a direct resource address in the target module")
	}
	return root.Name + "." + attribute.Name, nil
}

func waitForImportPlan(ctx context.Context, tfeClient *tfe.Client, runID string, pollInterval time.Duration) (*tfe.Plan, error) {
	for {
		run, err := tfeClient.Runs.ReadWithOptions(ctx, runID, &tfe.RunReadOptions{Include: []tfe.RunIncludeOpt{tfe.RunPlan}})
		if err != nil {
			return nil, err
		}
		switch run.Status {
		case tfe.RunCanceled, tfe.RunDiscarded, tfe.RunErrored:
			return nil, fmt.Errorf("run reached terminal status %q", run.Status)
		}
		if run.Plan != nil && run.Plan.ID != "" {
			plan, err := tfeClient.Plans.Read(ctx, run.Plan.ID)
			if err != nil {
				return nil, err
			}
			switch plan.Status {
			case tfe.PlanFinished:
				return plan, nil
			case tfe.PlanCanceled, tfe.PlanErrored, tfe.PlanUnreachable:
				return nil, fmt.Errorf("plan %s reached terminal status %q", plan.ID, plan.Status)
			}
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("timed out waiting for import plan: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func importQueryJSONResult(value any, logger *log.Logger) (*mcp.CallToolResult, error) {
	result, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return importQueryToolErrorf(logger, "failed to marshal response: %v", err)
	}
	return mcp.NewToolResultText(string(result)), nil
}

func importQueryToolErrorf(logger *log.Logger, format string, args ...any) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("import_query_results: %s", message)
	}
	return mcp.NewToolResultError(message), nil
}

const importQueryResultsDescription = `Prepares and verifies bulk imports from a finished HCP Terraform query run.

Call phase=prepare first. Pass the exact query_configuration JSON used with execute_query so a new,
empty target workspace does not need pre-existing Explorer provider metadata. The tool reads the target workspace, asks for configuration_path
when it is absent, reads only the root module selected by the workspace working directory,
extracts generated resource/import blocks from query results, discovers exact provider versions
from query_configuration (or the Explorer API as a fallback), and fetches matching public Registry resource documentation. Shape the
returned HCL to the module's conventions, preferring existing variables, locals, module outputs,
data sources, and resource references over raw literals. Symbols in parent, sibling, or child
modules are out of scope unless exposed through that module's inputs or outputs.

Then call phase=verify with the shaped generated_configuration. Verification requires
confirm_speculative_run=true, validates that every resource and literal import identity matches
the selected query results, refuses to overwrite files, writes the HCL into the target module,
uploads a speculative and provisional configuration version, and creates a plan-only run. It
waits for the plan and succeeds only when it contains exactly the selected import actions with
zero add, change, or destroy actions. This tool never applies a run or imports resources into state.`
