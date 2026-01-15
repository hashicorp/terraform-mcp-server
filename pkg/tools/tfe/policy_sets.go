// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MatchingPolicySet represents a policy set that applies to a workspace.
type MatchingPolicySet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Global      bool   `json:"global"`
	Reason      string `json:"reason"`
}

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

// ReadWorkspacePolicySets creates a tool to read all policy sets attached to a workspace.
func ReadWorkspacePolicySets(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("read_workspace_policy_sets",
			mcp.WithDescription("Read all policy sets attached to a workspace. Returns both directly attached policy sets and global policy sets that apply to all workspaces."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID to get policy sets for (e.g., ws-2HRvNs49EWPjDqT1)")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return readWorkspacePolicySetsHandler(ctx, request, logger)
		},
	}
}

func readWorkspacePolicySetsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	orgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	workspaceID, err := request.RequireString("workspace_id")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// Paginate through all policy sets with the workspaces included
	var matchingPolicySets []*MatchingPolicySet
	pageNumber := 1

	for {
		policySets, err := tfeClient.PolicySets.List(ctx, orgName, &tfe.PolicySetListOptions{
			Include: []tfe.PolicySetIncludeOpt{tfe.PolicySetWorkspaces},
			ListOptions: tfe.ListOptions{
				PageNumber: pageNumber,
				PageSize:   100,
			},
		})
		if err != nil {
			return ToolErrorf(logger, "failed to list policy sets for org '%s': %v", orgName, err)
		}

		// Filter policy sets that apply to this workspace
		for _, ps := range policySets.Items {
			applies := false
			reason := ""

			// Global policy sets apply to all workspaces
			if ps.Global {
				applies = true
				reason = "global"
			} else {
				for _, ws := range ps.Workspaces {
					if ws.ID == workspaceID {
						applies = true
						reason = "directly attached"
						break
					}
				}
			}

			if applies {
				matchingPolicySets = append(matchingPolicySets, &MatchingPolicySet{
					ID:          ps.ID,
					Name:        ps.Name,
					Description: ps.Description,
					Kind:        string(ps.Kind),
					Global:      ps.Global,
					Reason:      reason,
				})
			}
		}

		// Check if there are more pages
		if policySets.NextPage == 0 {
			break
		}
		pageNumber++
	}

	if len(matchingPolicySets) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(fmt.Sprintf("No policy sets are attached to workspace %s", workspaceID)),
			},
		}, nil
	}

	result, err := json.MarshalIndent(matchingPolicySets, "", "  ")
	if err != nil {
		return ToolError(logger, "failed to marshal policy sets", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(string(result)),
		},
	}, nil
}
