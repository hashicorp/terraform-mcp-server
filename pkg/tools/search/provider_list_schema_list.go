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
			mcp.WithString("organization_name", mcp.Description(QueryOrganizationDescription)),
			mcp.WithString("workspace_name", mcp.Description(QueryWorkspaceDescription)),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return providerListSchemaListHandler(ctx, request, logger)
		},
	}
}

func providerListSchemaListHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	providerNamespace := strings.TrimSpace(request.GetString("provider_namespace", ""))
	providerName := strings.TrimSpace(strings.ToLower(request.GetString("provider_name", "")))
	scope := ResolveSearchScope(request.GetString("organization_name", ""), request.GetString("workspace_name", ""))
	if !scope.Ready() {
		return mcp.NewToolResultText(scope.InputRequest()), nil
	}
	orgName, workspaceName := scope.Organization.Value, scope.Workspace.Value

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "failed to get Terraform client — ensure TFE_TOKEN and TFE_ADDRESS are configured: %v", err)
	}
	return providerListSchemaListWithClient(ctx, tfeClient, orgName, workspaceName, providerNamespace, providerName, logger)
}

func providerListSchemaListWithClient(ctx context.Context, tfeClient *tfe.Client, orgName, workspaceName, providerNamespace, providerName string, logger *log.Logger) (*mcp.CallToolResult, error) {
	text, err := ProviderListSchemaListText(ctx, tfeClient, orgName, workspaceName, providerNamespace, providerName)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "%v", err)
	}
	return mcp.NewToolResultText(text), nil
}

// ProviderListSchemaListText checks the workspace and reads an organization-scoped catalog.
// It returns plain text/error so neither MCP transport needs to translate the other's result type.
func ProviderListSchemaListText(ctx context.Context, tfeClient *tfe.Client, orgName, workspaceName, providerNamespace, providerName string) (string, error) {
	orgName = strings.TrimSpace(orgName)
	workspaceName = strings.TrimSpace(workspaceName)
	providerNamespace = strings.TrimSpace(providerNamespace)
	providerName = strings.TrimSpace(strings.ToLower(providerName))
	if orgName == "" || workspaceName == "" {
		return "", fmt.Errorf("organization_name and workspace_name are required; ask the user to provide both before fetching provider schemas")
	}
	if (providerNamespace == "") != (providerName == "") {
		return "", fmt.Errorf("provider_namespace and provider_name must either both be supplied or both be omitted")
	}
	if _, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName); err != nil {
		return "", fmt.Errorf("failed to read workspace %q in organization %q: %w", workspaceName, orgName, err)
	}

	// ── Branch: list all providers ────────────────────────────────────────────
	if providerNamespace == "" {
		return listSupportedProvidersText(ctx, tfeClient, orgName)
	}

	// ── Branch: fetch schema for a specific provider ──────────────────────────

	// The catalog is authoritative for the version. Do not accept a caller- or
	// model-supplied version because it may not have list-resource schemas.
	selection, err := discoverProviderSelection(ctx, tfeClient, orgName, providerNamespace, providerName)
	if err != nil {
		return "", err
	}

	return fetchProviderSchemaText(ctx, tfeClient, orgName, selection.Namespace, selection.Name, selection.Version)
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
	text, err := listSupportedProvidersText(ctx, tfeClient, orgName)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "%v", err)
	}
	return mcp.NewToolResultText(text), nil
}

func listSupportedProvidersText(ctx context.Context, tfeClient *tfe.Client, orgName string) (string, error) {
	resp, err := readProviderVersions(ctx, tfeClient, orgName)
	if err != nil {
		return "", fmt.Errorf("failed to fetch supported providers: %w", err)
	}

	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no search-compatible providers are available for this organization")
	}

	type providerSummary struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		Version   string `json:"version"`
	}

	summaries := make([]providerSummary, 0, len(resp.Data))
	for _, d := range resp.Data {
		if d.Attributes.Namespace == "" || d.Attributes.Name == "" || d.Attributes.Version == "" {
			return "", fmt.Errorf("provider catalog returned incomplete metadata; retry provider discovery later")
		}
		summaries = append(summaries, providerSummary{
			Namespace: d.Attributes.Namespace,
			Name:      d.Attributes.Name,
			Version:   d.Attributes.Version,
		})
	}

	out, err := json.MarshalIndent(map[string]any{
		"supported_providers": summaries,
		"note":                "Call provider_list_schema_list again with the same organization_name and workspace_name, plus provider_namespace and provider_name, to fetch the full list_resource_schemas for a specific provider. FETCH uses the catalog-selected version.",
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal provider list: %w", err)
	}

	return string(out), nil
}

// ── select provider metadata from the catalog ────────────────────────────────

type noCodeProviderSelection struct {
	Namespace string
	Name      string
	Version   string
}

func discoverProviderSelection(ctx context.Context, tfeClient *tfe.Client, orgName, namespace, name string) (noCodeProviderSelection, error) {
	resp, err := readProviderVersions(ctx, tfeClient, orgName)
	if err != nil {
		return noCodeProviderSelection{}, fmt.Errorf("failed to fetch provider list to discover version: %w", err)
	}

	for _, d := range resp.Data {
		if strings.EqualFold(d.Attributes.Namespace, namespace) && strings.EqualFold(d.Attributes.Name, name) {
			if d.Attributes.Namespace == "" || d.Attributes.Name == "" || d.Attributes.Version == "" {
				return noCodeProviderSelection{}, fmt.Errorf("provider catalog returned incomplete metadata; retry provider discovery later")
			}
			return noCodeProviderSelection{Namespace: d.Attributes.Namespace, Name: d.Attributes.Name, Version: d.Attributes.Version}, nil
		}
	}

	return noCodeProviderSelection{}, fmt.Errorf(
		"provider %s/%s is not in the search-compatible catalog — "+
			"call provider_list_schema_list with organization_name and workspace_name to see supported providers, "+
			"or use search_providers to find a provider in the public Terraform Registry",
		namespace, name,
	)
}

// ── fetch schema for a specific provider ──────────────────────────────────────

func fetchProviderSchema(ctx context.Context, tfeClient *tfe.Client, orgName, namespace, name, version string, logger *log.Logger) (*mcp.CallToolResult, error) {
	text, err := fetchProviderSchemaText(ctx, tfeClient, orgName, namespace, name, version)
	if err != nil {
		return toolErrorf(logger, "provider_list_schema_list", "%v", err)
	}
	return mcp.NewToolResultText(text), nil
}

func fetchProviderSchemaText(ctx context.Context, tfeClient *tfe.Client, orgName, namespace, name, version string) (string, error) {
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
		return "", fmt.Errorf("failed to build schema request for %s/%s@%s: %w", namespace, name, version, err)
	}

	var resp noCodeProviderSchemaResponse
	if err := doProviderRequest(ctx, request, &resp); err != nil {
		return "", fmt.Errorf("failed to fetch schema for %s/%s@%s: %w", namespace, name, version, err)
	}
	// Atlas's schema-detail serializer exposes schemas, not provider metadata.
	// The catalog selection supplied these values; a detail response that does
	// include metadata must not silently contradict the selected provider.
	attrs := resp.Data.Attributes
	if attrs.Namespace != "" && !strings.EqualFold(attrs.Namespace, namespace) ||
		attrs.Name != "" && !strings.EqualFold(attrs.Name, name) ||
		attrs.Version != "" && attrs.Version != version {
		return "", fmt.Errorf("provider schema metadata conflicts with catalog selection; retry provider discovery")
	}

	lrs := attrs.ListResourceSchemas
	if lrs == nil || string(lrs) == "null" {
		return "", fmt.Errorf(
			"provider %s/%s@%s exists in the catalog but has no list_resource_schemas — "+
				"schema generation may not have run for this version yet",
			namespace, name, version,
		)
	}

	out, err := json.MarshalIndent(map[string]any{
		"namespace":             namespace,
		"name":                  name,
		"version":               version,
		"list_resource_schemas": lrs,
		"note": fmt.Sprintf(
			"Pass list_resource_schemas to generate_query_configuration "+
				"(with provider_namespace=%q, provider_name=%q, provider_version=%q) "+
				"to get a full schema guide and example configuration. Use only exact resource type keys present in list_resource_schemas; "+
				"ordinary managed resource names are not automatically supported list resources. Then pass the configuration with organization_name and workspace_name to execute_query.",
			namespace,
			name,
			version,
		),
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema response: %w", err)
	}

	return string(out), nil
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

Scope arguments are optional. Use configured environment defaults unless the user explicitly
specifies an organization or workspace; pass those explicit values as arguments to override
the corresponding defaults. Do not invent scope arguments. The server uses
TF_MCP_QUERY_WORKSPACE for queries; TF_MCP_IMPORT_WORKSPACE is
a separate import target. If the query variable is absent, use TF_WORKSPACE.
For organization, use TF_CLOUD_ORGANIZATION unless explicitly overridden. Use configured environment
values directly without confirmation. If inputs are missing, ask only for those inputs.
The server never makes an unscoped API request. An input_required result is a normal response, not a tool
failure. Use its context and guidance to ask the user only for the missing inputs, then
retry with those values as explicit arguments, preserving resolved context.

The tool has two modes:

LIST mode (no provider identifiers supplied; scope resolved by the server):
  Returns all providers currently in the search-compatible catalog so the agent
  can choose one. Each entry includes namespace, name, and version.

FETCH mode (provider_namespace and provider_name supplied; scope resolved by the server):
  Returns the full list_resource_schemas for the requested provider, ready to
  pass directly into generate_query_configuration.
  - The provider version is always read from the provider catalog response. Never infer or
    supply a version from model knowledge, examples, or the public Terraform Registry.
  - If the provider is not found in the catalog, an error is returned with a hint
    to use search_providers to find the provider in the public Terraform Registry.

Typical agent workflow:
If the user already supplied an explicit organization or workspace, pass it as a scope argument to override that default throughout this workflow. Otherwise follow the default-first steps below.
  1. Call without scope args first; if the result reports a missing value, ask the user for only that value and retry.
  2. Use the LIST response to discover available providers.
  3. Call provider_list_schema_list with provider_namespace and provider_name to fetch the catalog-selected version and its schema. Reuse any explicit scope inputs from step 1.
  4. Pass the returned list_resource_schemas to generate_query_configuration to get
     a full guide and example query configuration.
  5. Select only an exact resource type key present in list_resource_schemas. Never infer list
     support from an ordinary managed resource name. If no key matches the user's request,
     explain that the selected provider version cannot list that resource and stop.
  6. Fill in the configuration and pass it to execute_query, reusing any explicit scope inputs from step 1; otherwise let the server resolve configured scope values.

Requires TFE_TOKEN and TFE_ADDRESS to be configured (same credentials used for
other HCP Terraform tools).`
