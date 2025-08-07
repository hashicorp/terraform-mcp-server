// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
		return nil, utils.LogAndReturnError(logger, "provider_doc_id is required", err)
	}
	if providerDocID == "" {
		return nil, utils.LogAndReturnError(logger, "provider_doc_id cannot be empty", nil)
	}
	if _, err := strconv.Atoi(providerDocID); err != nil {
		return nil, utils.LogAndReturnError(logger, "provider_doc_id must be a valid number", err)
	}

	// Get a simple http client to access the public Terraform registry from context
	terraformClients, err := client.GetTerraformClientFromContext(ctx, logger)
	if err != nil {
		logger.WithError(err).Error("failed to get http client for public Terraform registry")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
	}

	httpClient := terraformClients.HttpClient

	detailResp, err := client.SendRegistryCall(httpClient, "GET", path.Join("provider-docs", providerDocID), logger, "v2")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("Error fetching provider-docs/%s, please make sure provider_doc_id is valid and the search_providers tool has run prior", providerDocID), err)
	}

	var details client.ProviderResourceDetails
	if err := json.Unmarshal(detailResp, &details); err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("error unmarshalling provider-docs/%s", providerDocID), err)
	}
	return mcp.NewToolResultText(details.Data.Attributes.Content), nil
}
