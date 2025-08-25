// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// CreateAndUploadConfigurationWithStreaming creates the MCP tool for creating, uploading config, and streaming run status
func CreateAndUploadConfigurationWithStreaming(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_configuration_version_with_streaming",
			Description: "Creates a new configuration version, uploads files, and streams run status updates with polling every 5 seconds and 10-minute timeout",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create a configuration version for",
					},
					"configuration_files_base64": map[string]interface{}{
						"type":        "string",
						"description": "Base64-encoded tar.gz archive containing the Terraform configuration files",
					},
					"auto_queue_runs": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to automatically queue runs when the configuration version is uploaded (default: true)",
						"default":     true,
					},
					"speculative": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether this is a speculative configuration version (default: false)",
						"default":     false,
					},
					"polling_interval_seconds": map[string]interface{}{
						"type":        "integer",
						"description": "Polling interval in seconds (default: 5, min: 2, max: 30)",
						"minimum":     2,
						"maximum":     30,
						"default":     5,
					},
					"timeout_minutes": map[string]interface{}{
						"type":        "integer",
						"description": "Timeout in minutes after which streaming stops (default: 10, max: 60)",
						"minimum":     1,
						"maximum":     60,
						"default":     10,
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "configuration_files_base64"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createAndUploadConfigurationWithStreamingHandler(hcpClient, request, logger, ctx)
		},
	}
}

// createAndUploadConfigurationWithStreamingHandler handles the streaming configuration upload
func createAndUploadConfigurationWithStreamingHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger, ctx context.Context) (*mcp.CallToolResult, error) {
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

	configurationFilesBase64 := request.GetString("configuration_files_base64", "")
	if configurationFilesBase64 == "" {
		err := fmt.Errorf("configuration_files_base64 is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Extract optional parameters
	autoQueueRuns := request.GetBool("auto_queue_runs", true)
	speculative := request.GetBool("speculative", false)
	pollingIntervalSeconds := request.GetInt("polling_interval_seconds", 5)
	timeoutMinutes := request.GetInt("timeout_minutes", 10)

	// Validate polling interval
	if pollingIntervalSeconds < 2 || pollingIntervalSeconds > 30 {
		pollingIntervalSeconds = 5
		logger.Warnf("Invalid polling interval, using default: %d seconds", pollingIntervalSeconds)
	}

	// Validate timeout
	if timeoutMinutes < 1 || timeoutMinutes > 60 {
		timeoutMinutes = 10
		logger.Warnf("Invalid timeout, using default: %d minutes", timeoutMinutes)
	}

	logger.Infof("Starting configuration upload with streaming for workspace %s", workspaceID)

	// Step 1: Create configuration version
	createRequest := &hcp_terraform.ConfigurationVersionCreateRequest{
		Data: hcp_terraform.ConfigurationVersionCreateData{
			Type: "configuration-versions",
			Attributes: hcp_terraform.ConfigurationVersionCreateAttributes{
				AutoQueueRuns: autoQueueRuns,
				Speculative:   speculative,
			},
		},
	}

	configVersionResponse, err := hcpClient.CreateWorkspaceConfigurationVersion(token, workspaceID, createRequest)
	if err != nil {
		logger.Errorf("Configuration version creation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration version creation", err)
	}

	logger.Infof("Created configuration version: %s", configVersionResponse.Data.ID)

	// Step 2: Upload configuration files
	uploadURL := configVersionResponse.Data.Attributes.UploadURL
	if uploadURL == nil {
		err := fmt.Errorf("no upload URL provided in configuration version response")
		logger.Errorf("Upload failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration upload", err)
	}

	_, err = UploadHCPTerraformConfigurationFilesTool(hcpClient, *uploadURL, configurationFilesBase64)
	if err != nil {
		logger.Errorf("Configuration files upload failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration files upload", err)
	}

	logger.Infof("Successfully uploaded configuration files")

	// Step 3: Start streaming run status if auto_queue_runs is enabled
	result := map[string]interface{}{
		"configuration_version_id": configVersionResponse.Data.ID,
		"upload_status":           "success",
		"auto_queue_runs":         autoQueueRuns,
		"workspace_id":            workspaceID,
	}

	if !autoQueueRuns {
		// If auto_queue_runs is false, just return the configuration version info
		result["message"] = "Configuration uploaded successfully. No runs were automatically queued."
		jsonResponse, err := json.Marshal(result)
		if err != nil {
			logger.Errorf("Failed to marshal response: %v", err)
			return nil, utils.LogAndReturnError(logger, "response marshaling", err)
		}
		return mcp.NewToolResultText(string(jsonResponse)), nil
	}

	// Step 4: Stream run status updates
	logger.Infof("Starting run status streaming with %d second intervals", pollingIntervalSeconds)
	
	runUpdates, err := streamRunStatus(ctx, hcpClient, token, workspaceID, time.Duration(pollingIntervalSeconds)*time.Second, time.Duration(timeoutMinutes)*time.Minute, logger)
	if err != nil {
		logger.Errorf("Run streaming failed: %v", err)
		// Still return partial success since config was uploaded
		result["streaming_error"] = err.Error()
		result["message"] = "Configuration uploaded successfully, but run streaming failed"
	} else {
		result["run_updates"] = runUpdates
		result["message"] = "Configuration uploaded and run status streamed successfully"
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully completed configuration upload with streaming for workspace %s", workspaceID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

// streamRunStatus polls for run status updates and collects them
func streamRunStatus(ctx context.Context, hcpClient *hcp_terraform.Client, token, workspaceID string, pollingInterval, timeout time.Duration, logger *log.Logger) ([]map[string]interface{}, error) {
	var runUpdates []map[string]interface{}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	var lastRunID string
	var lastStatus string
	startTime := time.Now()

	logger.Infof("Starting run status polling for workspace %s", workspaceID)

	// Initial check for runs
	runs, err := hcpClient.ListRuns(token, workspaceID, &hcp_terraform.RunListOptions{
		PageSize:   1,
		PageNumber: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list initial runs: %w", err)
	}

	if len(runs.Data) > 0 {
		lastRunID = runs.Data[0].ID
		lastStatus = runs.Data[0].Attributes.Status
		
		update := map[string]interface{}{
			"timestamp":  time.Now().Format(time.RFC3339),
			"run_id":     lastRunID,
			"status":     lastStatus,
			"elapsed":    time.Since(startTime).String(),
			"event_type": "run_detected",
		}
		runUpdates = append(runUpdates, update)
		logger.Infof("Detected run %s with status: %s", lastRunID, lastStatus)
	}

	for {
		select {
		case <-timeoutCtx.Done():
			logger.Infof("Streaming timeout reached after %v", timeout)
			update := map[string]interface{}{
				"timestamp":  time.Now().Format(time.RFC3339),
				"event_type": "timeout",
				"message":    fmt.Sprintf("Streaming stopped after %v timeout", timeout),
				"elapsed":    time.Since(startTime).String(),
			}
			runUpdates = append(runUpdates, update)
			return runUpdates, nil

		case <-ticker.C:
			// Poll for the latest run
			currentRuns, err := hcpClient.ListRuns(token, workspaceID, &hcp_terraform.RunListOptions{
				PageSize:   1,
				PageNumber: 1,
			})
			if err != nil {
				logger.Warnf("Failed to poll runs: %v", err)
				continue
			}

			if len(currentRuns.Data) == 0 {
				// No runs yet, continue polling
				continue
			}

			currentRun := currentRuns.Data[0]
			currentRunID := currentRun.ID
			currentStatus := currentRun.Attributes.Status

			// Check if this is a new run or status change
			if currentRunID != lastRunID {
				// New run detected
				lastRunID = currentRunID
				lastStatus = currentStatus
				
				update := map[string]interface{}{
					"timestamp":  time.Now().Format(time.RFC3339),
					"run_id":     currentRunID,
					"status":     currentStatus,
					"elapsed":    time.Since(startTime).String(),
					"event_type": "new_run",
				}
				runUpdates = append(runUpdates, update)
				logger.Infof("New run detected: %s with status: %s", currentRunID, currentStatus)
			} else if currentStatus != lastStatus {
				// Status change for existing run
				lastStatus = currentStatus
				
				update := map[string]interface{}{
					"timestamp":   time.Now().Format(time.RFC3339),
					"run_id":      currentRunID,
					"status":      currentStatus,
					"elapsed":     time.Since(startTime).String(),
					"event_type":  "status_change",
					"prev_status": lastStatus,
				}
				runUpdates = append(runUpdates, update)
				logger.Infof("Run %s status changed to: %s", currentRunID, currentStatus)
			}

			// Check if run has reached a terminal state
			if isTerminalStatus(currentStatus) {
				logger.Infof("Run %s reached terminal status: %s", currentRunID, currentStatus)
				update := map[string]interface{}{
					"timestamp":  time.Now().Format(time.RFC3339),
					"run_id":     currentRunID,
					"status":     currentStatus,
					"elapsed":    time.Since(startTime).String(),
					"event_type": "terminal_status",
					"message":    fmt.Sprintf("Run completed with status: %s", currentStatus),
				}
				runUpdates = append(runUpdates, update)
				return runUpdates, nil
			}
		}
	}
}

// isTerminalStatus checks if a run status is terminal (final)
func isTerminalStatus(status string) bool {
	terminalStatuses := map[string]bool{
		"applied":         true,
		"discarded":       true,
		"errored":         true,
		"canceled":        true,
		"force_canceled":  true,
		"planned_and_finished": true,
	}
	return terminalStatuses[status]
}






