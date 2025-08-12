package hcp_terraform

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetHCPTerraformCurrentStateVersionTool retrieves the current state version for a workspace
func GetHCPTerraformCurrentStateVersionTool(client *hcp_terraform.Client, authToken, workspaceID string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	// Call client method
	response, err := client.GetCurrentStateVersion(authToken, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current state version: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"state_version": response.Data,
		"message":       fmt.Sprintf("Retrieved current state version %s for workspace %s", response.Data.ID, workspaceID),
	}

	return result, nil
}

// DownloadHCPTerraformStateVersionTool downloads state data from a state version
func DownloadHCPTerraformStateVersionTool(client *hcp_terraform.Client, authToken, stateVersionID string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if stateVersionID == "" {
		return nil, fmt.Errorf("state_version_id is required")
	}

	// Call client method
	stateData, err := client.DownloadStateVersion(authToken, stateVersionID)
	if err != nil {
		return nil, fmt.Errorf("failed to download state version: %v", err)
	}

	// Encode binary content as base64 for JSON response
	encodedContent := base64.StdEncoding.EncodeToString(stateData)

	// Format response for user
	result := map[string]interface{}{
		"state_version_id":   stateVersionID,
		"file_size":          len(stateData),
		"content_encoding":   "base64",
		"content":            encodedContent,
		"message":            fmt.Sprintf("Downloaded state data for version %s (%d bytes)", stateVersionID, len(stateData)),
		"usage_instructions": "The content is base64 encoded. Decode it to get the raw state file content.",
	}

	return result, nil
}

// CreateHCPTerraformStateVersionTool creates a new state version for a workspace
func CreateHCPTerraformStateVersionTool(client *hcp_terraform.Client, authToken, workspaceID, stateContentBase64 string, serial int, lineage string) (map[string]interface{}, error) {
	// Validate required parameters
	if authToken == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if stateContentBase64 == "" {
		return nil, fmt.Errorf("state_content_base64 is required")
	}
	if serial < 0 {
		return nil, fmt.Errorf("serial must be non-negative")
	}

	// Decode base64 content to calculate MD5
	stateContent, err := base64.StdEncoding.DecodeString(stateContentBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 state content: %v", err)
	}

	// Calculate MD5 hash
	hasher := md5.New()
	hasher.Write(stateContent)
	md5Hash := hex.EncodeToString(hasher.Sum(nil))

	// Create request
	attributes := hcp_terraform.StateVersionCreateAttributes{
		Serial: serial,
		MD5:    md5Hash,
		State:  stateContentBase64,
	}

	// Add lineage if provided
	if lineage != "" {
		attributes.Lineage = &lineage
	}

	request := &hcp_terraform.StateVersionCreateRequest{
		Data: hcp_terraform.StateVersionCreateData{
			Type:       "state-versions",
			Attributes: attributes,
		},
	}

	// Call client method
	response, err := client.CreateStateVersion(authToken, workspaceID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to create state version: %v", err)
	}

	// Format response for user
	result := map[string]interface{}{
		"state_version": response.Data,
		"message":       fmt.Sprintf("Created state version %s for workspace %s", response.Data.ID, workspaceID),
	}

	return result, nil
}

// GetCurrentStateVersion creates the MCP tool for getting the current state version
func GetCurrentStateVersion(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_hcp_terraform_current_state_version",
			Description: "Fetches the current state version for an HCP Terraform workspace",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to get the current state version for",
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
			return getCurrentStateVersionHandler(hcpClient, request, logger)
		},
	}
}

// DownloadStateVersion creates the MCP tool for downloading state data
func DownloadStateVersion(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "download_hcp_terraform_state_version",
			Description: "Downloads state data from an HCP Terraform state version as base64-encoded content",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"state_version_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the state version to download data from",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"state_version_id"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return downloadStateVersionHandler(hcpClient, request, logger)
		},
	}
}

// CreateStateVersion creates the MCP tool for creating a new state version
func CreateStateVersion(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_hcp_terraform_state_version",
			Description: "Creates a new state version for an HCP Terraform workspace from base64-encoded state content",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"workspace_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the workspace to create a state version for",
					},
					"state_content_base64": map[string]interface{}{
						"type":        "string",
						"description": "Base64-encoded state file content",
					},
					"serial": map[string]interface{}{
						"type":        "integer",
						"description": "The serial number for the state version",
					},
					"lineage": map[string]interface{}{
						"type":        "string",
						"description": "Optional lineage for the state version",
					},
					"authorization": map[string]interface{}{
						"type":        "string",
						"description": "Optional Bearer token for authentication (if not provided via HCP_TERRAFORM_TOKEN environment variable)",
					},
				},
				Required: []string{"workspace_id", "state_content_base64", "serial"},
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createStateVersionHandler(hcpClient, request, logger)
		},
	}
}

// Handler implementations

func getCurrentStateVersionHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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
	result, err := GetHCPTerraformCurrentStateVersionTool(hcpClient, token, workspaceID)
	if err != nil {
		logger.Errorf("Current state version retrieval failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "current state version retrieval", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully retrieved current state version for workspace %s", workspaceID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

func downloadStateVersionHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Resolve authentication token
	token, err := resolveToken(request)
	if err != nil {
		logger.Errorf("Token resolution failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "token resolution", err)
	}

	// Get required parameters
	stateVersionID := request.GetString("state_version_id", "")
	if stateVersionID == "" {
		err := fmt.Errorf("state_version_id is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	// Call the tool function
	result, err := DownloadHCPTerraformStateVersionTool(hcpClient, token, stateVersionID)
	if err != nil {
		logger.Errorf("State version download failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "state version download", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully downloaded state version %s", stateVersionID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}

func createStateVersionHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	stateContentBase64 := request.GetString("state_content_base64", "")
	if stateContentBase64 == "" {
		err := fmt.Errorf("state_content_base64 is required")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	serial := request.GetInt("serial", -1)
	if serial < 0 {
		err := fmt.Errorf("serial is required and must be non-negative")
		logger.Errorf("Validation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "parameter validation", err)
	}

	lineage := request.GetString("lineage", "")

	// Call the tool function
	result, err := CreateHCPTerraformStateVersionTool(hcpClient, token, workspaceID, stateContentBase64, serial, lineage)
	if err != nil {
		logger.Errorf("State version creation failed: %v", err)
		return nil, utils.LogAndReturnError(logger, "state version creation", err)
	}

	// Return JSON response
	jsonResponse, err := json.Marshal(result)
	if err != nil {
		logger.Errorf("Failed to marshal response: %v", err)
		return nil, utils.LogAndReturnError(logger, "response marshaling", err)
	}

	logger.Infof("Successfully created state version for workspace %s", workspaceID)
	return mcp.NewToolResultText(string(jsonResponse)), nil
}
