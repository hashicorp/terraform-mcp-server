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
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("Organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("Workspace name"),
			),
			mcp.WithString("tags",
				mcp.Required(),
				mcp.Description("Comma-separated list of tag names to add, for key-value tags use key:value"),
			),
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
			var tags []*tfe.TagBinding
			for _, tagName := range tagNames {
				tagName = strings.TrimSpace(tagName)
				// Support key:value format for key-value tags
				if strings.Contains(tagName, ":") {
					parts := strings.SplitN(tagName, ":", 2)
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if key != "" {
						tags = append(tags, &tfe.TagBinding{Key: key, Value: value})
					}
					continue
				}
				// Otherwise treat as a tag with only a key
				if tagName != "" {
					tags = append(tags, &tfe.TagBinding{Key: tagName})
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

			_, err = tfeClient.Workspaces.AddTagBindings(ctx, workspace.ID, tfe.WorkspaceAddTagBindingsOptions{
				TagBindings: tags,
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

// ReadWorkspaceTags creates a tool to read tags from a workspace.
func ReadWorkspaceTags(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("read_workspace_tags",
			mcp.WithDescription("Read all tags from a Terraform workspace."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("Organization name"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("Workspace name"),
			),
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
			tags, err := tfeClient.Workspaces.ListTags(ctx, workspace.ID, nil)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "listing tags", err)
			}
			for _, tag := range tags.Items {
				tagNames = append(tagNames, tag.Name)
			}

			var tagBindings []string
			bindings, err := tfeClient.Workspaces.ListTagBindings(ctx, workspace.ID)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "listing tag bindings", err)
			}
			for _, binding := range bindings {
				if binding.Value != "" {
					tagBindings = append(tagBindings, fmt.Sprintf("%s:%s", binding.Key, binding.Value))
				} else {
					tagBindings = append(tagBindings, binding.Key)
				}
			}

			tagResponse := fmt.Sprintf("Workspace %s has %d tags: %s", workspaceName, len(tagNames), strings.Join(tagNames, ", "))
			if len(tagBindings) > 0 {
				tagResponse += fmt.Sprintf("Workspace %s has %d tag bindings: %s", workspaceName, len(tagBindings), strings.Join(tagBindings, ", "))
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(tagResponse),
				},
			}, nil
		},
	}
}
