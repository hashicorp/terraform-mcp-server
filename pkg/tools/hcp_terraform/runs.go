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

// CreateRun creates the MCP tool for creating a new run
func CreateRun(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_run",
			Description: "Creates a new run (plan/apply/destroy) for an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create the run for (format: ws-xxxxxxxxxx)",
					},
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Optional message to describe the run",
					},
					"is_destroy": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether this is a destroy run (default: false)",
						"default":     false,
					},
					"target_addrs": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional array of resource addresses to target for the run",
					},
					"auto_apply": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to automatically apply the run if the plan succeeds (overrides workspace setting)",
					},
					"plan_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether this is a plan-only run (default: false)",
						"default":     false,
					},
					"refresh": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to refresh state before planning (default: true)",
						"default":     true,
					},
					"refresh_only": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether this is a refresh-only run (default: false)",
						"default":     false,
					},
					"replace_addrs": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional array of resource addresses to force replacement",
					},
					"configuration_version_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional configuration version ID to use (defaults to workspace's current configuration)",
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
			return createRunHandler(hcpClient, request, logger)
		},
	}
}

// GetRun creates the MCP tool for getting run details
func GetRun(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_run",
			Description: "Fetches detailed information about a specific HCP Terraform run",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"run_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the run to retrieve (format: run-xxxxxxxxxx)",
					},
					"include": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional array of related objects to include (plan, apply, configuration-version, workspace, created-by, etc.)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"run_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getRunHandler(hcpClient, request, logger)
		},
	}
}

// ListRuns creates the MCP tool for listing workspace runs
func ListRuns(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "list_hcp_terraform_runs",
			Description: "Lists runs for an HCP Terraform workspace with filtering and pagination support",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to list runs for (format: ws-xxxxxxxxxx)",
					},
					"page_size": map[string]interface{}{
						"type":        "integer",
						"description": "Number of runs per page (default: 20, max: 100)",
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
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Optional filter by run status (pending, planning, planned, applying, applied, discarded, errored, canceled, etc.)",
					},
					"operation": map[string]interface{}{
						"type":        "string",
						"description": "Optional filter by operation type (plan_and_apply, plan_only, refresh_only, destroy, etc.)",
					},
					"source": map[string]interface{}{
						"type":        "string",
						"description": "Optional filter by run source (ui, api, vcs, etc.)",
					},
					"include": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Optional array of related objects to include",
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
			return listRunsHandler(hcpClient, request, logger)
		},
	}
}

// ====================
// Handler Functions
// ====================

func createRunHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Build run creation request
	createRequest := &hcp_terraform.RunCreateRequest{
		Data: hcp_terraform.RunCreateData{
			Type: "runs",
			Attributes: hcp_terraform.RunCreateAttributes{
				IsDestroy:   request.GetBool("is_destroy", false),
				Refresh:     request.GetBool("refresh", true),
				RefreshOnly: request.GetBool("refresh_only", false),
				PlanOnly:    request.GetBool("plan_only", false),
			},
			Relationships: hcp_terraform.RunCreateRelationships{
				Workspace: hcp_terraform.RelationshipDataItem{
					Type: "workspaces",
					ID:   workspaceID,
				},
			},
		},
	}

	// Add optional attributes
	if message := request.GetString("message", ""); message != "" {
		createRequest.Data.Attributes.Message = &message
	}

	if configVersionID := request.GetString("configuration_version_id", ""); configVersionID != "" {
		createRequest.Data.Relationships.ConfigurationVersion = &hcp_terraform.RelationshipDataItem{
			Type: "configuration-versions",
			ID:   configVersionID,
		}
	}

	// Handle boolean attributes with proper optional checking
	if arguments := request.GetArguments(); arguments != nil {
		if autoApplyRaw, exists := arguments["auto_apply"]; exists {
			if autoApply, ok := autoApplyRaw.(bool); ok {
				createRequest.Data.Attributes.AutoApply = &autoApply
			}
		}
	}

	// Handle array parameters
	if targetAddrs := request.GetStringSlice("target_addrs", []string{}); len(targetAddrs) > 0 {
		createRequest.Data.Attributes.TargetAddrs = targetAddrs
	}
	if replaceAddrs := request.GetStringSlice("replace_addrs", []string{}); len(replaceAddrs) > 0 {
		createRequest.Data.Attributes.ReplaceAddrs = replaceAddrs
	}

	logger.Debugf("Creating HCP Terraform run for workspace '%s'", workspaceID)

	// Create run
	response, err := hcpClient.CreateRun(token, createRequest)
	if err != nil {
		logger.Errorf("Failed to create HCP Terraform run: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully created run: %s with status: %s", response.Data.ID, response.Data.Attributes.Status)

	// Return JSON response
	jsonResult, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return mcp.NewToolResultText("Error marshaling response"), nil
	}
	return mcp.NewToolResultText(string(jsonResult)), nil
}

func getRunHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	runID := request.GetString("run_id", "")
	if runID == "" {
		err := fmt.Errorf("run_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Parse optional include parameter
	include := request.GetStringSlice("include", []string{})

	logger.Debugf("Fetching HCP Terraform run: %s", runID)

	// Fetch run
	response, err := hcpClient.GetRun(token, runID, include)
	if err != nil {
		logger.Errorf("Failed to fetch HCP Terraform run: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully fetched run: %s with status: %s", response.Data.ID, response.Data.Attributes.Status)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(response)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func listRunsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Parse request parameters
	opts := &hcp_terraform.RunListOptions{
		PageSize:   request.GetInt("page_size", 20),
		PageNumber: request.GetInt("page_number", 1),
		Status:     request.GetString("status", ""),
		Operation:  request.GetString("operation", ""),
		Source:     request.GetString("source", ""),
		Include:    request.GetStringSlice("include", []string{}),
	}

	logger.Debugf("Fetching HCP Terraform runs for workspace '%s' with options: %+v", workspaceID, opts)

	// Fetch runs
	response, err := hcpClient.ListRuns(token, workspaceID, opts)
	if err != nil {
		logger.Errorf("Failed to fetch HCP Terraform runs: %v", err)
		// Format error response for better user experience
		errorMsg := formatErrorResponse(err)
		return mcp.NewToolResultText(errorMsg), nil
	}

	logger.Infof("Successfully fetched %d runs from workspace '%s'", len(response.Data), workspaceID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(response)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

// ApplyRun creates the MCP tool for applying a planned run
func ApplyRun(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "apply_hcp_terraform_run",
			Description: "Applies a planned HCP Terraform run (transitions from planned to applying state)",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"run_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the run to apply (format: run-xxxxxxxxxx). Run must be in 'planned' status.",
					},
					"comment": map[string]interface{}{
						"type":        "string",
						"description": "Optional comment to explain why the run is being applied",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"run_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return applyRunHandler(hcpClient, request, logger)
		},
	}
}

// DiscardRun creates the MCP tool for discarding a planned run
func DiscardRun(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "discard_hcp_terraform_run",
			Description: "Discards a planned HCP Terraform run (cancels it without applying changes)",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"run_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the run to discard (format: run-xxxxxxxxxx). Run must be in 'planned' status.",
					},
					"comment": map[string]interface{}{
						"type":        "string",
						"description": "Optional comment to explain why the run is being discarded",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"run_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return discardRunHandler(hcpClient, request, logger)
		},
	}
}

// CancelRun creates the MCP tool for canceling a running run
func CancelRun(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "cancel_hcp_terraform_run",
			Description: "Cancels a running HCP Terraform run (stops execution in progress)",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"run_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the run to cancel (format: run-xxxxxxxxxx). Run must be in an active state (planning, applying, etc.).",
					},
					"comment": map[string]interface{}{
						"type":        "string",
						"description": "Optional comment to explain why the run is being canceled",
					},
					"force_cancel": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to force cancel the run (use with caution, default: false)",
						"default":     false,
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"run_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return cancelRunHandler(hcpClient, request, logger)
		},
	}
}

// Handler functions for run actions

func applyRunHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	runID := request.GetString("run_id", "")
	if runID == "" {
		err := fmt.Errorf("run_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get optional parameters
	comment := request.GetString("comment", "")

	// Apply the run
	logger.Infof("Applying run '%s'", runID)
	run, err := hcpClient.ApplyRun(token, runID, comment)
	if err != nil {
		logger.Errorf("Failed to apply run '%s': %v", runID, err)
		return nil, utils.LogAndReturnError(logger, "apply run", err)
	}

	logger.Infof("Successfully applied run '%s'", runID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(run)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func discardRunHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	runID := request.GetString("run_id", "")
	if runID == "" {
		err := fmt.Errorf("run_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get optional parameters
	comment := request.GetString("comment", "")

	// Discard the run
	logger.Infof("Discarding run '%s'", runID)
	run, err := hcpClient.DiscardRun(token, runID, comment)
	if err != nil {
		logger.Errorf("Failed to discard run '%s': %v", runID, err)
		return nil, utils.LogAndReturnError(logger, "discard run", err)
	}

	logger.Infof("Successfully discarded run '%s'", runID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(run)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func cancelRunHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	runID := request.GetString("run_id", "")
	if runID == "" {
		err := fmt.Errorf("run_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get optional parameters
	comment := request.GetString("comment", "")
	forceCancel := request.GetBool("force_cancel", false)

	// Cancel the run
	action := "Canceling"
	if forceCancel {
		action = "Force-canceling"
	}
	logger.Infof("%s run '%s'", action, runID)
	run, err := hcpClient.CancelRun(token, runID, comment, forceCancel)
	if err != nil {
		logger.Errorf("Failed to cancel run '%s': %v", runID, err)
		return nil, utils.LogAndReturnError(logger, "cancel run", err)
	}

	action = "canceled"
	if forceCancel {
		action = "force-canceled"
	}
	logger.Infof("Successfully %s run '%s'", action, runID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(run)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

// GetPlan creates the MCP tool for getting plan details
func GetPlan(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_plan",
			Description: "Fetches detailed information about a specific HCP Terraform plan",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"plan_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the plan to retrieve (format: plan-xxxxxxxxxx)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"plan_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPlanHandler(hcpClient, request, logger)
		},
	}
}

// GetApply creates the MCP tool for getting apply details
func GetApply(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_apply",
			Description: "Fetches detailed information about a specific HCP Terraform apply",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"apply_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the apply to retrieve (format: apply-xxxxxxxxxx)",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"apply_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getApplyHandler(hcpClient, request, logger)
		},
	}
}

// Handler functions for plan/apply details

func getPlanHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	planID := request.GetString("plan_id", "")
	if planID == "" {
		err := fmt.Errorf("plan_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get the plan
	logger.Infof("Fetching plan '%s'", planID)
	plan, err := hcpClient.GetPlan(token, planID)
	if err != nil {
		logger.Errorf("Failed to fetch plan '%s': %v", planID, err)
		return nil, utils.LogAndReturnError(logger, "get plan", err)
	}

	logger.Infof("Successfully fetched plan '%s'", planID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(plan)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func getApplyHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	applyID := request.GetString("apply_id", "")
	if applyID == "" {
		err := fmt.Errorf("apply_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get the apply
	logger.Infof("Fetching apply '%s'", applyID)
	apply, err := hcpClient.GetApply(token, applyID)
	if err != nil {
		logger.Errorf("Failed to fetch apply '%s': %v", applyID, err)
		return nil, utils.LogAndReturnError(logger, "get apply", err)
	}

	logger.Infof("Successfully fetched apply '%s'", applyID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(apply)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

// GetRunLogs creates the MCP tool for getting run logs
func GetRunLogs(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_run_logs",
			Description: "Fetches logs for a specific HCP Terraform run (plan or apply logs)",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"run_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the run to get logs for (format: run-xxxxxxxxxx)",
					},
					"log_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of logs to retrieve: 'plan' or 'apply'",
						"enum":        []interface{}{"plan", "apply"},
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"run_id", "log_type"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getRunLogsHandler(hcpClient, request, logger)
		},
	}
}

// GetRunOutput creates the MCP tool for getting structured run output
func GetRunOutput(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_run_output",
			Description: "Fetches structured output and resource changes for a completed HCP Terraform run",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"run_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the run to get output for (format: run-xxxxxxxxxx)",
					},
					"include_plan": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to include plan output (default: true)",
						"default":     true,
					},
					"include_apply": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to include apply output (default: true)",
						"default":     true,
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"run_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getRunOutputHandler(hcpClient, request, logger)
		},
	}
}

// Handler functions for advanced features

func getRunLogsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	runID := request.GetString("run_id", "")
	if runID == "" {
		err := fmt.Errorf("run_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	logType := request.GetString("log_type", "")
	if logType == "" {
		err := fmt.Errorf("log_type is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	if logType != "plan" && logType != "apply" {
		err := fmt.Errorf("log_type must be 'plan' or 'apply'")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get the run logs
	logger.Infof("Fetching %s logs for run '%s'", logType, runID)
	logs, err := hcpClient.GetRunLogs(token, runID, logType)
	if err != nil {
		logger.Errorf("Failed to fetch %s logs for run '%s': %v", logType, runID, err)
		return nil, utils.LogAndReturnError(logger, "get run logs", err)
	}

	logger.Infof("Successfully fetched %s logs for run '%s'", logType, runID)

	// Return logs as text
	result := map[string]interface{}{
		"run_id":   runID,
		"log_type": logType,
		"logs":     logs,
	}

	jsonResult, jsonErr := json.Marshal(result)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}

func getRunOutputHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	runID := request.GetString("run_id", "")
	if runID == "" {
		err := fmt.Errorf("run_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Get optional parameters
	includePlan := request.GetBool("include_plan", true)
	includeApply := request.GetBool("include_apply", true)

	// Get the run output
	logger.Infof("Fetching output for run '%s' (plan: %v, apply: %v)", runID, includePlan, includeApply)
	output, err := hcpClient.GetRunOutput(token, runID, includePlan, includeApply)
	if err != nil {
		logger.Errorf("Failed to fetch output for run '%s': %v", runID, err)
		return nil, utils.LogAndReturnError(logger, "get run output", err)
	}

	logger.Infof("Successfully fetched output for run '%s'", runID)

	// Return raw API response as JSON
	jsonResult, jsonErr := json.Marshal(output)
	if jsonErr != nil {
		logger.Errorf("Failed to marshal result to JSON: %v", jsonErr)
		return nil, utils.LogAndReturnError(logger, "JSON marshaling", jsonErr)
	}

	return mcp.NewToolResultText(string(jsonResult)), nil
}
