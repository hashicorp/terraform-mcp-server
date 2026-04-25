// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const MODULE_BASE_PATH = "registry://modules"
const modulePartNameFallback = "(unnamed)"

type modulePartKind struct {
	CollectionName  string
	CollectionTitle string
	SingularName    string
	SingularTitle   string
	ToolName        string
	ToolTitle       string
	ArgumentName    string
	Description     string
	Select          func(client.TerraformModuleVersionDetails) []client.ModulePart
}

var (
	moduleExamplesKind = modulePartKind{
		CollectionName:  "examples",
		CollectionTitle: "Examples",
		SingularName:    "example",
		SingularTitle:   "Example",
		ToolName:        "get_module_examples",
		ToolTitle:       "Retrieve Terraform module examples",
		ArgumentName:    "example_name",
		Description:     `Fetches examples for a Terraform module independently from root module details. Omit example_name to list available examples, or provide example_name to fetch one specific example's README and metadata. You must call 'search_modules' first to obtain the exact valid and compatible module_id required to use this tool.`,
		Select:          func(m client.TerraformModuleVersionDetails) []client.ModulePart { return m.Examples },
	}
	moduleSubmodulesKind = modulePartKind{
		CollectionName:  "submodules",
		CollectionTitle: "Submodules",
		SingularName:    "submodule",
		SingularTitle:   "Submodule",
		ToolName:        "get_module_submodules",
		ToolTitle:       "Retrieve Terraform module submodules",
		ArgumentName:    "submodule_name",
		Description:     `Fetches submodules for a Terraform module independently from root module details. Omit submodule_name to list available submodules, or provide submodule_name to fetch one specific submodule's README, inputs, outputs, resources, and dependencies. You must call 'search_modules' first to obtain the exact valid and compatible module_id required to use this tool.`,
		Select:          func(m client.TerraformModuleVersionDetails) []client.ModulePart { return m.Submodules },
	}
)

func ModuleDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_module_details",
			mcp.WithDescription(`Fetches up-to-date documentation on how to use a Terraform module. Returns root module inputs, outputs, provider dependencies, and lightweight indexes of available examples and submodules. Use get_module_examples or get_module_submodules to fetch selected example or submodule details. You must call 'search_modules' first to obtain the exact valid and compatible module_id required to use this tool.`),
			mcp.WithTitleAnnotation("Retrieve documentation for a specific Terraform module"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithString("module_id",
				mcp.Required(),
				mcp.Description("Exact valid and compatible module_id retrieved from search_modules (e.g., 'squareops/terraform-kubernetes-mongodb/mongodb/2.1.1', 'GoogleCloudPlatform/vertex-ai/google/0.2.0')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getModuleDetailsHandler(ctx, request, logger)
		},
	}
}

func ModuleExamples(logger *log.Logger) server.ServerTool {
	return modulePartTool(logger, moduleExamplesKind)
}

func ModuleSubmodules(logger *log.Logger) server.ServerTool {
	return modulePartTool(logger, moduleSubmodulesKind)
}

func modulePartTool(logger *log.Logger, kind modulePartKind) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(kind.ToolName,
			mcp.WithDescription(kind.Description),
			mcp.WithTitleAnnotation(kind.ToolTitle),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithString("module_id",
				mcp.Required(),
				mcp.Description("Exact valid and compatible module_id retrieved from search_modules (e.g., 'terraform-aws-modules/vpc/aws/2.1.0')"),
			),
			mcp.WithString(kind.ArgumentName,
				mcp.Description(fmt.Sprintf("Optional %s name from get_module_details. If omitted, returns the available %s names and paths.", kind.SingularName, kind.CollectionName)),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getModulePartHandler(ctx, request, logger, kind)
		},
	}
}

func getModuleDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	moduleID, errResult := parseAndValidateModuleID(request, logger)
	if errResult != nil {
		return errResult, nil
	}

	terraformModules, errResult := fetchAndParseModuleDetails(ctx, moduleID, logger)
	if errResult != nil {
		return errResult, nil
	}

	return mcp.NewToolResultText(formatTerraformModule(terraformModules)), nil
}

func getModulePartHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger, kind modulePartKind) (*mcp.CallToolResult, error) {
	moduleID, errResult := parseAndValidateModuleID(request, logger)
	if errResult != nil {
		return errResult, nil
	}

	partName := strings.TrimSpace(request.GetString(kind.ArgumentName, ""))

	terraformModules, errResult := fetchAndParseModuleDetails(ctx, moduleID, logger)
	if errResult != nil {
		return errResult, nil
	}

	moduleData, err := formatTerraformModulePart(terraformModules, kind, partName)
	if err != nil {
		return ToolError(logger, err.Error(), nil)
	}

	return mcp.NewToolResultText(moduleData), nil
}

func parseAndValidateModuleID(request mcp.CallToolRequest, logger *log.Logger) (string, *mcp.CallToolResult) {
	moduleID, err := request.RequireString("module_id")
	if err != nil {
		result, _ := ToolError(logger, "missing required input: module_id", err)
		return "", result
	}
	if moduleID == "" {
		result, _ := ToolError(logger, "module_id cannot be empty", nil)
		return "", result
	}
	if err := validateModuleID(moduleID); err != nil {
		result, _ := ToolError(logger, err.Error(), nil)
		return "", result
	}
	return strings.ToLower(moduleID), nil
}

// fetchAndParseModuleDetails splits its three failure modes (client init, registry fetch,
// JSON parse) so each surfaces a distinct, actionable message rather than a generic
// "module not found".
func fetchAndParseModuleDetails(ctx context.Context, moduleID string, logger *log.Logger) (client.TerraformModuleVersionDetails, *mcp.CallToolResult) {
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		result, _ := ToolError(logger, "failed to get http client for public Terraform registry", err)
		return client.TerraformModuleVersionDetails{}, result
	}

	response, err := getModuleDetails(httpClient, moduleID, 0, logger)
	if err != nil {
		result, _ := ToolErrorf(logger, "module not found: %s - use search_modules first to find valid module IDs", moduleID)
		return client.TerraformModuleVersionDetails{}, result
	}

	module, err := unmarshalTerraformModuleDetails(response)
	if err != nil {
		result, _ := ToolErrorf(logger, "failed to parse module details for %s: %v", moduleID, err)
		return client.TerraformModuleVersionDetails{}, result
	}

	return module, nil
}

func getModuleDetails(httpClient *http.Client, moduleID string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleID != "" {
		uri = fmt.Sprintf("modules/%s", moduleID)
	}

	uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	response, err := client.SendRegistryCall(httpClient, "GET", uri, logger)
	if err != nil {
		return nil, fmt.Errorf("getting module(s) for: %v, please provide a different provider name like aws, azurerm or google etc", moduleID)
	}

	return response, nil
}

func unmarshalTerraformModuleDetails(response []byte) (client.TerraformModuleVersionDetails, error) {
	var terraformModules client.TerraformModuleVersionDetails
	err := json.Unmarshal(response, &terraformModules)
	if err != nil {
		return client.TerraformModuleVersionDetails{}, fmt.Errorf("unmarshalling module details: %w", err)
	}
	return terraformModules, nil
}

func formatTerraformModule(terraformModules client.TerraformModuleVersionDetails) string {
	var builder strings.Builder
	writeModuleHeader(&builder, terraformModules)
	writeModulePartSchema(&builder, terraformModules.Root, false)
	writeModulePartIndex(&builder, terraformModules, moduleExamplesKind, terraformModules.Examples)
	writeModulePartIndex(&builder, terraformModules, moduleSubmodulesKind, terraformModules.Submodules)
	return builder.String()
}

func formatTerraformModulePart(terraformModules client.TerraformModuleVersionDetails, kind modulePartKind, partName string) (string, error) {
	parts := kind.Select(terraformModules)
	var part client.ModulePart
	if partName != "" {
		var found bool
		part, found = findModulePart(parts, partName)
		if !found {
			return "", fmt.Errorf("%s %q not found for module %s. Available %s: %s",
				kind.SingularName,
				partName,
				moduleVersionID(terraformModules),
				kind.CollectionName,
				modulePartNames(parts),
			)
		}
	}

	var builder strings.Builder
	writeModuleHeader(&builder, terraformModules)

	if partName == "" {
		writeModulePartIndex(&builder, terraformModules, kind, parts)
		return builder.String(), nil
	}

	fmt.Fprintf(&builder, "### %s: %s\n\n", kind.SingularTitle, modulePartDisplayName(part))
	if part.Path != "" {
		fmt.Fprintf(&builder, "**Path:** %s\n\n", part.Path)
	}
	if part.Readme != "" {
		builder.WriteString("### README\n\n")
		builder.WriteString(part.Readme)
		builder.WriteString("\n\n")
	}
	writeModulePartSchema(&builder, part, true)
	return builder.String(), nil
}

func writeModuleHeader(builder *strings.Builder, terraformModules client.TerraformModuleVersionDetails) {
	fmt.Fprintf(builder, "# %s/%s/%s\n\n", MODULE_BASE_PATH, terraformModules.Namespace, terraformModules.Name)
	fmt.Fprintf(builder, "**Description:** %s\n\n", terraformModules.Description)
	fmt.Fprintf(builder, "**Module Version:** %s\n\n", terraformModules.Version)
	fmt.Fprintf(builder, "**Namespace:** %s\n\n", terraformModules.Namespace)
	fmt.Fprintf(builder, "**Source:** %s\n\n", terraformModules.Source)
}

func moduleVersionID(terraformModules client.TerraformModuleVersionDetails) string {
	return fmt.Sprintf("%s/%s/%s/%s", terraformModules.Namespace, terraformModules.Name, terraformModules.Provider, terraformModules.Version)
}

func writeModulePartSchema(builder *strings.Builder, part client.ModulePart, includeImplementationDetails bool) {
	if len(part.Inputs) > 0 {
		builder.WriteString("### Inputs\n\n")
		builder.WriteString("| Name | Type | Description | Default | Required |\n")
		builder.WriteString("|---|---|---|---|---|\n")
		for _, input := range part.Inputs {
			fmt.Fprintf(builder, "| %s | %s | %s | `%v` | %t |\n",
				escapeMarkdownTableCell(input.Name),
				escapeMarkdownTableCell(input.Type),
				escapeMarkdownTableCell(input.Description),
				input.Default,
				input.Required,
			)
		}
		builder.WriteString("\n")
	}

	if len(part.Outputs) > 0 {
		builder.WriteString("### Outputs\n\n")
		builder.WriteString("| Name | Description |\n")
		builder.WriteString("|---|---|\n")
		for _, output := range part.Outputs {
			fmt.Fprintf(builder, "| %s | %s |\n",
				escapeMarkdownTableCell(output.Name),
				escapeMarkdownTableCell(output.Description),
			)
		}
		builder.WriteString("\n")
	}

	if includeImplementationDetails && len(part.Dependencies) > 0 {
		builder.WriteString("### Module Dependencies\n\n")
		builder.WriteString("| Name | Source | Version |\n")
		builder.WriteString("|---|---|---|\n")
		for _, dep := range part.Dependencies {
			fmt.Fprintf(builder, "| %s | %s | %s |\n",
				escapeMarkdownTableCell(dep.Name),
				escapeMarkdownTableCell(dep.Source),
				escapeMarkdownTableCell(dep.Version),
			)
		}
		builder.WriteString("\n")
	}

	if len(part.ProviderDependencies) > 0 {
		builder.WriteString("### Provider Dependencies\n\n")
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|---|---|---|---|\n")
		for _, dep := range part.ProviderDependencies {
			fmt.Fprintf(builder, "| %s | %s | %s | %s |\n",
				escapeMarkdownTableCell(dep.Name),
				escapeMarkdownTableCell(dep.Namespace),
				escapeMarkdownTableCell(dep.Source),
				escapeMarkdownTableCell(dep.Version),
			)
		}
		builder.WriteString("\n")
	}

	if includeImplementationDetails && len(part.Resources) > 0 {
		builder.WriteString("### Resources\n\n")
		builder.WriteString("| Name | Type |\n")
		builder.WriteString("|---|---|\n")
		for _, resource := range part.Resources {
			fmt.Fprintf(builder, "| %s | %s |\n",
				escapeMarkdownTableCell(resource.Name),
				escapeMarkdownTableCell(resource.Type),
			)
		}
		builder.WriteString("\n")
	}
}

// escapeMarkdownTableCell prevents registry-supplied text from breaking pipe-delimited
// table rows. Pipes are escaped; CR/LF are collapsed to spaces.
var markdownTableCellReplacer = strings.NewReplacer("|", `\|`, "\r\n", " ", "\n", " ", "\r", " ")

func escapeMarkdownTableCell(s string) string {
	return markdownTableCellReplacer.Replace(s)
}

func writeModulePartIndex(builder *strings.Builder, terraformModules client.TerraformModuleVersionDetails, kind modulePartKind, parts []client.ModulePart) {
	fmt.Fprintf(builder, "### Available %s\n\n", kind.CollectionTitle)
	if len(parts) == 0 {
		fmt.Fprintf(builder, "No %s found for this module.\n\n", kind.CollectionName)
		return
	}

	builder.WriteString("| Name | Path |\n")
	builder.WriteString("|---|---|\n")
	for _, part := range parts {
		fmt.Fprintf(builder, "| %s | %s |\n",
			escapeMarkdownTableCell(modulePartDisplayName(part)),
			escapeMarkdownTableCell(modulePartDisplayPath(part)),
		)
	}
	builder.WriteString("\n")
	fmt.Fprintf(builder, "To fetch one %s, call `%s` with `module_id` set to `%s` and `%s` set to one of the names above.\n\n",
		kind.SingularName,
		kind.ToolName,
		moduleVersionID(terraformModules),
		kind.ArgumentName,
	)
}

func findModulePart(parts []client.ModulePart, name string) (client.ModulePart, bool) {
	// Path is matched case-sensitively because registry paths mirror filesystem paths.
	for _, part := range parts {
		if strings.EqualFold(part.Name, name) || part.Path == name {
			return part, true
		}
		if part.Name == "" && strings.EqualFold(modulePartDisplayName(part), name) {
			return part, true
		}
	}
	return client.ModulePart{}, false
}

func modulePartDisplayName(part client.ModulePart) string {
	if part.Name != "" {
		return part.Name
	}
	path := strings.Trim(part.Path, "/")
	if path == "" {
		return modulePartNameFallback
	}
	segments := strings.Split(path, "/")
	return segments[len(segments)-1]
}

func modulePartDisplayPath(part client.ModulePart) string {
	if part.Path == "" {
		return "-"
	}
	return part.Path
}

func modulePartNames(parts []client.ModulePart) string {
	if len(parts) == 0 {
		return "none"
	}

	names := make([]string, 0, len(parts))
	for _, part := range parts {
		names = append(names, modulePartDisplayName(part))
	}
	return strings.Join(names, ", ")
}

func validateModuleID(moduleID string) error {
	parts := strings.Split(moduleID, "/")
	if len(parts) != 4 {
		return fmt.Errorf("invalid module ID format '%s'. Expected format: namespace/name/provider/version (4 parts). Use search_modules to find valid module IDs", moduleID)
	}
	return nil
}
