// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
			mcp.WithString("organization_name",
				mcp.Required(),
				mcp.Description(
					"HCP Terraform organization name used to scope every provider catalog request. "+
						"If the user has not supplied it, ask for both organization_name and workspace_name before calling this tool.",
				),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description(
					"HCP Terraform workspace name that will execute the query. "+
						"If the user has not supplied it, ask for both organization_name and workspace_name before calling this tool.",
				),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return providerListSchemaListHandler(ctx, request, logger)
		},
	}
}

func providerListSchemaListHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	providerNamespace := strings.TrimSpace(request.GetString("provider_namespace", ""))
	providerName := strings.TrimSpace(strings.ToLower(request.GetString("provider_name", "")))
	orgName := strings.TrimSpace(request.GetString("organization_name", ""))
	workspaceName := strings.TrimSpace(request.GetString("workspace_name", ""))
	if orgName == "" || workspaceName == "" {
		return searchToolErrorf(logger, "organization_name and workspace_name are required; ask the user to provide both before fetching provider schemas")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return searchToolErrorf(logger, "failed to get Terraform client — ensure TFE_TOKEN and TFE_ADDRESS are configured: %v", err)
	}
	if _, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName); err != nil {
		return searchToolErrorf(logger, "workspace %q not found in organization %q: %v", workspaceName, orgName, err)
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
		return searchToolErrorf(logger, "%v", err)
	}

	return fetchProviderSchema(ctx, tfeClient, orgName, providerNamespace, providerName, providerVersion, logger)
}

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
		return searchToolErrorf(logger, "failed to fetch supported providers: %v", err)
	}

	if len(resp.Data) == 0 {
		return searchToolErrorf(logger, "no search-compatible providers are available for this organization")
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
		return searchToolErrorf(logger, "failed to marshal provider list: %v", err)
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
		return searchToolErrorf(logger, "failed to build schema request for %s/%s@%s: %v", namespace, name, version, err)
	}

	var resp noCodeProviderSchemaResponse
	if err := request.DoJSON(ctx, &resp); err != nil {
		return searchToolErrorf(logger, "failed to fetch schema for %s/%s@%s: %v", namespace, name, version, err)
	}

	lrs := resp.Data.Attributes.ListResourceSchemas
	if lrs == nil || string(lrs) == "null" {
		return searchToolErrorf(logger,
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
		return searchToolErrorf(logger, "failed to marshal schema response: %v", err)
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
	if err := request.DoJSON(ctx, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// searchToolErrorf returns a tool error result and logs the message.
func searchToolErrorf(logger *log.Logger, format string, args ...any) (*mcp.CallToolResult, error) {
	msg := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("provider_list_schema_list: %s", msg)
	}
	return mcp.NewToolResultError(msg), nil
}

const providerListSchemaListDescription = `Fetches list_resource_schemas for a search-compatible Terraform provider from the
HCP Terraform no-code stub endpoint (GET /api/v2/search/provider-versions).

Every call must be scoped with organization_name and workspace_name. Never call this tool
without them. If either value is not present in the user's request, ask the user to provide
both values before making any tool call. Do not attempt an unscoped request first.

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
  1. Obtain organization_name and workspace_name from the user's request. If either is absent,
     ask the user for both and wait for their response.
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
