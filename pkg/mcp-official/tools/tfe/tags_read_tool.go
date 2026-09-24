// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReadWorkspaceTagsArguments holds the required inputs for reading tags from a workspace.
type ReadWorkspaceTagsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The name of the Terraform Cloud/Enterprise Workspace"`
}

// ReadWorkspaceTagsResult holds the tags and tag bindings for a workspace. The workspace
// name is echoed back so a caller can confirm which workspace the tags belong to.
type ReadWorkspaceTagsResult struct {
	WorkspaceName string   `json:"workspace_name"`
	Tags          []string `json:"tags"`
	TagBindings   []string `json:"tag_bindings"`
}

func ReadWorkspaceTagsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "read_workspace_tags",
		Description: "Read all tags from a Terraform workspace.",
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"workspace_name": {Type: "string"},
				"tags": {
					Type:  "array",
					Items: &jsonschema.Schema{Type: "string"},
				},
				"tag_bindings": {
					Type:  "array",
					Items: &jsonschema.Schema{Type: "string"},
				},
			},
			PropertyOrder:        []string{"workspace_name", "tags", "tag_bindings"},
			Required:             []string{"workspace_name", "tags", "tag_bindings"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Read Terraform workspace tags",
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ReadWorkspaceTagsFunc(ctx context.Context, request *mcp.CallToolRequest, input ReadWorkspaceTagsArguments) (*mcp.CallToolResult, *ReadWorkspaceTagsResult, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace %q not found in org %q: %w", workspaceName, terraformOrgName, err)
	}

	tags, err := tfeClient.Workspaces.ListTags(ctx, workspace.ID, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list tags for workspace %q: %w", workspaceName, err)
	}

	// Both lists start empty rather than nil so an untagged workspace reports [] instead
	// of null.
	tagNames := []string{}
	for _, tag := range tags.Items {
		tagNames = append(tagNames, tag.Name)
	}

	bindings, err := tfeClient.Workspaces.ListTagBindings(ctx, workspace.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list tag bindings for workspace %q: %w", workspaceName, err)
	}

	tagBindings := []string{}
	for _, binding := range bindings {
		tagBindings = append(tagBindings, formatTagBinding(binding))
	}

	return nil, &ReadWorkspaceTagsResult{
		WorkspaceName: workspaceName,
		Tags:          tagNames,
		TagBindings:   tagBindings,
	}, nil
}
