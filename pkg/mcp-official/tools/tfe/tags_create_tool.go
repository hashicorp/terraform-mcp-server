// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateWorkspaceTagsArguments holds the required inputs for adding tags to a workspace.
type CreateWorkspaceTagsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The name of the Terraform Cloud/Enterprise Workspace"`
	Tags             string `json:"tags" jsonschema:"Comma-separated list of tag names to add, for key-value tags use key:value"`
}

// CreateWorkspaceTagsResult reports the tags that were added, in the same key:value form
// read_workspace_tags returns, so a caller can confirm how its tag string was parsed.
type CreateWorkspaceTagsResult struct {
	WorkspaceName string   `json:"workspace_name"`
	TagsAdded     []string `json:"tags_added"`
}

func CreateWorkspaceTagsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_workspace_tags",
		Description: "Add tags to a Terraform workspace.",
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"workspace_name": {Type: "string"},
				"tags_added": {
					Type:  "array",
					Items: &jsonschema.Schema{Type: "string"},
				},
			},
			PropertyOrder:        []string{"workspace_name", "tags_added"},
			Required:             []string{"workspace_name", "tags_added"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create Terraform workspace tags",
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func CreateWorkspaceTagsFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateWorkspaceTagsArguments) (*mcp.CallToolResult, *CreateWorkspaceTagsResult, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	tagsInput := strings.TrimSpace(input.Tags)

	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}
	if tagsInput == "" {
		return nil, nil, fmt.Errorf("tags must not be blank")
	}

	// Separators alone parse to nothing, and adding no tags would report success for a
	// call that changed nothing.
	bindings := parseTagBindings(tagsInput)
	if len(bindings) == 0 {
		return nil, nil, fmt.Errorf("tags %q contained no valid tag names", input.Tags)
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace %q not found in org %q: %w", workspaceName, terraformOrgName, err)
	}

	if _, err := tfeClient.Workspaces.AddTagBindings(ctx, workspace.ID, tfe.WorkspaceAddTagBindingsOptions{
		TagBindings: bindings,
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to add tags to workspace %q: %w", workspaceName, err)
	}

	tagsAdded := make([]string, len(bindings))
	for i, binding := range bindings {
		tagsAdded[i] = formatTagBinding(binding)
	}

	return nil, &CreateWorkspaceTagsResult{
		WorkspaceName: workspaceName,
		TagsAdded:     tagsAdded,
	}, nil
}
