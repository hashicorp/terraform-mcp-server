package hcp_terraform

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// LockHCPTerraformWorkspaceTool locks a workspace to prevent concurrent operations
func LockHCPTerraformWorkspaceTool(client *hcp_terraform.Client, authToken, workspaceID string, reason *string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Call client method
	response, err := client.LockWorkspace(authToken, workspaceID, reason)
	if err != nil {
		return nil, fmt.Errorf("failed to lock workspace: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"workspace_id": workspaceID,
		"locked":       response.Data.Attributes.Locked,
		"locked_by":    response.Data.Attributes.LockedBy,
		"locked_at":    response.Data.Attributes.LockedAt,
		"message":      fmt.Sprintf("Successfully locked workspace %s", workspaceID),
		"status":       "success",
	}

	return result, nil
}

// UnlockHCPTerraformWorkspaceTool unlocks a workspace to allow operations
func UnlockHCPTerraformWorkspaceTool(client *hcp_terraform.Client, authToken, workspaceID string, forceUnlock *bool) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Call client method
	response, err := client.UnlockWorkspace(authToken, workspaceID, forceUnlock)
	if err != nil {
		return nil, fmt.Errorf("failed to unlock workspace: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"workspace_id": workspaceID,
		"locked":       response.Data.Attributes.Locked,
		"locked_by":    response.Data.Attributes.LockedBy,
		"locked_at":    response.Data.Attributes.LockedAt,
		"message":      fmt.Sprintf("Successfully unlocked workspace %s", workspaceID),
		"status":       "success",
	}

	return result, nil
}

// LockWorkspace creates the MCP tool for locking a workspace
func LockWorkspace(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "lock_hcp_terraform_workspace",
			Description: "Locks an HCP Terraform workspace to prevent concurrent operations",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to lock",
					},
					"reason": map[string]interface{}{
						"type":        "string",
						"description": "Optional reason for locking the workspace",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return lockWorkspaceHandler(hcpClient, request, logger)
		},
	}
}

// UnlockWorkspace creates the MCP tool for unlocking a workspace
func UnlockWorkspace(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "unlock_hcp_terraform_workspace",
			Description: "Unlocks an HCP Terraform workspace to allow operations",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to unlock",
					},
					"force_unlock": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to force unlock the workspace (use with caution)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return unlockWorkspaceHandler(hcpClient, request, logger)
		},
	}
}

// Handler implementations

func lockWorkspaceHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	workspaceID := request.GetString("workspace_id", "")
	if workspaceID == "" {
		err := fmt.Errorf("workspace_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get optional reason
	reason := request.GetString("reason", "")
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}

	// Call the tool function
	result, err := LockHCPTerraformWorkspaceTool(hcpClient, token, workspaceID, reasonPtr)
	if err != nil {
		logger.Errorf("Workspace locking failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "workspace locking", err)
	}

	// Format the response
	formattedResult := formatWorkspaceLockResponse(result, "locked")
	logger.Infof("Successfully locked workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

func unlockWorkspaceHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	workspaceID := request.GetString("workspace_id", "")
	if workspaceID == "" {
		err := fmt.Errorf("workspace_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get optional force unlock flag
	forceUnlockRaw := request.GetString("force_unlock", "")
	var forceUnlock *bool
	if forceUnlockRaw != "" {
		if forceUnlockRaw == "true" {
			force := true
			forceUnlock = &force
		} else if forceUnlockRaw == "false" {
			force := false
			forceUnlock = &force
		}
	}

	// Call the tool function
	result, err := UnlockHCPTerraformWorkspaceTool(hcpClient, token, workspaceID, forceUnlock)
	if err != nil {
		logger.Errorf("Workspace unlocking failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "workspace unlocking", err)
	}

	// Format the response
	formattedResult := formatWorkspaceLockResponse(result, "unlocked")
	logger.Infof("Successfully unlocked workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

// Formatting functions

func formatWorkspaceLockResponse(result map[string]interface{}, operation string) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	response.WriteString("### Workspace Lock Status\n")

	if workspaceID, ok := result["workspace_id"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Workspace ID**: %s\n", workspaceID))
	}

	if locked, ok := result["locked"].(bool); ok {
		lockStatus := "🔓 Unlocked"
		if locked {
			lockStatus = "🔒 Locked"
		}
		response.WriteString(fmt.Sprintf("- **Status**: %s\n", lockStatus))
	}

	if lockedBy, ok := result["locked_by"].(*string); ok && lockedBy != nil {
		response.WriteString(fmt.Sprintf("- **Locked By**: %s\n", *lockedBy))
	}

	if lockedAt, ok := result["locked_at"].(*string); ok && lockedAt != nil {
		response.WriteString(fmt.Sprintf("- **Locked At**: %s\n", *lockedAt))
	}

	if status, ok := result["status"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Operation Status**: %s\n", status))
	}

	// Add usage notes
	response.WriteString("\n### Notes\n")
	if operation == "locked" {
		response.WriteString("- The workspace is now locked and cannot be modified until unlocked\n")
		response.WriteString("- Use `unlock_hcp_terraform_workspace` to unlock when operations are complete\n")
	} else {
		response.WriteString("- The workspace is now unlocked and available for operations\n")
		response.WriteString("- Be careful when force unlocking to avoid conflicts\n")
	}

	return response.String()
}
