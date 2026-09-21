// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type GetProviderCapabilitiesArguments struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
}

func GetProviderCapabilitiesTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "get_provider_capabilities",
		Description: `Get the capabilities of a Terraform provider including the types of resources, data sources, functions, guides, and other features it supports.
This tool analyzes the provider documentation to determine what types of capabilities are available:
- resources: Infrastructure resources that can be created/managed
- data-sources: Read-only data sources for querying existing infrastructure  
- functions: Provider-specific functions for data transformation
- guides: Documentation guides and tutorials for using the provider
- actions: Available provider actions (if any)
- ephemeral resources: Temporary resources for credentials and tokens
- list-resources: List resources for querying existing cloud resources (Terraform Search)

Returns a summary with counts and examples for each capability type.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Terraform provider capabilities and supported features",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "The namespace of the Terraform provider, typically the company or GitHub organization that created it, e.g. hashicorp",
				},
				"name": {
					Type:        "string",
					Description: "The name of the Terraform provider, e.g. aws, azurerm, or google",
				},
				"version": {
					Type:        "string",
					Description: "The version of the provider to analyze (defaults to latest)",
					Default:     json.RawMessage(`"latest"`),
				},
			},
			Required: []string{"namespace", "name"},
		},
	}
}

func GetProviderCapabilitiesFunc(ctx context.Context, request *mcp.CallToolRequest, input GetProviderCapabilitiesArguments) (*mcp.CallToolResult, any, error) {
	// TODO: Replace with structured slog logging
	logger := log.StandardLogger()

	namespace := strings.TrimSpace(input.Namespace)
	if namespace == "" {
		return nil, nil, fmt.Errorf("missing required input: namespace")
	}
	namespace = strings.ToLower(namespace)

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, nil, fmt.Errorf("missing required input: name")
	}
	name = strings.ToLower(name)

	version := strings.ToLower(strings.TrimSpace(input.Version))
	if version == "latest" || !utils.IsValidProviderVersionFormat(version) {
		httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
		}

		latestVersion, err := registryapi.GetLatestProviderVersion(ctx, httpClient, namespace, name, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("provider not found: %s/%s - verify the namespace and provider name are correct", namespace, name)
		}
		version = latestVersion
	}

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	uri := fmt.Sprintf("providers/%s/%s/%s", namespace, name, version)
	response, err := registryapi.SendRegistryCall(ctx, httpClient, "GET", uri, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch provider docs for %s/%s:%s - verify the provider exists", namespace, name, version)
	}

	var providerDocs registryapi.ProviderDocs
	if err := json.Unmarshal(response, &providerDocs); err != nil {
		return nil, nil, fmt.Errorf("failed to parse provider docs for %s/%s:%s", namespace, name, version)
	}

	output := analyzeAndFormatCapabilities(providerDocs, namespace, name, version)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: output}}}, nil, nil
}

func analyzeAndFormatCapabilities(docs registryapi.ProviderDocs, namespace, name, version string) string {
	capabilities := make(map[string][]registryapi.ProviderDoc)

	for _, doc := range docs.Docs {
		if doc.Language != "hcl" {
			continue
		}

		category := strings.ToLower(doc.Category)
		capabilities[category] = append(capabilities[category], doc)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Provider Capabilities: %s/%s (v%s)\n\n", namespace, name, version))

	if len(capabilities) == 0 {
		builder.WriteString("No capabilities found for this provider.\n")
		return builder.String()
	}

	for capType, items := range capabilities {
		title := strings.ReplaceAll(capType, "-", " ")
		title = cases.Title(language.English).String(title)
		builder.WriteString(fmt.Sprintf("%s: %d available\n", title, len(items)))

		limit := 3
		if len(items) <= 10 {
			limit = len(items)
		}

		for i, item := range items {
			if i >= limit {
				builder.WriteString(fmt.Sprintf("  ... and %d more\n", len(items)-limit))
				break
			}
			builder.WriteString(fmt.Sprintf("  - %s (provider_doc_id: %s)\n", item.Title, item.ID))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
