// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ProviderListSchemaList returns the MCP tool that fetches list_resource_schemas
// from the HCP Terraform no-code stub endpoint and returns the raw schema blob
// ready to pass directly into generate_query_configuration.
//
// Endpoints used:
//
//	GET /api/v2/search/provider-versions
//	    → list of search-compatible providers (namespace, name, version)
//
//	GET /api/v2/search/provider-versions/:namespace/:name/:version
//	    → full schema for that provider, including list_resource_schemas
//
// When the caller supplies namespace + name, the tool
// fetches the schema for that specific provider directly.
// When neither is supplied, it lists all supported providers for the required
// organization context so the agent can pick one and call the tool again.
func ProviderListSchemaList(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("provider_list_schema_list",
			mcp.WithDescription(providerListSchemaListDescription),
			mcp.WithTitleAnnotation("Fetch list_resource_schemas for a search-compatible provider"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("provider_namespace",
				mcp.Description(
					"Provider namespace, e.g. \"hashicorp\". "+
						"Required together with provider_name to fetch a schema directly. "+
						"When omitted (along with provider_name) the tool lists all supported providers instead.",
				),
			),
			mcp.WithString("provider_name",
				mcp.Description(
					"Short provider name, e.g. \"aws\", \"azurerm\", \"google\". "+
						"Required together with provider_namespace to fetch a schema directly.",
				),
			),
			mcp.WithString("organization_name", mcp.Description(queryOrganizationDescription)),
			mcp.WithString("workspace_name", mcp.Description(queryWorkspaceDescription)),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return providerListSchemaListHandler(ctx, request, logger)
		},
	}
}

func providerListSchemaListHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	providerNamespace := strings.TrimSpace(request.GetString("provider_namespace", ""))
	providerName := strings.TrimSpace(strings.ToLower(request.GetString("provider_name", "")))
	queryScope := resolveSearchScope(request)
	if !queryScope.Ready() {
		return mcp.NewToolResultText(queryScope.InputRequest()), nil
	}
	orgName, workspaceName := queryScope.Organization.Value, queryScope.Workspace.Value

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to get Terraform client — ensure TFE_TOKEN and TFE_ADDRESS are configured: %v", err)
	}
	if _, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName); err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to read workspace %q in organization %q: %v", workspaceName, orgName, err)
	}

	// ── Branch: list all providers ────────────────────────────────────────────
	if providerNamespace == "" || providerName == "" {
		return listSupportedProviders(ctx, tfeClient, orgName, logger)
	}

	// ── Branch: fetch schema for a specific provider ──────────────────────────

	// The catalog is authoritative for the version. Do not accept a caller- or
	// model-supplied version because it may not have list-resource schemas.
	providerVersion, err := discoverProviderVersion(ctx, tfeClient, orgName, providerNamespace, providerName)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "%v", err)
	}

	return fetchProviderSchema(ctx, tfeClient, orgName, providerNamespace, providerName, providerVersion, logger)
}

type searchScopeValue struct {
	Value                string `json:"value"`
	Source               string `json:"source,omitempty"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

type searchScope struct {
	Organization searchScopeValue `json:"organization_name"`
	Workspace    searchScopeValue `json:"workspace_name"`
}

func resolveSearchScope(request mcp.CallToolRequest) searchScope {
	return searchScope{
		Organization: resolveSearchScopeValue(request.GetString("organization_name", ""), "", "TF_CLOUD_ORGANIZATION", "TFC_ORGANIZATION"),
		Workspace:    resolveSearchScopeValue(request.GetString("workspace_name", ""), "TF_MCP_QUERY_WORKSPACE", "TF_WORKSPACE"),
	}
}

func resolveSearchScopeValue(explicit, mcpEnv string, defaults ...string) searchScopeValue {
	if value := strings.TrimSpace(explicit); value != "" {
		return searchScopeValue{Value: value, Source: "argument"}
	}
	if mcpEnv != "" {
		if value := strings.TrimSpace(os.Getenv(mcpEnv)); value != "" {
			return searchScopeValue{Value: value, Source: mcpEnv}
		}
	}
	for _, name := range defaults {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return searchScopeValue{Value: value, Source: name, RequiresConfirmation: true}
		}
	}
	return searchScopeValue{}
}

func (s searchScope) Ready() bool {
	return s.Organization.Value != "" && s.Workspace.Value != "" &&
		!s.Organization.RequiresConfirmation && !s.Workspace.RequiresConfirmation
}

// InputRequest exposes only organization/workspace identifiers, never credentials.
func (s searchScope) InputRequest() string {
	missing := []string{}
	if s.Organization.Value == "" {
		missing = append(missing, "organization_name")
	}
	if s.Workspace.Value == "" {
		missing = append(missing, "workspace_name")
	}
	status := "input_required"
	message := "organization_name and workspace_name are required; ask the user for the missing inputs, then retry with explicit arguments."
	if s.Organization.RequiresConfirmation || s.Workspace.RequiresConfirmation {
		status = "confirmation_required"
		message = "Terraform defaults are candidates only; ask the user whether to use these defaults or supply their own. Ask for any missing inputs too. Wait for their response, then retry with accepted or replacement values as explicit arguments. Do not make scoped API requests before confirmation."
	}
	payload, _ := json.Marshal(struct {
		Status  string      `json:"status"`
		Message string      `json:"message"`
		Scope   searchScope `json:"scope"`
		Missing []string    `json:"missing_inputs"`
	}{status, message, s, missing})
	return string(payload)
}

const queryOrganizationDescription = "HCP Terraform organization containing the query workspace. Explicit input overrides environment values. TF_CLOUD_ORGANIZATION, then TFC_ORGANIZATION, are candidates only: ask the user to confirm or replace them before passing the value. If absent, ask the user for organization_name."

const queryWorkspaceDescription = "HCP Terraform workspace for queries. Explicit input overrides TF_MCP_QUERY_WORKSPACE. If neither is set, TF_WORKSPACE is a candidate only: ask the user to confirm or replace it before passing the value. If absent, ask the user for workspace_name. TF_MCP_IMPORT_WORKSPACE is a separate import target, not a query default."

// ── list all supported providers ─────────────────────────────────────────────

// noCodeProviderVersionsResponse is the JSON:API response from
// GET /api/v2/search/provider-versions.
type noCodeProviderVersionsResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Version   string `json:"version"`
		} `json:"attributes"`
	} `json:"data"`
}

// noCodeProviderSchemaResponse is the JSON:API response from
// GET /api/v2/search/provider-versions/:namespace/:name/:version.
type noCodeProviderSchemaResponse struct {
	Data struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Namespace           string          `json:"namespace"`
			Name                string          `json:"name"`
			Version             string          `json:"version"`
			ListResourceSchemas json.RawMessage `json:"list-resource-schemas"`
		} `json:"attributes"`
	} `json:"data"`
}

func listSupportedProviders(ctx context.Context, tfeClient *tfe.Client, orgName string, logger *log.Logger) (*mcp.CallToolResult, error) {
	resp, err := readProviderVersions(ctx, tfeClient, orgName)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to fetch supported providers: %v", err)
	}

	if len(resp.Data) == 0 {
		return toolErrorf(logger, "provider_list_schema_list", "no search-compatible providers are available for this organization")
	}

	type providerSummary struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Version   string `json:"version"`
	}

	summaries := make([]providerSummary, 0, len(resp.Data))
	for _, d := range resp.Data {
		summaries = append(summaries, providerSummary{
			Namespace: d.Attributes.Namespace,
			Name:      d.Attributes.Name,
			Version:   d.Attributes.Version,
		})
	}

	out, err := json.MarshalIndent(map[string]any{
		"supported_providers": summaries,
		"note":                "Call provider_list_schema_list again with the same organization_name and workspace_name, plus provider_namespace and provider_name (and optionally provider_version), to fetch the full list_resource_schemas for a specific provider.",
	}, "", "  ")
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to marshal provider list: %v", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}

// ── discover version from index ───────────────────────────────────────────────

func discoverProviderVersion(ctx context.Context, tfeClient *tfe.Client, orgName, namespace, name string) (string, error) {
	resp, err := readProviderVersions(ctx, tfeClient, orgName)
	if err != nil {
		return "", fmt.Errorf("failed to fetch provider list to discover version: %w", err)
	}

	for _, d := range resp.Data {
		if strings.EqualFold(d.Attributes.Namespace, namespace) && strings.EqualFold(d.Attributes.Name, name) {
			return d.Attributes.Version, nil
		}
	}

	return "", fmt.Errorf(
		"provider %s/%s is not in the search-compatible catalog — "+
			"call provider_list_schema_list with organization_name and workspace_name to see supported providers, "+
			"or use search_providers to find a provider in the public Terraform Registry",
		namespace, name,
	)
}

// ── fetch schema for a specific provider ──────────────────────────────────────

func fetchProviderSchema(ctx context.Context, tfeClient *tfe.Client, orgName, namespace, name, version string, logger *log.Logger) (*mcp.CallToolResult, error) {
	requestPath := fmt.Sprintf("search/provider-versions/%s/%s/%s",
		url.PathEscape(namespace),
		url.PathEscape(name),
		url.PathEscape(version),
	)
	if orgName != "" {
		query := url.Values{}
		query.Set("filter[organization][name]", orgName)
		requestPath += "?" + query.Encode()
	}

	request, err := tfeClient.NewRequest(http.MethodGet, requestPath, nil)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to build schema request for %s/%s@%s: %v", namespace, name, version, err)
	}

	var resp noCodeProviderSchemaResponse
	if err := doProviderRequest(ctx, request, &resp); err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to fetch schema for %s/%s@%s: %v", namespace, name, version, err)
	}

	lrs := resp.Data.Attributes.ListResourceSchemas
	if lrs == nil || string(lrs) == "null" {
		return toolErrorf(logger, "provider_list_schema_list",
			"provider %s/%s@%s exists in the catalog but has no list_resource_schemas — "+
				"schema generation may not have run for this version yet",
			namespace, name, version,
		)
	}

	out, err := json.MarshalIndent(map[string]any{
		"namespace":             resp.Data.Attributes.Namespace,
		"name":                  resp.Data.Attributes.Name,
		"version":               resp.Data.Attributes.Version,
		"list_resource_schemas": lrs,
		"note": fmt.Sprintf(
			"Pass list_resource_schemas to generate_query_configuration "+
				"(with provider_namespace=%q, provider_name=%q, provider_version=%q) "+
				"to get a full schema guide and example configuration. Use only exact resource type keys present in list_resource_schemas; "+
				"ordinary managed resource names are not automatically supported list resources. Then pass the configuration with organization_name and workspace_name to execute_query.",
			resp.Data.Attributes.Namespace,
			resp.Data.Attributes.Name,
			resp.Data.Attributes.Version,
		),
	}, "", "  ")
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to marshal schema response: %v", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}

func readProviderVersions(ctx context.Context, tfeClient *tfe.Client, orgName string) (*noCodeProviderVersionsResponse, error) {
	requestPath := "search/provider-versions"
	if orgName != "" {
		query := url.Values{}
		query.Set("filter[organization][name]", orgName)
		requestPath += "?" + query.Encode()
	}

	request, err := tfeClient.NewRequest(http.MethodGet, requestPath, nil)
	if err != nil {
		return nil, fmt.Errorf("building provider list request: %w", err)
	}

	var resp noCodeProviderVersionsResponse
	if err := doProviderRequest(ctx, request, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func doProviderRequest(ctx context.Context, request *tfe.ClientRequest, response any) error {
	var body bytes.Buffer
	if err := request.Do(ctx, &body); err != nil {
		return err
	}
	return json.Unmarshal(body.Bytes(), response)
}

const providerListSchemaListDescription = `Fetches list_resource_schemas for a search-compatible Terraform provider from the
HCP Terraform no-code stub endpoint (GET /api/v2/search/provider-versions).

Every call requires a resolved organization_name and workspace_name. Explicit user inputs override
environment values. Otherwise use TF_MCP_QUERY_WORKSPACE for queries; TF_MCP_IMPORT_WORKSPACE is
a separate import target. If the query variable is absent, TF_WORKSPACE is a candidate only.
For organization, check TF_CLOUD_ORGANIZATION, then TFC_ORGANIZATION. Before using Terraform
defaults, show their values and sources and ask the user to confirm them or supply their own.
If inputs are missing, ask only for those inputs. Never make an unscoped request or silently
accept Terraform defaults. A confirmation_required or input_required result is a normal response,
not a tool failure. Use its context and guidance to ask the user a concise question, then wait
for the user's response and retry with accepted or replacement values as
explicit arguments. Confirmation is handled by the agent, not an interactive server terminal.

The tool has two modes:

LIST mode (organization_name and workspace_name supplied; no provider identifiers supplied):
  Returns all providers currently in the search-compatible catalog so the agent
  can choose one. Each entry includes namespace, name, and version.

  FETCH mode (organization_name, workspace_name, provider_namespace, and provider_name supplied):
  Returns the full list_resource_schemas for the requested provider, ready to
  pass directly into generate_query_configuration.
  - The provider version is always read from the provider catalog response. Never infer or
    supply a version from model knowledge, examples, or the public Terraform Registry.
  - If the provider is not found in the catalog, an error is returned with a hint
    to use search_providers to find the provider in the public Terraform Registry.

Typical agent workflow:
	1. Resolve the context using explicit inputs, then TF_MCP_QUERY_WORKSPACE. For Terraform
		 defaults, ask the user to confirm or replace the values; ask for missing inputs and wait.
  2. Call provider_list_schema_list(organization_name, workspace_name) to discover available providers.
  3. Call provider_list_schema_list(organization_name, workspace_name, provider_namespace,
     provider_name) to fetch the catalog-selected version and its schema.
  4. Pass the returned list_resource_schemas to generate_query_configuration to get
     a full guide and example query configuration.
  5. Select only an exact resource type key present in list_resource_schemas. Never infer list
     support from an ordinary managed resource name. If no key matches the user's request,
     explain that the selected provider version cannot list that resource and stop.
  6. Fill in the configuration and pass it with organization_name and workspace_name to execute_query.

Requires TFE_TOKEN and TFE_ADDRESS to be configured (same credentials used for
other HCP Terraform tools).`
