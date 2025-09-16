// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// CreateWorkspaceTags creates a tool to add tags to a workspace.
func CreateWorkspaceTags(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_workspace_tags",
			mcp.WithDescription("Add tags to a Terraform workspace."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
			mcp.WithString("tags", mcp.Required(), mcp.Description("Comma-separated list of tag names to add")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}
			tagsStr, err := request.RequireString("tags")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "tags is required", err)
			}

			tagNames := strings.Split(strings.TrimSpace(tagsStr), ",")
			var tags []*tfe.Tag
			for _, tagName := range tagNames {
				tagName = strings.TrimSpace(tagName)
				if tagName != "" {
					tags = append(tags, &tfe.Tag{Name: tagName})
				}
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			err = tfeClient.Workspaces.AddTags(ctx, workspace.ID, tfe.WorkspaceAddTagsOptions{
				Tags: tags,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "adding tags to workspace", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Added %d tags to workspace %s", len(tags), workspaceName)),
				},
			}, nil
		},
	}
}

// UpdateWorkspaceTags creates a tool to replace all tags on a workspace.
func UpdateWorkspaceTags(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("update_workspace_tags",
			mcp.WithDescription("Replace all tags on a Terraform workspace."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
			mcp.WithString("tags", mcp.Required(), mcp.Description("Comma-separated list of tag names to set")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}
			tagsStr, err := request.RequireString("tags")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "tags is required", err)
			}

			tagNames := strings.Split(strings.TrimSpace(tagsStr), ",")
			var tags []*tfe.Tag
			for _, tagName := range tagNames {
				tagName = strings.TrimSpace(tagName)
				if tagName != "" {
					tags = append(tags, &tfe.Tag{Name: tagName})
				}
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			// Read current workspace to get existing tags, then remove all and add new ones
			workspace, err := tfeClient.Workspaces.ReadWithOptions(ctx, orgName, workspaceName, &tfe.WorkspaceReadOptions{
				Include: []tfe.WSIncludeOpt{"tags"},
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			// Remove existing tags if any
			if len(workspace.Tags) > 0 {
				err = tfeClient.Workspaces.RemoveTags(ctx, workspace.ID, tfe.WorkspaceRemoveTagsOptions{
					Tags: workspace.Tags,
				})
				if err != nil {
					return nil, utils.LogAndReturnError(logger, "removing existing tags", err)
				}
			}

			// Add new tags
			if len(tags) > 0 {
				err = tfeClient.Workspaces.AddTags(ctx, workspace.ID, tfe.WorkspaceAddTagsOptions{
					Tags: tags,
				})
				if err != nil {
					return nil, utils.LogAndReturnError(logger, "adding new tags", err)
				}
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Updated workspace %s with %d tags", workspaceName, len(tags))),
				},
			}, nil
		},
	}
}

// ReadWorkspaceTags creates a tool to read tags from a workspace.
func ReadWorkspaceTags(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("read_workspace_tags",
			mcp.WithDescription("Read all tags from a Terraform workspace."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			var tagNames []string
			for _, tag := range workspace.Tags {
				tagNames = append(tagNames, tag.Name)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Workspace %s has %d tags: %s", workspaceName, len(tagNames), strings.Join(tagNames, ", "))),
				},
			}, nil
		},
	}
}

// DeleteWorkspaceTags creates a tool to remove tags from a workspace.
func DeleteWorkspaceTags(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_workspace_tags",
			mcp.WithDescription("Remove tags from a Terraform workspace."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
			mcp.WithString("tags", mcp.Required(), mcp.Description("Comma-separated list of tag names to remove")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}
			tagsStr, err := request.RequireString("tags")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "tags is required", err)
			}

			tagNames := strings.Split(strings.TrimSpace(tagsStr), ",")
			var tags []*tfe.Tag
			for _, tagName := range tagNames {
				tagName = strings.TrimSpace(tagName)
				if tagName != "" {
					tags = append(tags, &tfe.Tag{Name: tagName})
				}
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			err = tfeClient.Workspaces.RemoveTags(ctx, workspace.ID, tfe.WorkspaceRemoveTagsOptions{
				Tags: tags,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "removing tags from workspace", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Removed %d tags from workspace %s", len(tags), workspaceName)),
				},
			}, nil
		},
	}
}
