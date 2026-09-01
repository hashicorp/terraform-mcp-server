// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed static/default-cli-run.md
var defaultWorkspaceReadme string

// GetWorkspaceDetailsArguments holds the input parameters for retrieving workspace details.
type GetWorkspaceDetailsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The name of the workspace to get details for"`
}

func GetWorkspaceDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_workspace_details",
		Description: "Fetches detailed information about a specific Terraform workspace, including configuration, variables, and current state information.",
		Annotations: &mcp.ToolAnnotations{Title: "Get detailed information about a Terraform workspace", OpenWorldHint: ptr(true), ReadOnlyHint: true, DestructiveHint: ptr(false)},
	}
}

func GetWorkspaceDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetWorkspaceDetailsArguments) (*mcp.CallToolResult, any, error) {
	orgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if orgName == "" || workspaceName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name and workspace_name are required")
	}
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}
	workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace %q not found in org %q: %w", workspaceName, orgName, err)
	}

	readme := strings.ReplaceAll(defaultWorkspaceReadme, "<<your-terraform-org>>", orgName)
	readme = strings.ReplaceAll(readme, "<<your-terraform-workspace>>", workspace.Name)
	if reader, err := tfeClient.Workspaces.Readme(ctx, workspace.ID); err == nil && reader != nil {
		if content, err := io.ReadAll(reader); err == nil && len(content) > 0 {
			readme = string(content)
		}
	}

	var workspaceVariables []*tfe.Variable
	if vars, err := tfeClient.Variables.List(ctx, workspace.ID, &tfe.VariableListOptions{}); err == nil {
		workspaceVariables = vars.Items
	}

	payload := &tfeclient.WorkspaceToolResponse{
		Type:        "get_workspace_details",
		Success:     true,
		WorkspaceID: workspace.ID,
		Workspace:   workspace,
		Variables:   workspaceVariables,
		Readme:      readme,
	}

	var buf bytes.Buffer
	if err := jsonapi.MarshalPayload(&buf, payload); err != nil {
		return nil, nil, fmt.Errorf("failed to marshal workspace: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: buf.String()}},
	}, nil, nil
}
