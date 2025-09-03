package hcp_terraform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetHCPTerraformConfigurationVersionsTool retrieves configuration versions for a workspace
func GetHCPTerraformConfigurationVersionsTool(client *hcp_terraform.Client, authToken, workspaceID string, pageNumber, pageSize int) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Call client method
	response, err := client.GetWorkspaceConfigurationVersions(authToken, workspaceID, pageNumber, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration versions: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"configuration_versions": response.Data,
		"total_count":            len(response.Data),
	}

	// Add pagination info if available
	if response.Meta != nil {
		result["pagination"] = map[string]interface{}{
			"current_page": response.Meta.Pagination.CurrentPage,
			"total_pages":  response.Meta.Pagination.TotalPages,
			"total_count":  response.Meta.Pagination.TotalCount,
		}
	}

	return result, nil
}

// CreateHCPTerraformConfigurationVersionTool creates a new configuration version for a workspace
func CreateHCPTerraformConfigurationVersionTool(client *hcp_terraform.Client, authToken, workspaceID string, autoQueueRuns, speculative bool) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Create request
	request := &hcp_terraform.ConfigurationVersionCreateRequest{
		Data: hcp_terraform.ConfigurationVersionCreateData{
			Type: "configuration-versions",
			Attributes: hcp_terraform.ConfigurationVersionCreateAttributes{
				AutoQueueRuns: autoQueueRuns,
				Speculative:   speculative,
			},
		},
	}

	// Call client method
	response, err := client.CreateWorkspaceConfigurationVersion(authToken, workspaceID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration version: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"configuration_version": response.Data,
		"message":               fmt.Sprintf("Created configuration version %s for workspace %s", response.Data.ID, workspaceID),
	}

	// Include upload URL if available
	if response.Data.Attributes.UploadURL != nil {
		result["upload_url"] = *response.Data.Attributes.UploadURL
		result["upload_instructions"] = "Use the upload_url with the upload_hcp_terraform_configuration_files tool to upload your configuration files"
	}

	return result, nil
}

// DownloadHCPTerraformConfigurationFilesTool downloads configuration files from a configuration version
func DownloadHCPTerraformConfigurationFilesTool(client *hcp_terraform.Client, authToken, configurationVersionID string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if configurationVersionID == "" {
		return nil, fmt.Errorf("configuration_version_id is required")
	}

	// Call client method
	configFiles, err := client.DownloadConfigurationVersion(authToken, configurationVersionID)
	if err != nil {
		return nil, fmt.Errorf("failed to download configuration files: %v", err)
	}

	// Encode binary content as base64 for JSON response
	encodedContent := base64.StdEncoding.EncodeToString(configFiles)

	// Format response for user
	result := map[string]interface{}{
		"configuration_version_id": configurationVersionID,
		"file_size":                len(configFiles),
		"content_encoding":         "base64",
		"content":                  encodedContent,
		"message":                  fmt.Sprintf("Downloaded configuration files for version %s (%d bytes)", configurationVersionID, len(configFiles)),
		"usage_instructions":       "The content is base64 encoded. Decode it to get the tar.gz archive containing the Terraform configuration files.",
	}

	return result, nil
}

// UploadHCPTerraformConfigurationFilesTool uploads configuration files to a configuration version
func UploadHCPTerraformConfigurationFilesTool(client *hcp_terraform.Client, uploadURL, configurationFilesBase64 string) (map[string]interface{}, error) {
	// Validate required parameters
	if uploadURL == "" {
		return nil, fmt.Errorf("upload_url is required")
	}
	if configurationFilesBase64 == "" {
		return nil, fmt.Errorf("configuration_files_base64 is required")
	}

	// Decode base64 content
	configFiles, err := base64.StdEncoding.DecodeString(configurationFilesBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 configuration files: %v", err)
	}

	// Call client method
	err = client.UploadConfigurationFiles(uploadURL, configFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to upload configuration files: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"upload_url": uploadURL,
		"file_size":  len(configFiles),
		"message":    fmt.Sprintf("Successfully uploaded configuration files (%d bytes)", len(configFiles)),
		"status":     "success",
	}

	return result, nil
}

// parseOptionalInt parses an optional integer parameter
func parseOptionalInt(value interface{}) (int, error) {
	if value == nil {
		return 0, nil
	}

	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		if v == "" {
			return 0, nil
		}
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("invalid integer value: %v", value)
	}
}

// parseOptionalBool parses an optional boolean parameter
func parseOptionalBool(value interface{}) (bool, error) {
	if value == nil {
		return false, nil
	}

	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		if v == "" {
			return false, nil
		}
		return strconv.ParseBool(strings.ToLower(v))
	default:
		return false, fmt.Errorf("invalid boolean value: %v", value)
	}
}

// GetConfigurationVersions creates the MCP tool for getting configuration versions for a workspace
func GetConfigurationVersions(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_configuration_versions",
			Description: "Fetches configuration versions for an HCP Terraform workspace with pagination support",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to list configuration versions for",
					},
					"page_number": map[string]interface{}{
						"type":        "integer",
						"description": "Page number for pagination (optional)",
					},
					"page_size": map[string]interface{}{
						"type":        "integer",
						"description": "Number of configuration versions per page (optional)",
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
			return getConfigurationVersionsHandler(hcpClient, request, logger)
		},
	}
}

// CreateConfigurationVersion creates the MCP tool for creating a new configuration version
func CreateConfigurationVersion(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_configuration_version",
			Description: "Creates a new configuration version for an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create a configuration version for",
					},
					"auto_queue_runs": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to automatically queue runs when the configuration version is uploaded (default: false)",
					},
					"speculative": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether this is a speculative configuration version (default: false)",
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
			return createConfigurationVersionHandler(hcpClient, request, logger)
		},
	}
}

// DownloadConfigurationFiles creates the MCP tool for downloading configuration files
func DownloadConfigurationFiles(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "download_hcp_terraform_configuration_files",
			Description: "Downloads configuration files from an HCP Terraform configuration version as a base64-encoded tar.gz archive",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"configuration_version_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the configuration version to download files from",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"configuration_version_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return downloadConfigurationFilesHandler(hcpClient, request, logger)
		},
	}
}

// UploadConfigurationFiles creates the MCP tool for uploading configuration files
func UploadConfigurationFiles(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "upload_hcp_terraform_configuration_files",
			Description: "Uploads configuration files to an HCP Terraform configuration version from a base64-encoded tar.gz archive",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"upload_url": map[string]interface{}{
						"type":        "string",
						"description": "The upload URL obtained from creating a configuration version",
					},
					"configuration_files_base64": map[string]interface{}{
						"type":        "string",
						"description": "Base64-encoded tar.gz archive containing the Terraform configuration files",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"upload_url", "configuration_files_base64", "authorization"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return uploadConfigurationFilesHandler(hcpClient, request, logger)
		},
	}
}

// Handler implementations

func getConfigurationVersionsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Extract optional pagination parameters
	pageNumber := request.GetInt("page_number", 0)
	pageSize := request.GetInt("page_size", 0)

	// Call the tool function
	result, err := GetHCPTerraformConfigurationVersionsTool(hcpClient, token, workspaceID, pageNumber, pageSize)
	if err != nil {
		logger.Errorf("Configuration versions retrieval failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration versions retrieval", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully retrieved configuration versions for workspace %s", workspaceID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

func createConfigurationVersionHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	// Extract optional boolean parameters
	autoQueueRuns := request.GetBool("auto_queue_runs", false)
	speculative := request.GetBool("speculative", false)

	// Call the tool function
	result, err := CreateHCPTerraformConfigurationVersionTool(hcpClient, token, workspaceID, autoQueueRuns, speculative)
	if err != nil {
		logger.Errorf("Configuration version creation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration version creation", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully created configuration version for workspace %s", workspaceID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

func downloadConfigurationFilesHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	configurationVersionID := request.GetString("configuration_version_id", "")
	if configurationVersionID == "" {
		err := fmt.Errorf("configuration_version_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := DownloadHCPTerraformConfigurationFilesTool(hcpClient, token, configurationVersionID)
	if err != nil {
		logger.Errorf("Configuration files download failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration files download", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully downloaded configuration files for version %s", configurationVersionID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

func uploadConfigurationFilesHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get required parameters
	uploadURL := request.GetString("upload_url", "")
	if uploadURL == "" {
		err := fmt.Errorf("upload_url is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	configurationFilesBase64 := request.GetString("configuration_files_base64", "")
	if configurationFilesBase64 == "" {
		err := fmt.Errorf("configuration_files_base64 is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := UploadHCPTerraformConfigurationFilesTool(hcpClient, uploadURL, configurationFilesBase64)
	if err != nil {
		logger.Errorf("Configuration files upload failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "configuration files upload", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully uploaded configuration files")
	return mcp.NewToolResultText(string(jsonResponse)), nil
}
