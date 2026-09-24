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

// PolicySetsSummary holds a trimmed view of a single policy set that
// applies to a workspace
type PolicySetsSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Global      bool   `json:"global"`
	Reason      string `json:"reason"`
}

// PolicySetsSummaryList contains the policy sets that apply to a workspace.
// The handler aggregates every page into one list, so no pagination details are reported.
type PolicySetsSummaryList struct {
	Items []PolicySetsSummary `json:"items"`
}

// PolicySetsArguments holds the required inputs for listing policy sets attached to a workspace.
type PolicySetsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceID      string `json:"workspace_id" jsonschema:"The workspace ID to get policy sets for (e.g., ws-2HRvNs49EWPjDqT1)"`
}

func ListWorkspacePolicySetsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_workspace_policy_sets",
		Description: "Read all policy sets attached to a workspace. Returns both directly attached policy sets and global policy sets that apply to all workspaces.",
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"items": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"id":          {Type: "string"},
							"name":        {Type: "string"},
							"description": {Type: "string"},
							"kind":        {Type: "string"},
							"global":      {Type: "boolean"},
							"reason":      {Type: "string"},
						},
						PropertyOrder:        []string{"id", "name", "description", "kind", "global", "reason"},
						Required:             []string{"id", "name", "description", "kind", "global", "reason"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
			},
			PropertyOrder:        []string{"items"},
			Required:             []string{"items"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform workspaces policy sets",
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

// ListWorkspacePolicySetsFunc returns every policy set that applies to the workspace,
// directly attached or global.
func ListWorkspacePolicySetsFunc(ctx context.Context, request *mcp.CallToolRequest, input PolicySetsArguments) (*mcp.CallToolResult, *PolicySetsSummaryList, error) {
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

	// A workspace belongs to exactly one org and policy sets are org-scoped, so a
	// mismatch would report this org's global policy sets against a foreign workspace.
	if workspace.Organization != nil && !strings.EqualFold(workspace.Organization.Name, terraformOrgName) {
		return nil, nil, fmt.Errorf("workspace %q belongs to organization %q, not %q",
			workspaceID, workspace.Organization.Name, terraformOrgName)
	}

	// Paginate through all policy sets with the workspaces included. The slice starts
	// empty rather than nil so an unmatched workspace marshals as [] instead of null.
	matchingSets := []PolicySetsSummary{}
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
				matchingSets = append(matchingSets, PolicySetsSummary{
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

	return nil, &PolicySetsSummaryList{Items: matchingSets}, nil
}
