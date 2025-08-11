// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

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
						"default":     "",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Optional description of the variable",
					},
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Whether this is a Terraform or environment variable",
						"enum":        []string{"terraform", "env"},
					},
					"hcl": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to evaluate the value as HCL code (only applies to Terraform variables)",
						"default":     false,
					},
					"sensitive": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the value is sensitive (write-only after creation)",
						"default":     false,
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "key", "category"},
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
						"description": "The ID of the workspace that owns the variable (format: ws-xxxxxxxxxx)",
					},
					"variable_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the variable to update (format: var-xxxxxxxxxx)",
					},
					"key": map[string]interface{}{
						"type":        "string",
						"description": "Optional new name for the variable",
					},
					"value": map[string]interface{}{
						"type":        "string",
						"description": "Optional new value for the variable",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Optional new description for the variable",
					},
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Optional new category for the variable",
						"enum":        []string{"terraform", "env"},
					},
					"hcl": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to evaluate the value as HCL code (only applies to Terraform variables)",
					},
					"sensitive": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the value is sensitive (write-only after creation)",
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
						"description": "The ID of the workspace that owns the variable (format: ws-xxxxxxxxxx)",
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

// BulkCreateWorkspaceVariables creates the MCP tool for creating multiple workspace variables at once
func BulkCreateWorkspaceVariables(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "bulk_create_hcp_terraform_workspace_variables",
			Description: "Creates multiple variables in an HCP Terraform workspace efficiently in a single API call",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create variables in (format: ws-xxxxxxxxxx)",
					},
					"variables": map[string]interface{}{
						"type":        "array",
						"description": "Array of variable configuration objects",
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
									"default":     "",
								},
								"description": map[string]interface{}{
									"type":        "string",
									"description": "Optional description of the variable",
								},
								"category": map[string]interface{}{
									"type":        "string",
									"description": "Whether this is a Terraform or environment variable",
									"enum":        []string{"terraform", "env"},
								},
								"hcl": map[string]interface{}{
									"type":        "boolean",
									"description": "Whether to evaluate the value as HCL code (only applies to Terraform variables)",
									"default":     false,
								},
								"sensitive": map[string]interface{}{
									"type":        "boolean",
									"description": "Whether the value is sensitive (write-only after creation)",
									"default":     false,
								},
							},
							"required": []string{"key", "category"},
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

	// Format response
	result := formatVariablesResponse(response)
	return mcp.NewToolResultText(result), nil
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
				Key:       key,
				Value:     request.GetString("value", ""),
				Category:  category,
				HCL:       request.GetBool("hcl", false),
				Sensitive: request.GetBool("sensitive", false),
			},
		},
	}

	// Add optional description
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

	// Format response
	result := formatVariableDetailsResponse(response)
	return mcp.NewToolResultText(result), nil
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
			ID:         variableID,
			Type:       "vars",
			Attributes: &hcp_terraform.VariableUpdateAttributes{},
		},
	}

	// Handle optional parameters using GetArguments for proper optional checking
	if arguments := request.GetArguments(); arguments != nil {
		if keyRaw, exists := arguments["key"]; exists {
			if key, ok := keyRaw.(string); ok {
				updateRequest.Data.Attributes.Key = &key
			}
		}
		if valueRaw, exists := arguments["value"]; exists {
			if value, ok := valueRaw.(string); ok {
				updateRequest.Data.Attributes.Value = &value
			}
		}
		if descRaw, exists := arguments["description"]; exists {
			if desc, ok := descRaw.(string); ok {
				updateRequest.Data.Attributes.Description = &desc
			}
		}
		if categoryRaw, exists := arguments["category"]; exists {
			if category, ok := categoryRaw.(string); ok {
				updateRequest.Data.Attributes.Category = &category
			}
		}
		if hclRaw, exists := arguments["hcl"]; exists {
			if hcl, ok := hclRaw.(bool); ok {
				updateRequest.Data.Attributes.HCL = &hcl
			}
		}
		if sensitiveRaw, exists := arguments["sensitive"]; exists {
			if sensitive, ok := sensitiveRaw.(bool); ok {
				updateRequest.Data.Attributes.Sensitive = &sensitive
			}
		}
	}

	logger.Debugf("Updating HCP Terraform variable %s in workspace %s", variableID, workspaceID)

	// Update variable
	response, err := hcpClient.UpdateWorkspaceVariable(token, workspaceID, variableID, updateRequest)
	if err != nil {
		logger.Errorf("Failed to update HCP Terraform variable: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully updated variable: %s", response.Data.Attributes.Key)

	// Format response
	result := formatVariableDetailsResponse(response)
	return mcp.NewToolResultText(result), nil
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

	logger.Debugf("Deleting HCP Terraform variable %s from workspace %s", variableID, workspaceID)

	// Delete variable
	err = hcpClient.DeleteWorkspaceVariable(token, workspaceID, variableID)
	if err != nil {
		logger.Errorf("Failed to delete HCP Terraform variable: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully deleted variable %s from workspace %s", variableID, workspaceID)

	// Format success response
	result := fmt.Sprintf("Successfully deleted variable %s from workspace %s", variableID, workspaceID)
	return mcp.NewToolResultText(result), nil
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

	// Parse variables array
	var variables []hcp_terraform.VariableCreateData
	if arguments := request.GetArguments(); arguments != nil {
		if variablesRaw, exists := arguments["variables"]; exists {
			if variablesArray, ok := variablesRaw.([]interface{}); ok {
				for i, varRaw := range variablesArray {
					if varMap, ok := varRaw.(map[string]interface{}); ok {
						// Extract required fields
						key, hasKey := varMap["key"].(string)
						category, hasCategory := varMap["category"].(string)

						if !hasKey || !hasCategory {
							err := fmt.Errorf("variable %d: key and category are required", i)
							logger.Errorf("Validation failed: %v", err)
							return nil, utils.LogAndReturnError(logger, "parameter validation", err)
						}

						// Build variable data
						varData := hcp_terraform.VariableCreateData{
							Type: "vars",
							Attributes: hcp_terraform.VariableCreateAttributes{
								Key:       key,
								Value:     getStringFromInterface(varMap["value"], ""),
								Category:  category,
								HCL:       getBoolFromInterface(varMap["hcl"], false),
								Sensitive: getBoolFromInterface(varMap["sensitive"], false),
							},
						}

						// Add optional description
						if desc := getStringFromInterface(varMap["description"], ""); desc != "" {
							varData.Attributes.Description = desc
						}

						variables = append(variables, varData)
					}
				}
			}
		}
	}

	if len(variables) == 0 {
		err := fmt.Errorf("at least one variable is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	logger.Debugf("Creating %d HCP Terraform variables in workspace %s", len(variables), workspaceID)

	// Create variables
	response, err := hcpClient.BulkCreateWorkspaceVariables(token, workspaceID, variables)
	if err != nil {
		logger.Errorf("Failed to bulk create HCP Terraform variables: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully created %d variables in workspace %s", len(response.Data), workspaceID)

	// Format response
	result := formatBulkVariablesResponse(response)
	return mcp.NewToolResultText(result), nil
}

// ====================
// Helper Functions
// ====================

// formatVariablesResponse formats the variables response into a user-friendly format
func formatVariablesResponse(response *hcp_terraform.VariableResponse) string {
	if len(response.Data) == 0 {
		return "No variables found in this workspace."
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d variable(s):\n\n", len(response.Data)))

	// Group variables by category
	terraformVars := []hcp_terraform.Variable{}
	envVars := []hcp_terraform.Variable{}

	for _, variable := range response.Data {
		if variable.Attributes.Category == "terraform" {
			terraformVars = append(terraformVars, variable)
		} else {
			envVars = append(envVars, variable)
		}
	}

	// Display Terraform variables
	if len(terraformVars) > 0 {
		result.WriteString("## Terraform Variables\n")
		for i, variable := range terraformVars {
			result.WriteString(fmt.Sprintf("### %d. %s\n", i+1, variable.Attributes.Key))
			result.WriteString(fmt.Sprintf("- **ID**: %s\n", variable.ID))
			if variable.Attributes.Sensitive {
				result.WriteString("- **Value**: [SENSITIVE]\n")
			} else {
				result.WriteString(fmt.Sprintf("- **Value**: %s\n", variable.Attributes.Value))
			}
			result.WriteString(fmt.Sprintf("- **HCL**: %t\n", variable.Attributes.HCL))
			result.WriteString(fmt.Sprintf("- **Sensitive**: %t\n", variable.Attributes.Sensitive))
			if variable.Attributes.Description != "" {
				result.WriteString(fmt.Sprintf("- **Description**: %s\n", variable.Attributes.Description))
			}
			if i < len(terraformVars)-1 {
				result.WriteString("\n")
			}
		}
		result.WriteString("\n")
	}

	// Display Environment variables
	if len(envVars) > 0 {
		result.WriteString("## Environment Variables\n")
		for i, variable := range envVars {
			result.WriteString(fmt.Sprintf("### %d. %s\n", i+1, variable.Attributes.Key))
			result.WriteString(fmt.Sprintf("- **ID**: %s\n", variable.ID))
			if variable.Attributes.Sensitive {
				result.WriteString("- **Value**: [SENSITIVE]\n")
			} else {
				result.WriteString(fmt.Sprintf("- **Value**: %s\n", variable.Attributes.Value))
			}
			result.WriteString(fmt.Sprintf("- **Sensitive**: %t\n", variable.Attributes.Sensitive))
			if variable.Attributes.Description != "" {
				result.WriteString(fmt.Sprintf("- **Description**: %s\n", variable.Attributes.Description))
			}
			if i < len(envVars)-1 {
				result.WriteString("\n")
			}
		}
	}

	return result.String()
}

// formatVariableDetailsResponse formats a single variable response into a user-friendly format
func formatVariableDetailsResponse(response *hcp_terraform.SingleVariableResponse) string {
	variable := response.Data
	var result strings.Builder

	result.WriteString(fmt.Sprintf("# Variable: %s\n\n", variable.Attributes.Key))

	// Basic Information
	result.WriteString("## Details\n")
	result.WriteString(fmt.Sprintf("- **ID**: %s\n", variable.ID))
	result.WriteString(fmt.Sprintf("- **Key**: %s\n", variable.Attributes.Key))
	result.WriteString(fmt.Sprintf("- **Category**: %s\n", variable.Attributes.Category))

	if variable.Attributes.Sensitive {
		result.WriteString("- **Value**: [SENSITIVE]\n")
	} else {
		result.WriteString(fmt.Sprintf("- **Value**: %s\n", variable.Attributes.Value))
	}

	if variable.Attributes.Category == "terraform" {
		result.WriteString(fmt.Sprintf("- **HCL**: %t\n", variable.Attributes.HCL))
	}
	result.WriteString(fmt.Sprintf("- **Sensitive**: %t\n", variable.Attributes.Sensitive))

	if variable.Attributes.Description != "" {
		result.WriteString(fmt.Sprintf("- **Description**: %s\n", variable.Attributes.Description))
	}

	result.WriteString(fmt.Sprintf("- **Version ID**: %s\n", variable.Attributes.VersionID))

	return result.String()
}

// formatBulkVariablesResponse formats a bulk create response into a user-friendly format
func formatBulkVariablesResponse(response *hcp_terraform.BulkVariableCreateResponse) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Successfully created %d variable(s):\n\n", len(response.Data)))

	for i, variable := range response.Data {
		result.WriteString(fmt.Sprintf("## %d. %s (%s)\n", i+1, variable.Attributes.Key, variable.Attributes.Category))
		result.WriteString(fmt.Sprintf("- **ID**: %s\n", variable.ID))
		if variable.Attributes.Sensitive {
			result.WriteString("- **Value**: [SENSITIVE]\n")
		} else {
			result.WriteString(fmt.Sprintf("- **Value**: %s\n", variable.Attributes.Value))
		}
		if variable.Attributes.Description != "" {
			result.WriteString(fmt.Sprintf("- **Description**: %s\n", variable.Attributes.Description))
		}
		if i < len(response.Data)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
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
