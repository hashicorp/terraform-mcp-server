// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetProviderDocs creates a tool to get provider docs for a specific service from registry.
func GetProviderDocs(registryClient *http.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_provider_docs",
			mcp.WithDescription(`Fetches up-to-date documentation for a specific service from a Terraform provider. 
You must call 'resolve_provider_doc_id' tool first to obtain the exact tfprovider-compatible provider_doc_id required to use this tool.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform provider documentation using a document ID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("provider_doc_id",
				mcp.Required(),
				mcp.Description("Exact tfprovider-compatible provider_doc_id, (e.g., '8894603', '8906901') retrieved from 'resolve_provider_doc_id'")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getProviderDocsHandler(registryClient, req, logger)
		},
	}
}

func getProviderDocsHandler(registryClient *http.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	detailResp, err := client.SendRegistryCall(registryClient, "GET", fmt.Sprintf("provider-docs/%s", providerDocID), logger, "v2")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("Error fetching provider-docs/%s, please make sure provider_doc_id is valid and the resolve_provider_doc_id tool has run prior", providerDocID), err)
	}

	var details client.ProviderResourceDetails
	if err := json.Unmarshal(detailResp, &details); err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("error unmarshalling provider-docs/%s", providerDocID), err)
	}
	return mcp.NewToolResultText(details.Data.Attributes.Content), nil
}
