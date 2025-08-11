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

// GetHCPTerraformRemoteStateConsumersTool retrieves workspaces that can access this workspace's state
func GetHCPTerraformRemoteStateConsumersTool(client *hcp_terraform.Client, authToken, workspaceID string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Call client method
	response, err := client.GetWorkspaceRemoteStateConsumers(authToken, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote state consumers: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"workspace_id":           workspaceID,
		"remote_state_consumers": response.Data,
		"total_count":            len(response.Data),
		"message":                fmt.Sprintf("Retrieved %d remote state consumers for workspace %s", len(response.Data), workspaceID),
	}

	return result, nil
}

// AddHCPTerraformRemoteStateConsumersTool adds workspaces as remote state consumers
func AddHCPTerraformRemoteStateConsumersTool(client *hcp_terraform.Client, authToken, workspaceID string, consumerWorkspaceIDs []string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if len(consumerWorkspaceIDs) == 0 {
		return nil, fmt.Errorf("consumer_workspace_ids is required and must not be empty")
	}

	// Call client method
	err := client.AddWorkspaceRemoteStateConsumers(authToken, workspaceID, consumerWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to add remote state consumers: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"workspace_id":       workspaceID,
		"added_consumer_ids": consumerWorkspaceIDs,
		"added_count":        len(consumerWorkspaceIDs),
		"message":            fmt.Sprintf("Added %d remote state consumers to workspace %s", len(consumerWorkspaceIDs), workspaceID),
		"status":             "success",
	}

	return result, nil
}

// RemoveHCPTerraformRemoteStateConsumersTool removes workspaces as remote state consumers
func RemoveHCPTerraformRemoteStateConsumersTool(client *hcp_terraform.Client, authToken, workspaceID string, consumerWorkspaceIDs []string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if len(consumerWorkspaceIDs) == 0 {
		return nil, fmt.Errorf("consumer_workspace_ids is required and must not be empty")
	}

	// Call client method
	err := client.RemoveWorkspaceRemoteStateConsumers(authToken, workspaceID, consumerWorkspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to remove remote state consumers: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"workspace_id":         workspaceID,
		"removed_consumer_ids": consumerWorkspaceIDs,
		"removed_count":        len(consumerWorkspaceIDs),
		"message":              fmt.Sprintf("Removed %d remote state consumers from workspace %s", len(consumerWorkspaceIDs), workspaceID),
		"status":               "success",
	}

	return result, nil
}

// GetRemoteStateConsumers creates the MCP tool for getting remote state consumers
func GetRemoteStateConsumers(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_remote_state_consumers",
			Description: "Fetches workspaces that can access this workspace's remote state",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to get remote state consumers for",
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
			return getRemoteStateConsumersHandler(hcpClient, request, logger)
		},
	}
}

// AddRemoteStateConsumers creates the MCP tool for adding remote state consumers
func AddRemoteStateConsumers(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "add_hcp_terraform_remote_state_consumers",
			Description: "Adds workspaces as remote state consumers, allowing them to access this workspace's state",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace that will share its state",
					},
					"consumer_workspace_ids": map[string]interface{}{
						"type":        "array",
						"description": "Array of workspace IDs that should be granted access to the state",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "consumer_workspace_ids"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return addRemoteStateConsumersHandler(hcpClient, request, logger)
		},
	}
}

// RemoveRemoteStateConsumers creates the MCP tool for removing remote state consumers
func RemoveRemoteStateConsumers(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "remove_hcp_terraform_remote_state_consumers",
			Description: "Removes workspaces as remote state consumers, revoking their access to this workspace's state",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace that will stop sharing its state",
					},
					"consumer_workspace_ids": map[string]interface{}{
						"type":        "array",
						"description": "Array of workspace IDs that should have their access revoked",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "consumer_workspace_ids"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return removeRemoteStateConsumersHandler(hcpClient, request, logger)
		},
	}
}

// Handler implementations

func getRemoteStateConsumersHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Call the tool function
	result, err := GetHCPTerraformRemoteStateConsumersTool(hcpClient, token, workspaceID)
	if err != nil {
		logger.Errorf("Remote state consumers retrieval failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "remote state consumers retrieval", err)
	}

	// Format the response
	formattedResult := formatRemoteStateConsumersResponse(result)
	logger.Infof("Successfully retrieved remote state consumers for workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

func addRemoteStateConsumersHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	consumerWorkspaceIDs := request.GetStringSlice("consumer_workspace_ids", []string{})
	if len(consumerWorkspaceIDs) == 0 {
		err := fmt.Errorf("consumer_workspace_ids is required and must not be empty")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := AddHCPTerraformRemoteStateConsumersTool(hcpClient, token, workspaceID, consumerWorkspaceIDs)
	if err != nil {
		logger.Errorf("Remote state consumers addition failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "remote state consumers addition", err)
	}

	// Format the response
	formattedResult := formatAddRemoteStateConsumersResponse(result)
	logger.Infof("Successfully added remote state consumers to workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

func removeRemoteStateConsumersHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	consumerWorkspaceIDs := request.GetStringSlice("consumer_workspace_ids", []string{})
	if len(consumerWorkspaceIDs) == 0 {
		err := fmt.Errorf("consumer_workspace_ids is required and must not be empty")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := RemoveHCPTerraformRemoteStateConsumersTool(hcpClient, token, workspaceID, consumerWorkspaceIDs)
	if err != nil {
		logger.Errorf("Remote state consumers removal failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "remote state consumers removal", err)
	}

	// Format the response
	formattedResult := formatRemoveRemoteStateConsumersResponse(result)
	logger.Infof("Successfully removed remote state consumers from workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

// Formatting functions

func formatRemoteStateConsumersResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	if consumers, ok := result["remote_state_consumers"].([]hcp_terraform.RemoteStateConsumer); ok {
		if len(consumers) == 0 {
			response.WriteString("No remote state consumers found for this workspace.\n")
		} else {
			response.WriteString("### Remote State Consumers\n")
			for i, consumer := range consumers {
				response.WriteString(fmt.Sprintf("%d. **%s** (ID: %s)\n", i+1, consumer.Attributes.Name, consumer.ID))
				if consumer.Attributes.Description != nil && *consumer.Attributes.Description != "" {
					response.WriteString(fmt.Sprintf("   - Description: %s\n", *consumer.Attributes.Description))
				}
				if consumer.Attributes.Environment != nil && *consumer.Attributes.Environment != "" {
					response.WriteString(fmt.Sprintf("   - Environment: %s\n", *consumer.Attributes.Environment))
				}
				lockStatus := "🔓 Unlocked"
				if consumer.Attributes.Locked {
					lockStatus = "🔒 Locked"
				}
				response.WriteString(fmt.Sprintf("   - Status: %s\n", lockStatus))
			}
		}
	}

	if totalCount, ok := result["total_count"].(int); ok {
		response.WriteString(fmt.Sprintf("\n**Total Count**: %d\n", totalCount))
	}

	if workspaceID, ok := result["workspace_id"].(string); ok {
		response.WriteString(fmt.Sprintf("**Source Workspace**: %s\n", workspaceID))
	}

	return response.String()
}

func formatAddRemoteStateConsumersResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	response.WriteString("### Addition Details\n")

	if workspaceID, ok := result["workspace_id"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Source Workspace**: %s\n", workspaceID))
	}

	if addedCount, ok := result["added_count"].(int); ok {
		response.WriteString(fmt.Sprintf("- **Added Count**: %d\n", addedCount))
	}

	if addedIDs, ok := result["added_consumer_ids"].([]string); ok {
		response.WriteString("- **Added Consumer IDs**:\n")
		for i, id := range addedIDs {
			response.WriteString(fmt.Sprintf("  %d. %s\n", i+1, id))
		}
	}

	if status, ok := result["status"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Status**: %s\n", status))
	}

	response.WriteString("\n### Notes\n")
	response.WriteString("- The specified workspaces can now access this workspace's remote state\n")
	response.WriteString("- Use `terraform_remote_state` data source to access the state in consumer workspaces\n")

	return response.String()
}

func formatRemoveRemoteStateConsumersResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	response.WriteString("### Removal Details\n")

	if workspaceID, ok := result["workspace_id"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Source Workspace**: %s\n", workspaceID))
	}

	if removedCount, ok := result["removed_count"].(int); ok {
		response.WriteString(fmt.Sprintf("- **Removed Count**: %d\n", removedCount))
	}

	if removedIDs, ok := result["removed_consumer_ids"].([]string); ok {
		response.WriteString("- **Removed Consumer IDs**:\n")
		for i, id := range removedIDs {
			response.WriteString(fmt.Sprintf("  %d. %s\n", i+1, id))
		}
	}

	if status, ok := result["status"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Status**: %s\n", status))
	}

	response.WriteString("\n### Notes\n")
	response.WriteString("- The specified workspaces no longer have access to this workspace's remote state\n")
	response.WriteString("- Any `terraform_remote_state` data sources in consumer workspaces will fail until access is restored\n")

	return response.String()
}
