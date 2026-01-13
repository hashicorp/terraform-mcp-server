// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// AttachPolicySetToWorkspaces creates a tool to attach a policy set to workspaces.
func AttachPolicySetToWorkspaces(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("attach_policy_set_to_workspaces",
			mcp.WithDescription("Attach a policy set to one or more workspaces. Note: Policy sets marked as global cannot be attached to individual workspaces."),
			mcp.WithString("policy_set_id", mcp.Required(), mcp.Description("The ID of the policy set to attach (e.g., polset-3yVQZvHzf5j3WRJ1)")),
			mcp.WithString("workspace_ids", mcp.Required(), mcp.Description("Comma-separated list of workspace IDs to attach the policy set to")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			policySetID, err := request.RequireString("policy_set_id")
			if err != nil {
				return ToolError(logger, "missing required input: policy_set_id", err)
			}
			workspaceIDsStr, err := request.RequireString("workspace_ids")
			if err != nil {
				return ToolError(logger, "missing required input: workspace_ids", err)
			}

			workspaceIDsList := strings.Split(workspaceIDsStr, ",")
			var workspaces []*tfe.Workspace
			for _, id := range workspaceIDsList {
				trimmedID := strings.TrimSpace(id)
				if trimmedID != "" {
					workspaces = append(workspaces, &tfe.Workspace{ID: trimmedID})
				}
			}

			if len(workspaces) == 0 {
				return ToolError(logger, "no valid workspace IDs provided", nil)
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return ToolError(logger, "failed to get Terraform client", err)
			}

			err = tfeClient.PolicySets.AddWorkspaces(ctx, policySetID, tfe.PolicySetAddWorkspacesOptions{
				Workspaces: workspaces,
			})
			if err != nil {
				return ToolErrorf(logger, "failed to attach policy set '%s' to workspaces: %v", policySetID, err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Successfully attached policy set %s to %d workspace(s)", policySetID, len(workspaces))),
				},
			}, nil
		},
	}
}
