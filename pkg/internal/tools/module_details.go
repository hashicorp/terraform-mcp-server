// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-mcp-server/pkg/internal/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func ModuleDetails(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("moduleDetails",
			mcp.WithDescription(`Fetches up-to-date documentation on how to use a Terraform module. You must call 'searchModules' first to obtain the exact valid and compatible moduleID required to use this tool.`),
			mcp.WithTitleAnnotation("Retrieve documentation for a specific Terraform module"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("moduleID",
				mcp.Required(),
				mcp.Description("Exact valid and compatible moduleID retrieved from searchModules (e.g., 'squareops/terraform-kubernetes-mongodb/mongodb/2.1.1', 'GoogleCloudPlatform/vertex-ai/google/0.2.0')"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			moduleID, err := request.RequireString("moduleID")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "moduleID is required", err)
			}
			if moduleID == "" {
				return nil, utils.LogAndReturnError(logger, "moduleID cannot be empty", nil)
			}

			var errMsg string
			response, err := utils.GetModuleDetails(registryClient, moduleID, 0, logger)
			if err != nil {
				errMsg = fmt.Sprintf("no module(s) found for %v,", moduleID)
				return nil, utils.LogAndReturnError(logger, errMsg, nil)
			}
			moduleData, err := utils.UnmarshalModuleSingular(response)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "unmarshalling module details", err)
			}
			if moduleData == "" {
				errMsg = fmt.Sprintf("getting module(s), none found! %s please provider a different moduleProvider", errMsg)
				return nil, utils.LogAndReturnError(logger, errMsg, nil)
			}
			return mcp.NewToolResultText(moduleData), nil
		}
}
