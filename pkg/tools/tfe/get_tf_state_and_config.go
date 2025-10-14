// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetTfStateAndConfig creates a tool to fetch Terraform state for a workspace
func GetTfStateAndConfig(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_tf_state_and_config",
			mcp.WithDescription(`Fetches the current Terraform state for a workspace. Downloads the complete state file and provides raw state content without manipulation, suitable for comprehensive infrastructure analysis.`),
			mcp.WithTitleAnnotation("Get Terraform state for a workspace"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to fetch state for"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getTfStateAndConfigHandler(ctx, request, logger)
		},
	}
}

func getTfStateAndConfigHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get required parameters
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'workspace_name' parameter is required", err)
	}
	workspaceName = strings.TrimSpace(workspaceName)

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", err)
	}

	// Fetch workspace details
	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading workspace details", err)
	}

	// Convert workspace to JSON-serializable format
	workspaceInfo := &client.WorkspaceInfo{
		ID:                   workspace.ID,
		Name:                 workspace.Name,
		Description:          workspace.Description,
		Environment:          workspace.Environment,
		AutoApply:            workspace.AutoApply,
		TerraformVersion:     workspace.TerraformVersion,
		WorkingDirectory:     workspace.WorkingDirectory,
		ExecutionMode:        string(workspace.ExecutionMode),
		ResourceCount:        workspace.ResourceCount,
		ApplyDurationAverage: int64(workspace.ApplyDurationAverage),
		PlanDurationAverage:  int64(workspace.PlanDurationAverage),
	}

	// Build response with metadata using JSON-friendly struct
	response := &client.StateAndConfigJSONResponse{
		Type:      "get_tf_state_and_config",
		Success:   true,
		Workspace: workspaceInfo,
		Metadata: &client.ResponseMetadata{
			RetrievedAt:      time.Now(),
			WorkspaceID:      workspace.ID,
			OrganizationName: terraformOrgName,
			WorkspaceName:    workspaceName,
		},
	}

	// Fetch state data
	stateContent, err := fetchStateContent(ctx, tfeClient, workspace.ID, logger)
	if err != nil {
		logger.WithError(err).Warn("failed to fetch state data, continuing without it")
	} else {
		response.TfStateFileContent = stateContent
	}



	// Debug: Log what we're about to return
	if response.TfStateFileContent != nil {
		logger.WithFields(log.Fields{
			"tf_state_file_content_keys": getMapKeys(response.TfStateFileContent),
			"has_tf_state_file_content":  response.TfStateFileContent != nil,
		}).Info("Response state content summary")
	}

	// Debug: Log response preparation
	logger.WithFields(log.Fields{
		"has_tf_state_content": response.TfStateFileContent != nil,
		"workspace_id":         response.Workspace.ID,
	}).Info("Final structured response prepared")

	return mcp.NewToolResultStructuredOnly(response), nil
}

// fetchStateContent retrieves and parses the complete Terraform state file
func fetchStateContent(ctx context.Context, tfeClient *tfe.Client, workspaceID string, logger *log.Logger) (map[string]interface{}, error) {
	// Get current state version
	stateVersion, err := tfeClient.StateVersions.ReadCurrent(ctx, workspaceID)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading current state version", err)
	}

	// Download and parse the state file for raw content
	if stateVersion.JSONDownloadURL != "" {
		stateFileContent, err := downloadStateFile(ctx, stateVersion.JSONDownloadURL, logger)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, "failed to download state file", err)
		}

		return stateFileContent, nil
	}

	return nil, utils.LogAndReturnError(logger, "no state file download URL available", nil)
}



// downloadStateFile downloads the Terraform state file and returns it as raw JSON content
func downloadStateFile(ctx context.Context, downloadURL string, logger *log.Logger) (map[string]interface{}, error) {
	logger.Info("Downloading Terraform state file")

	// Get the token from environment (same way the TFE client was created)
	terraformToken := utils.GetEnv("TFE_TOKEN", "")
	if terraformToken == "" {
		return nil, fmt.Errorf("TFE_TOKEN environment variable is required for downloading state files")
	}

	// Create HTTP request with authorization
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Add authorization header
	req.Header.Set("Authorization", "Bearer "+terraformToken)

	// Make the request using standard HTTP client
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading state file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download state file: status %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading state file content: %w", err)
	}

	// Parse the JSON state file into a map for raw content
	var stateContent map[string]interface{}
	if err := json.Unmarshal(body, &stateContent); err != nil {
		return nil, fmt.Errorf("parsing state file JSON: %w", err)
	}

	// Log detailed information about what we received
	resourcesCount := 0
	outputsCount := 0
	
	if resources, ok := stateContent["resources"].([]interface{}); ok {
		resourcesCount = len(resources)
	}
	if outputs, ok := stateContent["outputs"].(map[string]interface{}); ok {
		outputsCount = len(outputs)
	}
	
	logger.WithFields(log.Fields{
		"state_size_bytes":    len(body),
		"top_level_keys":      getMapKeys(stateContent),
		"resources_count":     resourcesCount,
		"outputs_count":       outputsCount,
		"terraform_version":   stateContent["terraform_version"],
		"format_version":      stateContent["version"],
		"serial":              stateContent["serial"],
		"lineage":             stateContent["lineage"],
	}).Info("Successfully downloaded and parsed Terraform state file")

	// Debug: Log raw content size and structure for troubleshooting
	logger.WithFields(log.Fields{
		"raw_content_preview": fmt.Sprintf("%.200s...", string(body)),
	}).Debug("Raw state file content preview")

	return stateContent, nil
}

// getMapKeys returns a slice of keys from a map[string]interface{}
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
