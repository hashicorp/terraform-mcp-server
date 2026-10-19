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
func ProviderListSchemaList(logger *log.Logger, servers ...*server.MCPServer) server.ServerTool {
	var mcpServer *server.MCPServer
	if len(servers) > 0 {
		mcpServer = servers[0]
	}
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
			mcp.WithString(ScopeOrganizationInput, mcp.Description(QueryOrganizationDescription)),
			mcp.WithString("workspace_name", mcp.Description(QueryWorkspaceDescription)),
			mcp.WithSchemaAdditionalProperties(false),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return providerListSchemaListHandler(ctx, request, logger, mcpServer)
		},
	}
}

func providerListSchemaListHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger, servers ...*server.MCPServer) (*mcp.CallToolResult, error) {
	var mcpServer *server.MCPServer
	if len(servers) > 0 {
		mcpServer = servers[0]
	}
	providerNamespace := strings.TrimSpace(request.GetString("provider_namespace", ""))
	providerName := strings.TrimSpace(strings.ToLower(request.GetString("provider_name", "")))
	scope, interactionResult := resolveMark3LabsScope(ctx, mcpServer, ResolveSearchScope(request.GetString(ScopeOrganizationInput, ""), request.GetString(ScopeWorkspaceInput, "")), QuerySourceScope)
	if interactionResult != nil {
		return interactionResult, nil
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
		return "", fmt.Errorf("terraform_org_name and workspace_name are required; ask the user to provide both before fetching provider schemas")
	}
	if (providerNamespace == "") != (providerName == "") {
		return "", fmt.Errorf("provider_namespace and provider_name must either both be supplied or both be omitted")
	}
	if _, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName); err != nil {
		return "", fmt.Errorf("failed to read workspace %q in organization %q: %w", workspaceName, orgName, err)
	}

	// ── Branch: list all providers ────────────────────────────────────────────
	if providerNamespace == "" {
		text, err := listSupportedProvidersText(ctx, tfeClient, orgName)
		return providerResultWithScope(text, orgName, workspaceName, err)
	}

	// ── Branch: fetch schema for a specific provider ──────────────────────────

	// The catalog is authoritative for the version. Do not accept a caller- or
	// model-supplied version because it may not have list-resource schemas.
	selection, err := discoverProviderSelection(ctx, tfeClient, orgName, providerNamespace, providerName)
	if err != nil {
		return "", err
	}

	text, err := fetchProviderSchemaText(ctx, tfeClient, orgName, selection.Namespace, selection.Name, selection.Version)
	return providerResultWithScope(text, orgName, workspaceName, err)
}

func providerResultWithScope(text, organizationName, workspaceName string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return "", fmt.Errorf("failed to preserve confirmed Search scope")
	}
	result[ScopeOrganizationInput] = organizationName
	result[ScopeWorkspaceInput] = workspaceName
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to preserve confirmed Search scope")
	}
	return string(encoded), nil
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
		"note":                "Call provider_list_schema_list again with the same terraform_org_name and workspace_name, plus provider_namespace and provider_name, to fetch the full list_resource_schemas for a specific provider. FETCH uses the catalog-selected version.",
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
			"call provider_list_schema_list with terraform_org_name and workspace_name to see supported providers, "+
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
				"ordinary managed resource names are not automatically supported list resources. Then pass the configuration with terraform_org_name and workspace_name to execute_query.",
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

const providerListSchemaListDescription = `Lists Search-compatible providers or fetches one provider's list-resource
schemas for an HCP Terraform source workspace.

The terraform_org_name and workspace_name source arguments are optional. Omitted
fields resolve proposed values from TF_CLOUD_ORGANIZATION and
TF_MCP_QUERY_WORKSPACE, then TF_WORKSPACE. The server uses native MCP user input
to show and confirm the complete source before scoped API access. Reuse the
accepted pair explicitly in later Search calls.

In LIST mode, omit both provider_namespace and provider_name; the result contains
the providers available in the Search catalog. In FETCH mode, provide both
identifiers; the tool returns the catalog-selected provider metadata and
list_resource_schemas. The provider version comes from the catalog and must not
be inferred or supplied from model knowledge or the public Registry.

After LIST, choose a catalog provider with the user and call FETCH. After FETCH,
pass the returned provider metadata and list_resource_schemas to
generate_query_configuration. A searchable resource_type must be an exact key
in list_resource_schemas; an ordinary managed-resource name does not establish
Search list support. List-resource schemas are not target managed-resource
schemas.`
