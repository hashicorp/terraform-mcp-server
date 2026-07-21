// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetStateVersionWithID creates a tool to get the state versions for a given State Version ID.
func GetStateVersionWithID(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"get_state_version_with_id",
			mcp.WithDescription("Gets State-version for a given State-version ID"),
			mcp.WithTitleAnnotation("Gets State-Version with SV ID"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("state-version-id",
				mcp.Required(),
				mcp.Description("The State-Version id"),
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

	// init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)

	// Failed to get Terraform client
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// client context nill
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Required params
	stateVersionID, err := request.RequireString("state-version-id")
	if err != nil {
		return ToolError(logger, "missing required input: state-version-id", err)
	}
	stateVersionID = strings.TrimSpace(stateVersionID)

	// Check if input empty
	if stateVersionID == "" {
		return ToolError(logger, "state-version-id cannot be empty", nil)
	}

	// Get State Version
	sv, err := tfeClient.StateVersions.Read(ctx, stateVersionID)

	// If tool fails
	if err != nil {
		return ToolError(logger, "failed to list workspace state versions", err)
	}

	// Serialize -> Marshal JSON
	svJSON, err := json.Marshal(sv)

	if err != nil {
		return nil, fmt.Errorf("failed to serialize state version: %w", err)
	}

	return mcp.NewToolResultText(string(svJSON)), nil

}
