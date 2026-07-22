// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetStateVersion creates a tool to get the state versions for a given State Version ID.
func GetStateVersion(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"get_state_version",
			mcp.WithDescription("Retrieves a Terraform state version. If state_version_id is provided, retrieves that specific state version. Otherwise, retrieves the latest state version for the specified workspace. At least one of state_version_id or workspace_id must be provided."),
			mcp.WithTitleAnnotation(`Gets StateVersion with state_version_id`),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("state_version_id",
				mcp.Description("Optional StateVersion id to fetch exact version"),
			),
			mcp.WithString("workspace_id",
				mcp.Description("Optional Workspace id to fetch latest version"),
			),
		),

		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getStateVersionWithIDHandler(ctx, req, logger)
		},
	}

}

// getStateVersionWithIDHandler handles tool logics and functionality
func getStateVersionWithIDHandler(
	ctx context.Context,
	request mcp.CallToolRequest,
	logger *log.Logger) (*mcp.CallToolResult, error) {

	// Init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "Failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "Failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Both params optinoal at schema level
	stateVersionID, _ := request.RequireString("state_version_id")
	stateVersionID = strings.TrimLeft(strings.TrimSpace(stateVersionID), "#")

	workspaceID, _ := request.RequireString("workspace_id")
	workspaceID = strings.TrimLeft(strings.TrimSpace(workspaceID), "#")

	var sv *tfe.StateVersion

	if stateVersionID != "" {
		// Case 1: state_version_id provided
		sv, err = tfeClient.StateVersions.Read(ctx, stateVersionID)
		if err != nil {
			return ToolError(logger, "Failed to get state version", err)
		}

	} else if workspaceID != "" {
		// Case 2: workspace_id provided
		sv, err = tfeClient.StateVersions.ReadCurrent(ctx, workspaceID)
		if err != nil {
			return ToolError(logger, "Failed to get current state version for workspace", err)
		}

	} else {
		// Case 3: Neither provided
		return ToolError(logger, "At least one of state_version_id or workspace_id must be provided", nil)
	}

	// Marshal JSON
	svJSON, err := json.Marshal(sv)
	if err != nil {
		return ToolError(logger, "Failed to serialize state version", err)
	}

	return mcp.NewToolResultText(string(svJSON)), nil
}
