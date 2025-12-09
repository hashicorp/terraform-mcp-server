// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"path"
	"strconv"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetProviderDocs creates a tool to get provider docs for a specific service from registry.
func GetProviderDocs(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_provider_details",
			mcp.WithDescription(`Fetches up-to-date documentation for a specific service from a Terraform provider. 
You must call 'search_providers' tool first to obtain the exact tfprovider-compatible provider_doc_id required to use this tool.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform provider documentation using a document ID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("provider_doc_id",
				mcp.Required(),
				mcp.Description("Exact tfprovider-compatible provider_doc_id, (e.g., '8894603', '8906901') retrieved from 'search_providers'")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getProviderDocsHandler(ctx, req, logger)
		},
	}
}

func getProviderDocsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	providerDocID, err := request.RequireString("provider_doc_id")
	if err != nil {
		return utils.ToolError(logger, "missing required input: provider_doc_id", err)
	}
	if providerDocID == "" {
		return utils.ToolError(logger, "provider_doc_id cannot be empty", nil)
	}
	if _, err := strconv.Atoi(providerDocID); err != nil {
		return utils.ToolError(logger, "provider_doc_id must be a valid number - use search_providers first to find valid IDs", err)
	}

	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		return utils.ToolError(logger, "failed to get http client for public Terraform registry", err)
	}

	detailResp, err := client.SendRegistryCall(httpClient, "GET", path.Join("provider-docs", providerDocID), logger, "v2")
	if err != nil {
		return utils.ToolErrorf(logger, "provider doc not found: %s - use search_providers first to find valid provider_doc_id values", providerDocID)
	}

	var details client.ProviderResourceDetails
	if err := json.Unmarshal(detailResp, &details); err != nil {
		return utils.ToolErrorf(logger, "failed to parse provider docs for %s", providerDocID)
	}

	return mcp.NewToolResultText(details.Data.Attributes.Content), nil
}
