// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetProviderCapabilities creates a tool to get provider capabilities from registry.
func GetProviderCapabilities(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_provider_capabilities",
			mcp.WithDescription(`Get the capabilities of a Terraform provider including the types of resources, data sources, functions, guides, and other features it supports.
This tool analyzes the provider documentation to determine what types of capabilities are available:
- resources: Infrastructure resources that can be created/managed
- data-sources: Read-only data sources for querying existing infrastructure  
- functions: Provider-specific functions for data transformation
- guides: Documentation guides and tutorials for using the provider
- actions: Available provider actions (if any)
- ephemeral resources: Temporary resources for credentials and tokens
- list-resources: List resources for querying existing cloud resources (Terraform Search)

Returns a summary with counts and examples for each capability type.`),
			mcp.WithTitleAnnotation("Get Terraform provider capabilities and supported features"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("namespace",
				mcp.Required(),
				mcp.Description("The namespace of the Terraform provider, typically the name of the company, or their GitHub organization name that created the provider e.g., 'hashicorp'")),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("The name of the Terraform provider, e.g., 'aws', 'azurerm', 'google', etc.")),
			mcp.WithString("version",
				mcp.Description("The version of the provider to analyze (defaults to 'latest')")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getProviderCapabilitiesHandler(ctx, request, logger)
		},
	}
}

func getProviderCapabilitiesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "required input: namespace of the Terraform provider is required", err)
	}
	namespace = strings.ToLower(namespace)

	name, err := request.RequireString("name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "required input: name of the Terraform provider is required", err)
	}
	name = strings.ToLower(name)

	version := request.GetString("version", "latest")
	if version == "latest" || !utils.IsValidProviderVersionFormat(version) {
		// Get a simple http client to access the public Terraform registry from context
		httpClient, err := client.GetHttpClientFromContext(ctx, logger)
		if err != nil {
			logger.WithError(err).Error("failed to get http client for public Terraform registry")
			return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
		}

		latestVersion, err := client.GetLatestProviderVersion(httpClient, namespace, name, logger)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "fetching latest provider version", err)
		}
		version = latestVersion
	}

	// Get a simple http client to access the public Terraform registry from context
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		logger.WithError(err).Error("failed to get http client for public Terraform registry")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
	}

	// Get provider documentation
	uri := fmt.Sprintf("providers/%s/%s/%s", namespace, name, version)
	response, err := client.SendRegistryCall(httpClient, "GET", uri, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("fetching provider docs for %s/%s:%s", namespace, name, version), err)
	}

	var providerDocs client.ProviderDocs
	if err := json.Unmarshal(response, &providerDocs); err != nil {
		return nil, utils.LogAndReturnError(logger, "unmarshalling provider docs", err)
	}

	// Analyze and format capabilities
	output := analyzeAndFormatCapabilities(providerDocs, namespace, name, version)
	return mcp.NewToolResultText(output), nil
}

func analyzeAndFormatCapabilities(docs client.ProviderDocs, namespace, name, version string) string {
	capabilities := make(map[string][]client.ProviderDoc)

	// Analyze documentation
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

	// Show all capabilities as discovered
	for capType, items := range capabilities {
		title := strings.ReplaceAll(capType, "-", " ")
		title = cases.Title(language.English).String(title)
		builder.WriteString(fmt.Sprintf("%s: %d available\n", title, len(items)))

		// Dynamic listing: show all if ≤10, otherwise show 3 with "more" message
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
