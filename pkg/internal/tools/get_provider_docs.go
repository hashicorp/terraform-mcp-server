// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-mcp-server/pkg/internal/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/internal/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetProviderDocs creates a tool to get provider docs for a specific service from registry.
func GetProviderDocs(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("getProviderDocs",
			mcp.WithDescription(`Fetches up-to-date documentation for a specific service from a Terraform provider. You must call 'resolveProviderDocID' first to obtain the exact tfprovider-compatible providerDocID required to use this tool.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform provider documentation using a document ID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("providerDocID", mcp.Required(), mcp.Description("Exact tfprovider-compatible providerDocID, (e.g., '8894603', '8906901') retrieved from 'resolveProviderDocID'")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			providerDocID, err := request.RequireString("providerDocID")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "providerDocID is required", err)
			}
			if providerDocID == "" {
				return nil, utils.LogAndReturnError(logger, "providerDocID cannot be empty", nil)
			}

			detailResp, err := utils.SendRegistryCall(registryClient, "GET", fmt.Sprintf("provider-docs/%s", providerDocID), logger, "v2")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf("Error fetching provider-docs/%s, please make sure providerDocID is valid and the resolveProviderDocID tool has run prior", providerDocID), err)
			}

			var details client.ProviderResourceDetails
			if err := json.Unmarshal(detailResp, &details); err != nil {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf("error unmarshalling provider-docs/%s", providerDocID), err)
			}
			return mcp.NewToolResultText(details.Data.Attributes.Content), nil
		}
}
