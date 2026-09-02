// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListWorkspacePolicySetsSummary holds a trimmed view of a single policy set that
// applies to a workspace, along with the reason it applies.
type ListWorkspacePolicySetsSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Global      bool   `json:"global"`
	Reason      string `json:"reason"`
}

// ListWorkspacePolicySetsSummaryList contains the policy sets that apply to a workspace.
// The handler aggregates every page into one list, so no pagination details are reported.
type ListWorkspacePolicySetsSummaryList struct {
	Items []*ListWorkspacePolicySetsSummary `json:"items"`
}

// ListWorkspacePolicySetsArguments holds the required inputs for listing policy sets attached to a workspace.
type ListWorkspacePolicySetsArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceID      string `json:"workspace_id" jsonschema:"The workspace ID to get policy sets for (e.g., ws-2HRvNs49EWPjDqT1)"`
}

func ListWorkspacePolicySetsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_workspace_policy_sets",
		Description: "Read all policy sets attached to a workspace. Returns both directly attached policy sets and global policy sets that apply to all workspaces.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform workspaces policy sets",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListWorkspacePolicySetsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListWorkspacePolicySetsArguments) (*mcp.CallToolResult, *ListWorkspacePolicySetsSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceID := strings.TrimSpace(input.WorkspaceID)

	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	if workspaceID == "" {
		return nil, nil, fmt.Errorf("workspace_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.ReadByID(ctx, workspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace not found %q: %w", workspaceID, err)
	}

	if workspace.Organization != nil && !strings.EqualFold(workspace.Organization.Name, terraformOrgName) {
		return nil, nil, fmt.Errorf("workspace %q belongs to organization %q, not %q",
			workspaceID, workspace.Organization.Name, terraformOrgName)
	}

	var matchingSets []*ListWorkspacePolicySetsSummary
	pageNumber := 1

	for {
		policySets, err := tfeClient.PolicySets.List(ctx, terraformOrgName, &tfe.PolicySetListOptions{
			Include: []tfe.PolicySetIncludeOpt{tfe.PolicySetWorkspaces},
			ListOptions: tfe.ListOptions{
				PageNumber: pageNumber,
				PageSize:   100,
			},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list policy sets for org %q: %w", terraformOrgName, err)
		}

		for _, ps := range policySets.Items {
			applies, reason := false, ""
			if ps.Global {
				applies, reason = true, "global"
			} else {
				for _, ws := range ps.Workspaces {
					if ws.ID == workspaceID {
						applies, reason = true, "directly attached"
						break
					}
				}
			}
			if applies {
				matchingSets = append(matchingSets, &ListWorkspacePolicySetsSummary{
					ID:          ps.ID,
					Name:        ps.Name,
					Description: ps.Description,
					Kind:        string(ps.Kind),
					Global:      ps.Global,
					Reason:      reason,
				})
			}
		}

		if policySets.NextPage == 0 {
			break
		}
		pageNumber++
	}

	// Explicit Content suppresses the SDK's marshalled-JSON fallback text, and the empty
	// (non-nil) slice serializes as [] rather than null.
	if len(matchingSets) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{
				Text: fmt.Sprintf("No policy sets are attached to workspace %q", workspaceID),
			}},
		}, &ListWorkspacePolicySetsSummaryList{Items: []*ListWorkspacePolicySetsSummary{}}, nil
	}

	return nil, &ListWorkspacePolicySetsSummaryList{Items: matchingSets}, nil
}
