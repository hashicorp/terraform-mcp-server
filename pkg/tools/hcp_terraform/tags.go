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

// GetHCPTerraformWorkspaceTagsTool retrieves tag bindings for a workspace
func GetHCPTerraformWorkspaceTagsTool(client *hcp_terraform.Client, authToken, workspaceID string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Call client method
	response, err := client.GetWorkspaceTagBindings(authToken, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace tag bindings: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"tag_bindings": response.Data,
		"total_count":  len(response.Data),
		"message":      fmt.Sprintf("Retrieved %d tag bindings for workspace %s", len(response.Data), workspaceID),
	}

	return result, nil
}

// CreateHCPTerraformWorkspaceTagBindingsTool creates tag bindings for a workspace
func CreateHCPTerraformWorkspaceTagBindingsTool(client *hcp_terraform.Client, authToken, workspaceID string, tagBindings []map[string]string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if len(tagBindings) == 0 {
		return nil, fmt.Errorf("tag_bindings is required and must not be empty")
	}

	// Build tag binding create data
	var createData []hcp_terraform.TagBindingCreateData
	for _, binding := range tagBindings {
		key, keyExists := binding["key"]
		value, valueExists := binding["value"]

		if !keyExists || key == "" {
			return nil, fmt.Errorf("each tag binding must have a 'key' field")
		}
		if !valueExists {
			value = "" // Allow empty values
		}

		createData = append(createData, hcp_terraform.TagBindingCreateData{
			Type: "tag-bindings",
			Attributes: hcp_terraform.TagBindingCreateAttributes{
				Key:   key,
				Value: value,
			},
		})
	}

	// Create request
	request := &hcp_terraform.TagBindingCreateRequest{
		Data: createData,
	}

	// Call client method
	response, err := client.CreateWorkspaceTagBindings(authToken, workspaceID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace tag bindings: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"tag_bindings":  response.Data,
		"created_count": len(response.Data),
		"message":       fmt.Sprintf("Created %d tag bindings for workspace %s", len(response.Data), workspaceID),
	}

	return result, nil
}

// UpdateHCPTerraformWorkspaceTagBindingsTool updates existing tag bindings for a workspace
func UpdateHCPTerraformWorkspaceTagBindingsTool(client *hcp_terraform.Client, authToken, workspaceID string, tagUpdates []map[string]string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if len(tagUpdates) == 0 {
		return nil, fmt.Errorf("tag_updates is required and must not be empty")
	}

	// Build tag binding update data
	var updateData []hcp_terraform.TagBindingUpdateData
	for _, update := range tagUpdates {
		id, idExists := update["id"]
		value, valueExists := update["value"]

		if !idExists || id == "" {
			return nil, fmt.Errorf("each tag update must have an 'id' field")
		}
		if !valueExists {
			return nil, fmt.Errorf("each tag update must have a 'value' field")
		}

		updateData = append(updateData, hcp_terraform.TagBindingUpdateData{
			ID:   id,
			Type: "tag-bindings",
			Attributes: hcp_terraform.TagBindingUpdateAttributes{
				Value: value,
			},
		})
	}

	// Create request
	request := &hcp_terraform.TagBindingUpdateRequest{
		Data: updateData,
	}

	// Call client method
	response, err := client.UpdateWorkspaceTagBindings(authToken, workspaceID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to update workspace tag bindings: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"tag_bindings":  response.Data,
		"updated_count": len(response.Data),
		"message":       fmt.Sprintf("Updated %d tag bindings for workspace %s", len(response.Data), workspaceID),
	}

	return result, nil
}

// DeleteHCPTerraformWorkspaceTagsTool deletes tag bindings from a workspace
func DeleteHCPTerraformWorkspaceTagsTool(client *hcp_terraform.Client, authToken, workspaceID string, tagBindingIDs []string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if len(tagBindingIDs) == 0 {
		return nil, fmt.Errorf("tag_binding_ids is required and must not be empty")
	}

	// Call client method
	err := client.DeleteWorkspaceTagBindings(authToken, workspaceID, tagBindingIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to delete workspace tag bindings: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"deleted_count": len(tagBindingIDs),
		"deleted_ids":   tagBindingIDs,
		"message":       fmt.Sprintf("Deleted %d tag bindings from workspace %s", len(tagBindingIDs), workspaceID),
		"status":        "success",
	}

	return result, nil
}

// GetWorkspaceTags creates the MCP tool for getting workspace tag bindings
func GetWorkspaceTags(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_workspace_tags",
			Description: "Fetches tag bindings for an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to get tag bindings for",
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
			return getWorkspaceTagsHandler(hcpClient, request, logger)
		},
	}
}

// CreateWorkspaceTagBindings creates the MCP tool for creating workspace tag bindings
func CreateWorkspaceTagBindings(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_workspace_tag_bindings",
			Description: "Creates tag bindings for an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create tag bindings for",
					},
					"tag_bindings": map[string]interface{}{
						"type":        "array",
						"description": "Array of tag bindings to create, each with 'key' and 'value' fields",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"key": map[string]interface{}{
									"type":        "string",
									"description": "The tag key",
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "The tag value",
								},
							},
							"required": []string{"key", "value"},
						},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "tag_bindings"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createWorkspaceTagBindingsHandler(hcpClient, request, logger)
		},
	}
}

// UpdateWorkspaceTagBindings creates the MCP tool for updating workspace tag bindings
func UpdateWorkspaceTagBindings(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "update_hcp_terraform_workspace_tag_bindings",
			Description: "Updates existing tag bindings for an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to update tag bindings for",
					},
					"tag_updates": map[string]interface{}{
						"type":        "array",
						"description": "Array of tag binding updates, each with 'id' and 'value' fields",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id": map[string]interface{}{
									"type":        "string",
									"description": "The tag binding ID to update",
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "The new tag value",
								},
							},
							"required": []string{"id", "value"},
						},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "tag_updates"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return updateWorkspaceTagBindingsHandler(hcpClient, request, logger)
		},
	}
}

// DeleteWorkspaceTags creates the MCP tool for deleting workspace tag bindings
func DeleteWorkspaceTags(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "delete_hcp_terraform_workspace_tags",
			Description: "Deletes tag bindings from an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to delete tag bindings from",
					},
					"tag_binding_ids": map[string]interface{}{
						"type":        "array",
						"description": "Array of tag binding IDs to delete",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "tag_binding_ids"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return deleteWorkspaceTagsHandler(hcpClient, request, logger)
		},
	}
}

// Handler implementations

func getWorkspaceTagsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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
	result, err := GetHCPTerraformWorkspaceTagsTool(hcpClient, token, workspaceID)
	if err != nil {
		logger.Errorf("Workspace tags retrieval failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "workspace tags retrieval", err)
	}

	// Format the response
	formattedResult := formatWorkspaceTagsResponse(result)
	logger.Infof("Successfully retrieved tag bindings for workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

func createWorkspaceTagBindingsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Parse tag bindings array
	var tagBindings []map[string]string
	if arguments := request.GetArguments(); arguments != nil {
		if tagBindingsRaw, exists := arguments["tag_bindings"]; exists {
			if tagBindingsArray, ok := tagBindingsRaw.([]interface{}); ok {
				for i, item := range tagBindingsArray {
					if itemMap, ok := item.(map[string]interface{}); ok {
						binding := make(map[string]string)
						key, hasKey := itemMap["key"].(string)
						value, hasValue := itemMap["value"].(string)

						if !hasKey || key == "" {
							err := fmt.Errorf("tag binding %d: key is required", i)
							logger.Errorf("Validation failed: %v", err)
							return nil, utils.LogAndReturnError(logger, "parameter validation", err)
						}
						if !hasValue {
							value = "" // Allow empty values
						}

						binding["key"] = key
						binding["value"] = value
						tagBindings = append(tagBindings, binding)
					} else {
						err := fmt.Errorf("tag binding %d: must be an object with 'key' and 'value' fields", i)
						logger.Errorf("Validation failed: %v", err)
						return nil, utils.LogAndReturnError(logger, "parameter validation", err)
					}
				}
			} else {
				err := fmt.Errorf("tag_bindings must be an array of objects")
				logger.Errorf("Validation failed: %v", err)
				return nil, utils.LogAndReturnError(logger, "parameter validation", err)
			}
		}
	}

	if len(tagBindings) == 0 {
		err := fmt.Errorf("tag_bindings is required and must not be empty")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := CreateHCPTerraformWorkspaceTagBindingsTool(hcpClient, token, workspaceID, tagBindings)
	if err != nil {
		logger.Errorf("Workspace tag bindings creation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "workspace tag bindings creation", err)
	}

	// Format the response
	formattedResult := formatCreateTagBindingsResponse(result)
	logger.Infof("Successfully created tag bindings for workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

func updateWorkspaceTagBindingsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Parse tag updates array
	var tagUpdates []map[string]string
	if arguments := request.GetArguments(); arguments != nil {
		if tagUpdatesRaw, exists := arguments["tag_updates"]; exists {
			if tagUpdatesArray, ok := tagUpdatesRaw.([]interface{}); ok {
				for i, item := range tagUpdatesArray {
					if itemMap, ok := item.(map[string]interface{}); ok {
						update := make(map[string]string)
						id, hasID := itemMap["id"].(string)
						value, hasValue := itemMap["value"].(string)

						if !hasID || id == "" {
							err := fmt.Errorf("tag update %d: id is required", i)
							logger.Errorf("Validation failed: %v", err)
							return nil, utils.LogAndReturnError(logger, "parameter validation", err)
						}
						if !hasValue {
							err := fmt.Errorf("tag update %d: value is required", i)
							logger.Errorf("Validation failed: %v", err)
							return nil, utils.LogAndReturnError(logger, "parameter validation", err)
						}

						update["id"] = id
						update["value"] = value
						tagUpdates = append(tagUpdates, update)
					} else {
						err := fmt.Errorf("tag update %d: must be an object with 'id' and 'value' fields", i)
						logger.Errorf("Validation failed: %v", err)
						return nil, utils.LogAndReturnError(logger, "parameter validation", err)
					}
				}
			} else {
				err := fmt.Errorf("tag_updates must be an array of objects")
				logger.Errorf("Validation failed: %v", err)
				return nil, utils.LogAndReturnError(logger, "parameter validation", err)
			}
		}
	}

	if len(tagUpdates) == 0 {
		err := fmt.Errorf("tag_updates is required and must not be empty")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := UpdateHCPTerraformWorkspaceTagBindingsTool(hcpClient, token, workspaceID, tagUpdates)
	if err != nil {
		logger.Errorf("Workspace tag bindings update failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "workspace tag bindings update", err)
	}

	// Format the response
	formattedResult := formatUpdateTagBindingsResponse(result)
	logger.Infof("Successfully updated tag bindings for workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

func deleteWorkspaceTagsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	tagBindingIDs := request.GetStringSlice("tag_binding_ids", []string{})
	if len(tagBindingIDs) == 0 {
		err := fmt.Errorf("tag_binding_ids is required and must not be empty")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := DeleteHCPTerraformWorkspaceTagsTool(hcpClient, token, workspaceID, tagBindingIDs)
	if err != nil {
		logger.Errorf("Workspace tags deletion failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "workspace tags deletion", err)
	}

	// Format the response
	formattedResult := formatDeleteTagsResponse(result)
	logger.Infof("Successfully deleted tag bindings from workspace %s", workspaceID)

	return mcp.NewToolResultText(formattedResult), nil
}

// Formatting functions

func formatWorkspaceTagsResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	if tagBindings, ok := result["tag_bindings"].([]hcp_terraform.TagBinding); ok {
		if len(tagBindings) == 0 {
			response.WriteString("No tag bindings found for this workspace.\n")
		} else {
			response.WriteString("### Tag Bindings\n")
			for i, binding := range tagBindings {
				response.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, binding.Attributes.Key, binding.Attributes.Value))
				response.WriteString(fmt.Sprintf("   - ID: %s\n", binding.ID))
			}
		}
	}

	if totalCount, ok := result["total_count"].(int); ok {
		response.WriteString(fmt.Sprintf("\n**Total Count**: %d\n", totalCount))
	}

	return response.String()
}

func formatCreateTagBindingsResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	if tagBindings, ok := result["tag_bindings"].([]hcp_terraform.TagBinding); ok {
		response.WriteString("### Created Tag Bindings\n")
		for i, binding := range tagBindings {
			response.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, binding.Attributes.Key, binding.Attributes.Value))
			response.WriteString(fmt.Sprintf("   - ID: %s\n", binding.ID))
		}
	}

	if createdCount, ok := result["created_count"].(int); ok {
		response.WriteString(fmt.Sprintf("\n**Created Count**: %d\n", createdCount))
	}

	return response.String()
}

func formatUpdateTagBindingsResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	if tagBindings, ok := result["tag_bindings"].([]hcp_terraform.TagBinding); ok {
		response.WriteString("### Updated Tag Bindings\n")
		for i, binding := range tagBindings {
			response.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, binding.Attributes.Key, binding.Attributes.Value))
			response.WriteString(fmt.Sprintf("   - ID: %s\n", binding.ID))
		}
	}

	if updatedCount, ok := result["updated_count"].(int); ok {
		response.WriteString(fmt.Sprintf("\n**Updated Count**: %d\n", updatedCount))
	}

	return response.String()
}

func formatDeleteTagsResponse(result map[string]interface{}) string {
	var response strings.Builder

	if message, ok := result["message"].(string); ok {
		response.WriteString(fmt.Sprintf("✅ %s\n\n", message))
	}

	response.WriteString("### Deletion Details\n")

	if deletedCount, ok := result["deleted_count"].(int); ok {
		response.WriteString(fmt.Sprintf("- **Deleted Count**: %d\n", deletedCount))
	}

	if deletedIDs, ok := result["deleted_ids"].([]string); ok {
		response.WriteString("- **Deleted IDs**:\n")
		for i, id := range deletedIDs {
			response.WriteString(fmt.Sprintf("  %d. %s\n", i+1, id))
		}
	}

	if status, ok := result["status"].(string); ok {
		response.WriteString(fmt.Sprintf("- **Status**: %s\n", status))
	}

	return response.String()
}
