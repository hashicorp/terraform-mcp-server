// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
// When the caller supplies namespace + name (+ optional version), the tool
// fetches the schema for that specific provider directly.
// When neither is supplied, it lists all supported providers and returns them
// so the agent can pick one and call the tool again.
func ProviderListSchemaList(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("provider_list_schema_list",
			mcp.WithDescription(providerListSchemaListDescription),
			mcp.WithTitleAnnotation("Fetch list_resource_schemas for a search-compatible provider"),
			mcp.WithOpenWorldHintAnnotation(false),
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
			mcp.WithString("provider_version",
				mcp.Description(
					"Specific provider version to fetch, e.g. \"6.33.0\". "+
						"When omitted the latest version available in the stub catalog is used.",
				),
			),
			mcp.WithString("organization_name",
				mcp.Description(
					"HCP Terraform organization name. "+
						"Used to scope the provider-versions endpoint to the organization's "+
						"search-compatible catalog. Required when the HCP Terraform instance "+
						"enforces organization-scoped feature flags.",
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
	providerVersion := strings.TrimSpace(request.GetString("provider_version", ""))
	orgName := strings.TrimSpace(request.GetString("organization_name", ""))

	// Resolve bearer token from context (or env).
	token := client.GetTokenFromContext(ctx)
	if token == "" {
		return searchToolErrorf(logger, "no Terraform token available — ensure TFE_TOKEN is configured")
	}

	// Get the TFE client to derive the base URL.
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return searchToolErrorf(logger, "failed to get Terraform client — ensure TFE_TOKEN and TFE_ADDRESS are configured: %v", err)
	}

	// go-tfe sets BaseURL to <address>/api/v2; strip the suffix to build our own paths.
	// BaseURL() returns url.URL by value so we take its address to call String().
	baseURLRaw := tfeClient.BaseURL()
	baseURL := strings.TrimSuffix(strings.TrimRight(baseURLRaw.String(), "/"), "/api/v2")

	// Reuse the session HTTP client when available; fall back to stdlib default.
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil || httpClient == nil {
		httpClient = http.DefaultClient
	}

	// ── Branch: list all providers ────────────────────────────────────────────
	if providerNamespace == "" || providerName == "" {
		return listSupportedProviders(ctx, baseURL, orgName, token, httpClient, logger)
	}

	// ── Branch: fetch schema for a specific provider ──────────────────────────

	// If no version was given, discover it from the index endpoint first.
	if providerVersion == "" {
		discovered, err := discoverProviderVersion(ctx, baseURL, orgName, providerNamespace, providerName, token, httpClient, logger)
		if err != nil {
			return searchToolErrorf(logger, "%v", err)
		}
		providerVersion = discovered
	}

	return fetchProviderSchema(ctx, baseURL, orgName, providerNamespace, providerName, providerVersion, token, httpClient, logger)
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

func listSupportedProviders(ctx context.Context, baseURL, orgName, token string, httpClient *http.Client, logger *log.Logger) (*mcp.CallToolResult, error) {
	url := fmt.Sprintf("%s/api/v2/search/provider-versions", baseURL)
	if orgName != "" {
		url += fmt.Sprintf("?filter[organization][name]=%s", orgName)
	}

	body, err := doAuthenticatedGet(ctx, url, token, httpClient, logger)
	if err != nil {
		return searchToolErrorf(logger, "failed to fetch supported providers: %v", err)
	}

	var resp noCodeProviderVersionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return searchToolErrorf(logger, "failed to parse provider list response: %v", err)
	}

	if len(resp.Data) == 0 {
		return searchToolErrorf(logger, "no search-compatible providers found — the NO_CODE_QUERY feature flag may not be active for this organization")
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
		"note":                "Call provider_list_schema_list again with provider_namespace and provider_name (and optionally provider_version) to fetch the full list_resource_schemas for a specific provider.",
	}, "", "  ")
	if err != nil {
		return searchToolErrorf(logger, "failed to marshal provider list: %v", err)
	}

	return mcp.NewToolResultText(string(out)), nil
}

// ── discover version from index ───────────────────────────────────────────────

func discoverProviderVersion(ctx context.Context, baseURL, orgName, namespace, name, token string, httpClient *http.Client, logger *log.Logger) (string, error) {
	url := fmt.Sprintf("%s/api/v2/search/provider-versions", baseURL)
	if orgName != "" {
		url += fmt.Sprintf("?filter[organization][name]=%s", orgName)
	}

	body, err := doAuthenticatedGet(ctx, url, token, httpClient, logger)
	if err != nil {
		return "", fmt.Errorf("failed to fetch provider list to discover version: %w", err)
	}

	var resp noCodeProviderVersionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse provider list: %w", err)
	}

	for _, d := range resp.Data {
		if strings.EqualFold(d.Attributes.Namespace, namespace) && strings.EqualFold(d.Attributes.Name, name) {
			return d.Attributes.Version, nil
		}
	}

	return "", fmt.Errorf(
		"provider %s/%s is not in the search-compatible catalog — "+
			"call provider_list_schema_list without arguments to see supported providers, "+
			"or use search_providers to find a provider in the public Terraform Registry",
		namespace, name,
	)
}

// ── fetch schema for a specific provider ──────────────────────────────────────

func fetchProviderSchema(ctx context.Context, baseURL, orgName, namespace, name, version, token string, httpClient *http.Client, logger *log.Logger) (*mcp.CallToolResult, error) {
	url := fmt.Sprintf("%s/api/v2/search/provider-versions/%s/%s/%s",
		baseURL,
		namespace,
		name,
		version,
	)
	if orgName != "" {
		url += fmt.Sprintf("?filter[organization][name]=%s", orgName)
	}

	body, err := doAuthenticatedGet(ctx, url, token, httpClient, logger)
	if err != nil {
		return searchToolErrorf(logger, "failed to fetch schema for %s/%s@%s: %v", namespace, name, version, err)
	}

	var resp noCodeProviderSchemaResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return searchToolErrorf(logger, "failed to parse schema response for %s/%s@%s: %v", namespace, name, version, err)
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
				"to get a full schema guide and example configuration, then pass it with organization_name and workspace_name to create_query.",
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

// ── HTTP helpers ──────────────────────────────────────────────────────────────

// doAuthenticatedGet performs a GET with the TFE bearer token attached and
// returns the response body bytes.
func doAuthenticatedGet(ctx context.Context, rawURL, token string, httpClient *http.Client, logger *log.Logger) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.api+json")

	logger.Debugf("GET %s", rawURL)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("404 Not Found — the provider or endpoint does not exist (feature flag may be inactive)")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d — check that TFE_TOKEN has access to the no-code search endpoints", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	logger.Debugf("Response %d from %s", resp.StatusCode, rawURL)
	return body, nil
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

The tool has two modes:

LIST mode (no provider_namespace or provider_name supplied):
  Returns all providers currently in the search-compatible catalog so the agent
  can choose one. Each entry includes namespace, name, and version.

FETCH mode (provider_namespace + provider_name supplied):
  Returns the full list_resource_schemas for the requested provider, ready to
  pass directly into generate_query_configuration.
  - If provider_version is omitted, the latest version in the catalog is used.
  - If the provider is not found in the catalog, an error is returned with a hint
    to use search_providers to find the provider in the public Terraform Registry.

Typical agent workflow:
  1. Call provider_list_schema_list() with no arguments to discover available providers.
  2. Call provider_list_schema_list(provider_namespace, provider_name) to fetch the schema.
  3. Pass the returned list_resource_schemas to generate_query_configuration to get
     a full guide and example query configuration.
  4. Fill in the configuration and pass it with organization_name and workspace_name to create_query.

Requires TFE_TOKEN and TFE_ADDRESS to be configured (same credentials used for
other HCP Terraform tools). The NO_CODE_QUERY feature flag must be active for
the target organization.`
