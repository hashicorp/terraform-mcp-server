// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetWorkspaceVariables creates the MCP tool for listing workspace variables
func GetWorkspaceVariables(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_workspace_variables",
			Description: "Fetches all variables (Terraform and environment variables) for a specific HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to list variables for (format: ws-xxxxxxxxxx)",
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
			return getWorkspaceVariablesHandler(hcpClient, request, logger)
		},
	}
}

// CreateWorkspaceVariable creates the MCP tool for creating a new workspace variable
func CreateWorkspaceVariable(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_workspace_variable",
			Description: "Creates a new variable (Terraform or environment variable) in an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create the variable in (format: ws-xxxxxxxxxx)",
					},
					"key": map[string]interface{}{
						"type":        "string",
						"description": "The name of the variable",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "The value of the variable",
					},
					"category": map[string]interface{}{
						"type":        "string",
						"description": "The category of the variable",
						"enum":        []string{"terraform", "env"},
					},
					"hcl": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the variable value is HCL (only applies to terraform variables)",
						"default":     false,
					},
					"sensitive": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the variable value is sensitive",
						"default":     false,
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Optional description for the variable",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "key", "value", "category"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createWorkspaceVariableHandler(hcpClient, request, logger)
		},
	}
}

// UpdateWorkspaceVariable creates the MCP tool for updating an existing workspace variable
func UpdateWorkspaceVariable(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "update_hcp_terraform_workspace_variable",
			Description: "Updates an existing variable in an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace containing the variable (format: ws-xxxxxxxxxx)",
					},
					"variable_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the variable to update (format: var-xxxxxxxxxx)",
					},
					"key": map[string]interface{}{
						"type":        "string",
						"description": "The updated name of the variable",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "The updated value of the variable",
					},
					"hcl": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the variable value is HCL (only applies to terraform variables)",
					},
					"sensitive": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the variable value is sensitive",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Updated description for the variable",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "variable_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return updateWorkspaceVariableHandler(hcpClient, request, logger)
		},
	}
}

// DeleteWorkspaceVariable creates the MCP tool for deleting a workspace variable
func DeleteWorkspaceVariable(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "delete_hcp_terraform_workspace_variable",
			Description: "Deletes a variable from an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace containing the variable (format: ws-xxxxxxxxxx)",
					},
					"variable_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the variable to delete (format: var-xxxxxxxxxx)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "variable_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return deleteWorkspaceVariableHandler(hcpClient, request, logger)
		},
	}
}

// BulkCreateWorkspaceVariables creates the MCP tool for creating multiple workspace variables
func BulkCreateWorkspaceVariables(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "bulk_create_hcp_terraform_workspace_variables",
			Description: "Creates multiple variables in an HCP Terraform workspace in a single request",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create variables in (format: ws-xxxxxxxxxx)",
					},
					"variables": map[string]interface{}{
						"type":        "array",
						"description": "Array of variables to create",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"key": map[string]interface{}{
									"type":        "string",
									"description": "The name of the variable",
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "The value of the variable",
								},
								"category": map[string]interface{}{
									"type":        "string",
									"description": "The category of the variable",
									"enum":        []string{"terraform", "env"},
								},
								"hcl": map[string]interface{}{
									"type":        "boolean",
									"description": "Whether the variable value is HCL (only applies to terraform variables)",
									"default":     false,
								},
								"sensitive": map[string]interface{}{
									"type":        "boolean",
									"description": "Whether the variable value is sensitive",
									"default":     false,
								},
								"description": map[string]interface{}{
									"type":        "string",
									"description": "Optional description for the variable",
								},
							},
							"required": []string{"key", "value", "category"},
						},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "variables"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return bulkCreateWorkspaceVariablesHandler(hcpClient, request, logger)
		},
	}
}

// ====================
// Handler Functions
// ====================

func getWorkspaceVariablesHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	logger.Debugf("Fetching HCP Terraform variables for workspace: %s", workspaceID)

	// Fetch variables
	response, err := hcpClient.GetWorkspaceVariables(token, workspaceID)
	if err != nil {
		logger.Errorf("Failed to fetch HCP Terraform variables: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully fetched %d variables from workspace %s", len(response.Data), workspaceID)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func createWorkspaceVariableHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	workspaceID := request.GetString("workspace_id", "")
	key := request.GetString("key", "")
	value := request.GetString("value", "")
	category := request.GetString("category", "")

	if workspaceID == "" || key == "" || category == "" {
		err := fmt.Errorf("workspace_id, key, and category are required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Build variable creation request
	createRequest := &hcp_terraform.VariableCreateRequest{
		Data: hcp_terraform.VariableCreateData{
			Type: "vars",
			Attributes: hcp_terraform.VariableCreateAttributes{
				Key:      key,
				Value:    value,
				Category: category,
			},
		},
	}

	// Add optional attributes
	if hcl := request.GetBool("hcl", false); hcl && category == "terraform" {
		createRequest.Data.Attributes.HCL = hcl
	}
	if sensitive := request.GetBool("sensitive", false); sensitive {
		createRequest.Data.Attributes.Sensitive = sensitive
	}
	if desc := request.GetString("description", ""); desc != "" {
		createRequest.Data.Attributes.Description = desc
	}

	logger.Debugf("Creating HCP Terraform variable '%s' in workspace '%s'", key, workspaceID)

	// Create variable
	response, err := hcpClient.CreateWorkspaceVariable(token, workspaceID, createRequest)
	if err != nil {
		logger.Errorf("Failed to create HCP Terraform variable: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully created variable: %s", response.Data.Attributes.Key)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func updateWorkspaceVariableHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	workspaceID := request.GetString("workspace_id", "")
	variableID := request.GetString("variable_id", "")
	if workspaceID == "" || variableID == "" {
		err := fmt.Errorf("workspace_id and variable_id are required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Build variable update request
	updateRequest := &hcp_terraform.VariableUpdateRequest{
		Data: hcp_terraform.VariableUpdateData{
			Type:       "vars",
			ID:         variableID,
			Attributes: &hcp_terraform.VariableUpdateAttributes{
				// Only include non-empty values to allow partial updates
			},
		},
	}

	hasUpdates := false

	// Add optional attributes only if provided
	if key := request.GetString("key", ""); key != "" {
		updateRequest.Data.Attributes.Key = &key
		hasUpdates = true
	}
	if value := request.GetString("value", ""); value != "" {
		updateRequest.Data.Attributes.Value = &value
		hasUpdates = true
	}
	if arguments := request.GetArguments(); arguments != nil {
		if hclRaw, exists := arguments["hcl"]; exists {
			if hcl, ok := hclRaw.(bool); ok {
				updateRequest.Data.Attributes.HCL = &hcl
				hasUpdates = true
			}
		}
		if sensitiveRaw, exists := arguments["sensitive"]; exists {
			if sensitive, ok := sensitiveRaw.(bool); ok {
				updateRequest.Data.Attributes.Sensitive = &sensitive
				hasUpdates = true
			}
		}
	}
	if desc := request.GetString("description", ""); desc != "" {
		updateRequest.Data.Attributes.Description = &desc
		hasUpdates = true
	}

	if !hasUpdates {
		err := fmt.Errorf("at least one field must be provided for update")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	logger.Debugf("Updating HCP Terraform variable: %s", variableID)

	// Update variable
	response, err := hcpClient.UpdateWorkspaceVariable(token, workspaceID, variableID, updateRequest)
	if err != nil {
		logger.Errorf("Failed to update HCP Terraform variable: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully updated variable: %s", response.Data.Attributes.Key)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func deleteWorkspaceVariableHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	workspaceID := request.GetString("workspace_id", "")
	variableID := request.GetString("variable_id", "")
	if workspaceID == "" || variableID == "" {
		err := fmt.Errorf("workspace_id and variable_id are required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	logger.Debugf("Deleting HCP Terraform variable: %s", variableID)

	// Delete variable
	err = hcpClient.DeleteWorkspaceVariable(token, workspaceID, variableID)
	if err != nil {
		logger.Errorf("Failed to delete HCP Terraform variable: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully deleted variable: %s", variableID)

	return mcp.NewToolResultText(`{"message": "Variable deleted successfully"}`), nil
}

func bulkCreateWorkspaceVariablesHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Get variables array
	arguments := request.GetArguments()
	if arguments == nil {
		err := fmt.Errorf("variables array is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	variablesRaw, exists := arguments["variables"]
	if !exists {
		err := fmt.Errorf("variables array is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	variablesArray, ok := variablesRaw.([]interface{})
	if !ok {
		err := fmt.Errorf("variables must be an array")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Build bulk variable creation request
	var variablesData []hcp_terraform.VariableCreateData
	for i, varRaw := range variablesArray {
		varMap, ok := varRaw.(map[string]interface{})
		if !ok {
			err := fmt.Errorf("variable %d must be an object", i)
			logger.Errorf("Validation failed: %v", err)
			return nil, utils.LogAndReturnError(logger, "parameter validation", err)
		}

		key := getStringFromInterface(varMap["key"], "")
		value := getStringFromInterface(varMap["value"], "")
		category := getStringFromInterface(varMap["category"], "")

		if key == "" || category == "" {
			err := fmt.Errorf("variable %d: key and category are required", i)
			logger.Errorf("Validation failed: %v", err)
			return nil, utils.LogAndReturnError(logger, "parameter validation", err)
		}

		varData := hcp_terraform.VariableCreateData{
			Type: "vars",
			Attributes: hcp_terraform.VariableCreateAttributes{
				Key:      key,
				Value:    value,
				Category: category,
			},
		}

		// Add optional attributes
		if hcl := getBoolFromInterface(varMap["hcl"], false); hcl && category == "terraform" {
			varData.Attributes.HCL = hcl
		}
		if sensitive := getBoolFromInterface(varMap["sensitive"], false); sensitive {
			varData.Attributes.Sensitive = sensitive
		}
		if desc := getStringFromInterface(varMap["description"], ""); desc != "" {
			varData.Attributes.Description = desc
		}

		variablesData = append(variablesData, varData)
	}

	logger.Debugf("Creating %d variables in workspace '%s'", len(variablesData), workspaceID)

	// Create variables
	response, err := hcpClient.BulkCreateWorkspaceVariables(token, workspaceID, variablesData)
	if err != nil {
		logger.Errorf("Failed to bulk create HCP Terraform variables: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully created %d variables in workspace %s", len(response.Data), workspaceID)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}

// getStringFromInterface safely extracts a string from an interface{} with a default value
func getStringFromInterface(value interface{}, defaultValue string) string {
	if str, ok := value.(string); ok {
		return str
	}
	return defaultValue
}

// getBoolFromInterface safely extracts a bool from an interface{} with a default value
func getBoolFromInterface(value interface{}, defaultValue bool) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return defaultValue
}
