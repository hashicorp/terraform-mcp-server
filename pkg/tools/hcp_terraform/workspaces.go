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

// GetWorkspaces creates the MCP tool for listing workspaces in an organization
func GetWorkspaces(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_workspaces",
			Description: "Fetches workspaces from an HCP Terraform organization with filtering and pagination support",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"organization_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the organization to list workspaces from",
					},
					"page_size": map[string]interface{}{
						"type":        "integer",
						"description": "Number of workspaces per page (default: 20, max: 100)",
						"minimum":     1,
						"maximum":     100,
						"default":     20,
					},
					"page_number": map[string]interface{}{
						"type":        "integer",
						"description": "Page number to retrieve (default: 1)",
						"minimum":     1,
						"default":     1,
					},
					"search_name": map[string]interface{}{
						"type":        "string",
						"description": "Optional search query to filter workspaces by name using fuzzy search",
					},
					"search_tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional array of tag names to filter workspaces (workspaces must have all specified tags)",
					},
					"search_exclude_tags": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional array of tag names to exclude workspaces (workspaces with any of these tags will be excluded)",
					},
					"search_wildcard_name": map[string]interface{}{
						"type":        "string",
						"description": "Optional wildcard search for workspace names (e.g., '*-prod' for names ending in '-prod')",
					},
					"sort": map[string]interface{}{
						"type":        "string",
						"description": "Optional sort order. Options: 'name', 'current-run.created-at', 'latest-change-at'. Prepend '-' for reverse order (e.g., '-name')",
					},
					"filter_project_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional project ID to filter workspaces by project",
					},
					"filter_current_run_status": map[string]interface{}{
						"type":        "string",
						"description": "Optional run status to filter workspaces by current run status",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"organization_name"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getWorkspacesHandler(hcpClient, request, logger)
		},
	}
}

// GetWorkspaceDetails creates the MCP tool for getting detailed information about a specific workspace
func GetWorkspaceDetails(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_workspace_details",
			Description: "Fetches detailed information about a specific HCP Terraform workspace by ID or organization/workspace name",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to retrieve (use this OR organization_name + workspace_name)",
					},
					"organization_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the organization (required if using workspace_name instead of workspace_id)",
					},
					"workspace_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the workspace to retrieve (required if using organization_name instead of workspace_id)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{}, // Either workspace_id OR (organization_name + workspace_name) is required, validated in handler
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getWorkspaceDetailsHandler(hcpClient, request, logger)
		},
	}
}

// CreateWorkspace creates the MCP tool for creating a new workspace
func CreateWorkspace(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_workspace",
			Description: "Creates a new HCP Terraform workspace in the specified organization with comprehensive configuration options",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"organization_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the organization to create the workspace in",
					},
					"workspace_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the workspace to create (must be unique in the organization)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Optional description for the workspace",
					},
					"execution_mode": map[string]interface{}{
						"type":        "string",
						"description": "Execution mode: 'remote', 'local', or 'agent' (default: remote)",
						"enum":        []string{"remote", "local", "agent"},
						"default":     "remote",
					},
					"terraform_version": map[string]interface{}{
						"type":        "string",
						"description": "Terraform version to use (default: latest)",
					},
					"auto_apply": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to automatically apply successful plans (default: false)",
						"default":     false,
					},
					"working_directory": map[string]interface{}{
						"type":        "string",
						"description": "Relative path that Terraform will execute within (default: root)",
					},
					"global_remote_state": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether all workspaces in the organization can access this workspace's state (default: false)",
						"default":     false,
					},
					"queue_all_runs": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether runs should be queued immediately after workspace creation (default: false)",
						"default":     false,
					},
					"speculative_enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to allow automatic speculative plans (default: true)",
						"default":     true,
					},
					"assessments_enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to enable health assessments (HCP Terraform Plus only, default: false)",
						"default":     false,
					},
					"allow_destroy_plan": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether destroy plans can be queued on the workspace (default: true)",
						"default":     true,
					},
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional project ID to create the workspace in (default: organization's default project)",
					},
					"tag_bindings": map[string]interface{}{
						"type":        "array",
						"description": "Optional array of key-value tag pairs to attach to the workspace",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"key": map[string]interface{}{
									"type":        "string",
									"description": "Tag key",
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "Tag value",
								},
							},
							"required": []string{"key", "value"},
						},
					},
					"vcs_repo": map[string]interface{}{
						"type":        "object",
						"description": "Optional VCS repository configuration",
						"properties": map[string]interface{}{
							"identifier": map[string]interface{}{
								"type":        "string",
								"description": "VCS repository identifier (format: org/repo)",
							},
							"oauth_token_id": map[string]interface{}{
								"type":        "string",
								"description": "OAuth token ID for VCS authentication",
							},
							"branch": map[string]interface{}{
								"type":        "string",
								"description": "Repository branch (default: repository's default branch)",
							},
							"ingress_submodules": map[string]interface{}{
								"type":        "boolean",
								"description": "Whether to fetch submodules when cloning (default: false)",
								"default":     false,
							},
							"tags_regex": map[string]interface{}{
								"type":        "string",
								"description": "Regular expression for matching Git tags",
							},
						},
						"required": []string{"identifier", "oauth_token_id"},
					},
					"agent_pool_id": map[string]interface{}{
						"type":        "string",
						"description": "Agent pool ID (required when execution_mode is 'agent')",
					},
					"source_name": map[string]interface{}{
						"type":        "string",
						"description": "Friendly name for the application creating this workspace",
					},
					"source_url": map[string]interface{}{
						"type":        "string",
						"description": "URL for the application creating this workspace",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"organization_name", "workspace_name"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createWorkspaceHandler(hcpClient, request, logger)
		},
	}
}

// UpdateWorkspace creates the MCP tool for updating an existing workspace
func UpdateWorkspace(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "update_hcp_terraform_workspace",
			Description: "Updates an existing HCP Terraform workspace configuration by ID or organization/workspace name",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to update (use this OR organization_name + workspace_name)",
					},
					"organization_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the organization (required if using workspace_name instead of workspace_id)",
					},
					"workspace_name": map[string]interface{}{
						"type":        "string",
						"description": "The name of the workspace to update (required if using organization_name instead of workspace_id)",
					},
					"new_name": map[string]interface{}{
						"type":        "string",
						"description": "Optional new name for the workspace (WARNING: changes workspace URL)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Optional new description for the workspace",
					},
					"execution_mode": map[string]interface{}{
						"type":        "string",
						"description": "Execution mode: 'remote', 'local', or 'agent'",
						"enum":        []string{"remote", "local", "agent"},
					},
					"terraform_version": map[string]interface{}{
						"type":        "string",
						"description": "Terraform version to use",
					},
					"auto_apply": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to automatically apply successful plans",
					},
					"working_directory": map[string]interface{}{
						"type":        "string",
						"description": "Relative path that Terraform will execute within",
					},
					"global_remote_state": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether all workspaces in the organization can access this workspace's state",
					},
					"queue_all_runs": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether runs should be queued immediately",
					},
					"speculative_enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to allow automatic speculative plans",
					},
					"assessments_enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to enable health assessments (HCP Terraform Plus only)",
					},
					"allow_destroy_plan": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether destroy plans can be queued on the workspace",
					},
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Project ID to move the workspace to",
					},
					"tag_bindings": map[string]interface{}{
						"type":        "array",
						"description": "Array of key-value tag pairs to replace existing tags",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"key": map[string]interface{}{
									"type":        "string",
									"description": "Tag key",
								},
								"value": map[string]interface{}{
									"type":        "string",
									"description": "Tag value",
								},
							},
							"required": []string{"key", "value"},
						},
					},
					"vcs_repo": map[string]interface{}{
						"type":        "object",
						"description": "VCS repository configuration (set to null to remove VCS connection)",
						"properties": map[string]interface{}{
							"identifier": map[string]interface{}{
								"type":        "string",
								"description": "VCS repository identifier (format: org/repo)",
							},
							"oauth_token_id": map[string]interface{}{
								"type":        "string",
								"description": "OAuth token ID for VCS authentication",
							},
							"branch": map[string]interface{}{
								"type":        "string",
								"description": "Repository branch",
							},
							"ingress_submodules": map[string]interface{}{
								"type":        "boolean",
								"description": "Whether to fetch submodules when cloning",
							},
							"tags_regex": map[string]interface{}{
								"type":        "string",
								"description": "Regular expression for matching Git tags",
							},
						},
					},
					"agent_pool_id": map[string]interface{}{
						"type":        "string",
						"description": "Agent pool ID (required when execution_mode is 'agent')",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{}, // Either workspace_id OR (organization_name + workspace_name) is required, validated in handler
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return updateWorkspaceHandler(hcpClient, request, logger)
		},
	}
}

// ====================
// Handler Functions
// ====================

func getWorkspacesHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	organizationName := request.GetString("organization_name", "")
	if organizationName == "" {
		err := fmt.Errorf("organization_name is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Parse request parameters
	opts := &hcp_terraform.WorkspaceListOptions{
		PageSize:               request.GetInt("page_size", 20),
		PageNumber:             request.GetInt("page_number", 1),
		SearchName:             request.GetString("search_name", ""),
		SearchWildcardName:     request.GetString("search_wildcard_name", ""),
		Sort:                   request.GetString("sort", ""),
		FilterProjectID:        request.GetString("filter_project_id", ""),
		FilterCurrentRunStatus: request.GetString("filter_current_run_status", ""),
	}

	// Handle array parameters
	if searchTags := request.GetStringSlice("search_tags", []string{}); len(searchTags) > 0 {
		opts.SearchTags = searchTags
	}
	if searchExcludeTags := request.GetStringSlice("search_exclude_tags", []string{}); len(searchExcludeTags) > 0 {
		opts.SearchExcludeTags = searchExcludeTags
	}

	logger.Debugf("Fetching HCP Terraform workspaces for organization '%s' with options: %+v", organizationName, opts)

	// Fetch workspaces
	response, err := hcpClient.GetWorkspaces(token, organizationName, opts)
	if err != nil {
		logger.Errorf("Failed to fetch HCP Terraform workspaces: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully fetched %d workspaces from organization '%s'", len(response.Data), organizationName)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(response)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func getWorkspaceDetailsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get parameters
	workspaceID := request.GetString("workspace_id", "")
	organizationName := request.GetString("organization_name", "")
	workspaceName := request.GetString("workspace_name", "")

	// Validate parameters
	if workspaceID == "" && (organizationName == "" || workspaceName == "") {
		err := fmt.Errorf("either workspace_id OR (organization_name + workspace_name) is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	var response *hcp_terraform.SingleWorkspaceResponse

	// Fetch workspace by ID or name
	if workspaceID != "" {
		logger.Debugf("Fetching HCP Terraform workspace by ID: %s", workspaceID)
		response, err = hcpClient.GetWorkspaceByID(token, workspaceID)
	} else {
		logger.Debugf("Fetching HCP Terraform workspace by name: %s/%s", organizationName, workspaceName)
		response, err = hcpClient.GetWorkspaceByName(token, organizationName, workspaceName)
	}

	if err != nil {
		logger.Errorf("Failed to fetch HCP Terraform workspace: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully fetched workspace: %s", response.Data.Attributes.Name)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(response)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func createWorkspaceHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	organizationName := request.GetString("organization_name", "")
	workspaceName := request.GetString("workspace_name", "")

	if organizationName == "" || workspaceName == "" {
		err := fmt.Errorf("organization_name and workspace_name are required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Build workspace creation request
	createRequest := &hcp_terraform.WorkspaceCreateRequest{
		Data: hcp_terraform.WorkspaceCreateData{
			Type: "workspaces",
			Attributes: hcp_terraform.WorkspaceCreateAttributes{
				Name: workspaceName,
			},
		},
	}

	// Add optional attributes
	if desc := request.GetString("description", ""); desc != "" {
		createRequest.Data.Attributes.Description = &desc
	}
	if execMode := request.GetString("execution_mode", ""); execMode != "" {
		createRequest.Data.Attributes.ExecutionMode = &execMode
	}
	if tfVersion := request.GetString("terraform_version", ""); tfVersion != "" {
		createRequest.Data.Attributes.TerraformVersion = &tfVersion
	}
	if workingDir := request.GetString("working_directory", ""); workingDir != "" {
		createRequest.Data.Attributes.WorkingDirectory = &workingDir
	}
	if sourceName := request.GetString("source_name", ""); sourceName != "" {
		createRequest.Data.Attributes.SourceName = &sourceName
	}
	if sourceURL := request.GetString("source_url", ""); sourceURL != "" {
		createRequest.Data.Attributes.SourceURL = &sourceURL
	}
	if agentPoolID := request.GetString("agent_pool_id", ""); agentPoolID != "" {
		createRequest.Data.Attributes.AgentPoolID = &agentPoolID
	}

	// Handle boolean attributes with proper optional checking
	if arguments := request.GetArguments(); arguments != nil {
		if autoApplyRaw, exists := arguments["auto_apply"]; exists {
			if autoApply, ok := autoApplyRaw.(bool); ok {
				createRequest.Data.Attributes.AutoApply = &autoApply
			}
		}
		if globalRemoteStateRaw, exists := arguments["global_remote_state"]; exists {
			if globalRemoteState, ok := globalRemoteStateRaw.(bool); ok {
				createRequest.Data.Attributes.GlobalRemoteState = &globalRemoteState
			}
		}
		if queueAllRunsRaw, exists := arguments["queue_all_runs"]; exists {
			if queueAllRuns, ok := queueAllRunsRaw.(bool); ok {
				createRequest.Data.Attributes.QueueAllRuns = &queueAllRuns
			}
		}
		if speculativeEnabledRaw, exists := arguments["speculative_enabled"]; exists {
			if speculativeEnabled, ok := speculativeEnabledRaw.(bool); ok {
				createRequest.Data.Attributes.SpeculativeEnabled = &speculativeEnabled
			}
		}
		if assessmentsEnabledRaw, exists := arguments["assessments_enabled"]; exists {
			if assessmentsEnabled, ok := assessmentsEnabledRaw.(bool); ok {
				createRequest.Data.Attributes.AssessmentsEnabled = &assessmentsEnabled
			}
		}
		if allowDestroyPlanRaw, exists := arguments["allow_destroy_plan"]; exists {
			if allowDestroyPlan, ok := allowDestroyPlanRaw.(bool); ok {
				createRequest.Data.Attributes.AllowDestroyPlan = &allowDestroyPlan
			}
		}
	}

	// Handle relationships
	if projectID := request.GetString("project_id", ""); projectID != "" {
		createRequest.Data.Relationships = &hcp_terraform.WorkspaceCreateRelationships{
			Project: &hcp_terraform.WorkspaceCreateRelationshipProject{
				Data: hcp_terraform.RelationshipDataItem{
					Type: "projects",
					ID:   projectID,
				},
			},
		}
	}

	// Handle tag bindings
	if arguments := request.GetArguments(); arguments != nil {
		if tagBindingsRaw, exists := arguments["tag_bindings"]; exists {
			if tagBindingsArray, ok := tagBindingsRaw.([]interface{}); ok {
				var tagBindings []hcp_terraform.WorkspaceTagBinding
				for _, tagRaw := range tagBindingsArray {
					if tagMap, ok := tagRaw.(map[string]interface{}); ok {
						if key, hasKey := tagMap["key"].(string); hasKey {
							if value, hasValue := tagMap["value"].(string); hasValue {
								tagBindings = append(tagBindings, hcp_terraform.WorkspaceTagBinding{
									Type: "tag-bindings",
									Attributes: hcp_terraform.WorkspaceTagBindingAttrs{
										Key:   key,
										Value: value,
									},
								})
							}
						}
					}
				}
				if len(tagBindings) > 0 {
					if createRequest.Data.Relationships == nil {
						createRequest.Data.Relationships = &hcp_terraform.WorkspaceCreateRelationships{}
					}
					createRequest.Data.Relationships.TagBindings = &hcp_terraform.WorkspaceCreateRelationshipTagBindings{
						Data: tagBindings,
					}
				}
			}
		}
	}

	// Handle VCS repository configuration
	if arguments := request.GetArguments(); arguments != nil {
		if vcsRepoRaw, exists := arguments["vcs_repo"]; exists {
			if vcsRepoMap, ok := vcsRepoRaw.(map[string]interface{}); ok {
				vcsRepo := &hcp_terraform.WorkspaceCreateVCSRepo{}

				if identifier, ok := vcsRepoMap["identifier"].(string); ok {
					vcsRepo.Identifier = identifier
				}
				if oauthTokenID, ok := vcsRepoMap["oauth_token_id"].(string); ok {
					vcsRepo.OAuthTokenID = &oauthTokenID
				}
				if branch, ok := vcsRepoMap["branch"].(string); ok {
					vcsRepo.Branch = &branch
				}
				if ingressSubmodules, ok := vcsRepoMap["ingress_submodules"].(bool); ok {
					vcsRepo.IngressSubmodules = &ingressSubmodules
				}
				if tagsRegex, ok := vcsRepoMap["tags_regex"].(string); ok {
					vcsRepo.TagsRegex = &tagsRegex
				}
				if gitHubAppInstallationID, ok := vcsRepoMap["github_app_installation_id"].(string); ok {
					vcsRepo.GitHubAppInstallationID = &gitHubAppInstallationID
				}

				createRequest.Data.Attributes.VCSRepo = vcsRepo
			}
		}
	}

	logger.Debugf("Creating HCP Terraform workspace '%s' in organization '%s'", workspaceName, organizationName)

	// Create workspace
	response, err := hcpClient.CreateWorkspace(token, organizationName, createRequest)
	if err != nil {
		logger.Errorf("Failed to create HCP Terraform workspace: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully created workspace: %s", response.Data.Attributes.Name)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func updateWorkspaceHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get parameters
	workspaceID := request.GetString("workspace_id", "")
	organizationName := request.GetString("organization_name", "")
	workspaceName := request.GetString("workspace_name", "")

	// Validate parameters
	if workspaceID == "" && (organizationName == "" || workspaceName == "") {
		err := fmt.Errorf("either workspace_id OR (organization_name + workspace_name) is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Build workspace update request
	updateRequest := &hcp_terraform.WorkspaceUpdateRequest{
		Data: hcp_terraform.WorkspaceUpdateData{
			Type:       "workspaces",
			Attributes: &hcp_terraform.WorkspaceUpdateAttributes{},
		},
	}

	// Add optional attributes
	if newName := request.GetString("new_name", ""); newName != "" {
		updateRequest.Data.Attributes.Name = &newName
	}
	if desc := request.GetString("description", ""); desc != "" {
		updateRequest.Data.Attributes.Description = &desc
	}
	if execMode := request.GetString("execution_mode", ""); execMode != "" {
		updateRequest.Data.Attributes.ExecutionMode = &execMode
	}
	if tfVersion := request.GetString("terraform_version", ""); tfVersion != "" {
		updateRequest.Data.Attributes.TerraformVersion = &tfVersion
	}
	if workingDir := request.GetString("working_directory", ""); workingDir != "" {
		updateRequest.Data.Attributes.WorkingDirectory = &workingDir
	}
	if agentPoolID := request.GetString("agent_pool_id", ""); agentPoolID != "" {
		updateRequest.Data.Attributes.AgentPoolID = &agentPoolID
	}

	// Handle boolean attributes with proper optional checking
	if arguments := request.GetArguments(); arguments != nil {
		if autoApplyRaw, exists := arguments["auto_apply"]; exists {
			if autoApply, ok := autoApplyRaw.(bool); ok {
				updateRequest.Data.Attributes.AutoApply = &autoApply
			}
		}
		if globalRemoteStateRaw, exists := arguments["global_remote_state"]; exists {
			if globalRemoteState, ok := globalRemoteStateRaw.(bool); ok {
				updateRequest.Data.Attributes.GlobalRemoteState = &globalRemoteState
			}
		}
		if queueAllRunsRaw, exists := arguments["queue_all_runs"]; exists {
			if queueAllRuns, ok := queueAllRunsRaw.(bool); ok {
				updateRequest.Data.Attributes.QueueAllRuns = &queueAllRuns
			}
		}
		if speculativeEnabledRaw, exists := arguments["speculative_enabled"]; exists {
			if speculativeEnabled, ok := speculativeEnabledRaw.(bool); ok {
				updateRequest.Data.Attributes.SpeculativeEnabled = &speculativeEnabled
			}
		}
		if assessmentsEnabledRaw, exists := arguments["assessments_enabled"]; exists {
			if assessmentsEnabled, ok := assessmentsEnabledRaw.(bool); ok {
				updateRequest.Data.Attributes.AssessmentsEnabled = &assessmentsEnabled
			}
		}
		if allowDestroyPlanRaw, exists := arguments["allow_destroy_plan"]; exists {
			if allowDestroyPlan, ok := allowDestroyPlanRaw.(bool); ok {
				updateRequest.Data.Attributes.AllowDestroyPlan = &allowDestroyPlan
			}
		}
	}

	// Handle relationships
	if projectID := request.GetString("project_id", ""); projectID != "" {
		updateRequest.Data.Relationships = &hcp_terraform.WorkspaceCreateRelationships{
			Project: &hcp_terraform.WorkspaceCreateRelationshipProject{
				Data: hcp_terraform.RelationshipDataItem{
					Type: "projects",
					ID:   projectID,
				},
			},
		}
	}

	// Handle tag bindings
	if arguments := request.GetArguments(); arguments != nil {
		if tagBindingsRaw, exists := arguments["tag_bindings"]; exists {
			if tagBindingsArray, ok := tagBindingsRaw.([]interface{}); ok {
				var tagBindings []hcp_terraform.WorkspaceTagBinding
				for _, tagRaw := range tagBindingsArray {
					if tagMap, ok := tagRaw.(map[string]interface{}); ok {
						if key, hasKey := tagMap["key"].(string); hasKey {
							if value, hasValue := tagMap["value"].(string); hasValue {
								tagBindings = append(tagBindings, hcp_terraform.WorkspaceTagBinding{
									Type: "tag-bindings",
									Attributes: hcp_terraform.WorkspaceTagBindingAttrs{
										Key:   key,
										Value: value,
									},
								})
							}
						}
					}
				}
				if len(tagBindings) > 0 {
					if updateRequest.Data.Relationships == nil {
						updateRequest.Data.Relationships = &hcp_terraform.WorkspaceCreateRelationships{}
					}
					updateRequest.Data.Relationships.TagBindings = &hcp_terraform.WorkspaceCreateRelationshipTagBindings{
						Data: tagBindings,
					}
				}
			}
		}
	}

	// Handle VCS repository configuration
	if arguments := request.GetArguments(); arguments != nil {
		if vcsRepoRaw, exists := arguments["vcs_repo"]; exists {
			if vcsRepoMap, ok := vcsRepoRaw.(map[string]interface{}); ok {
				vcsRepo := &hcp_terraform.WorkspaceCreateVCSRepo{}

				if identifier, ok := vcsRepoMap["identifier"].(string); ok {
					vcsRepo.Identifier = identifier
				}
				if oauthTokenID, ok := vcsRepoMap["oauth_token_id"].(string); ok {
					vcsRepo.OAuthTokenID = &oauthTokenID
				}
				if branch, ok := vcsRepoMap["branch"].(string); ok {
					vcsRepo.Branch = &branch
				}
				if ingressSubmodules, ok := vcsRepoMap["ingress_submodules"].(bool); ok {
					vcsRepo.IngressSubmodules = &ingressSubmodules
				}
				if tagsRegex, ok := vcsRepoMap["tags_regex"].(string); ok {
					vcsRepo.TagsRegex = &tagsRegex
				}
				if gitHubAppInstallationID, ok := vcsRepoMap["github_app_installation_id"].(string); ok {
					vcsRepo.GitHubAppInstallationID = &gitHubAppInstallationID
				}

				updateRequest.Data.Attributes.VCSRepo = vcsRepo
			}
		}
	}

	var response *hcp_terraform.SingleWorkspaceResponse

	// Update workspace by ID or name
	if workspaceID != "" {
		logger.Debugf("Updating HCP Terraform workspace by ID: %s", workspaceID)
		response, err = hcpClient.UpdateWorkspaceByID(token, workspaceID, updateRequest)
	} else {
		logger.Debugf("Updating HCP Terraform workspace by name: %s/%s", organizationName, workspaceName)
		response, err = hcpClient.UpdateWorkspaceByName(token, organizationName, workspaceName, updateRequest)
	}

	if err != nil {
		logger.Errorf("Failed to update HCP Terraform workspace: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully updated workspace: %s", response.Data.Attributes.Name)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}
