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

// PolicySetDetails represents detailed information about a policy set.
type PolicySetDetails struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Description       string                  `json:"description"`
	Kind              string                  `json:"kind"`
	Global            bool                    `json:"global"`
	Overridable       bool                    `json:"overridable"`
	PoliciesPath      string                  `json:"policies_path,omitempty"`
	PolicyToolVersion string                  `json:"policy_tool_version,omitempty"`
	VCSRepo           *VCSRepoInfo            `json:"vcs_repo,omitempty"`
	Policies          []PolicyInfo            `json:"policies"`
	Workspaces        []WorkspaceInfo         `json:"workspaces"`
	Projects          []ProjectInfo           `json:"projects,omitempty"`
}

// PolicyInfo represents a policy within a policy set.
type PolicyInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	EnforcementLevel string `json:"enforcement_level"`
	PolicySetID     string `json:"policy_set_id"`
	Kind            string `json:"kind"`
}

// WorkspaceInfo represents basic workspace information.
type WorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProjectInfo represents basic project information.
type ProjectInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VCSRepoInfo represents VCS repository information.
type VCSRepoInfo struct {
	Identifier        string `json:"identifier"`
	Branch            string `json:"branch,omitempty"`
	IngressSubmodules bool   `json:"ingress_submodules"`
	OAuthTokenID      string `json:"oauth_token_id"`
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

// ListWorkspacePolicySets creates a tool to read all policy sets attached to a workspace.
func ListWorkspacePolicySets(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_workspace_policy_sets",
			mcp.WithDescription("Read all policy sets attached to a workspace. Returns both directly attached policy sets and global policy sets that apply to all workspaces."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_id", mcp.Required(), mcp.Description("The workspace ID to get policy sets for (e.g., ws-2HRvNs49EWPjDqT1)")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listWorkspacePolicySetsHandler(ctx, request, logger)
		},
	}
}

func listWorkspacePolicySetsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

// GetPolicySetDetails creates a tool to get detailed information about a specific policy set.
func GetPolicySetDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_policy_set_details",
			mcp.WithDescription("Get detailed information about a specific policy set, including all policies, enforcement levels, workspaces, and configuration."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("policy_set_id", mcp.Required(), mcp.Description("The ID of the policy set to get details for (e.g., polset-3yVQZvHzf5j3WRJ1)")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPolicySetDetailsHandler(ctx, request, logger)
		},
	}
}

func getPolicySetDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	policySetID, err := request.RequireString("policy_set_id")
	if err != nil {
		return ToolError(logger, "missing required input: policy_set_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// Read the policy set (v1.103.0 doesn't support include options in Read)
	policySet, err := tfeClient.PolicySets.Read(ctx, policySetID)
	if err != nil {
		return ToolErrorf(logger, "failed to read policy set '%s': %v", policySetID, err)
	}

	// Build the detailed response
	overridable := false
	if policySet.Overridable != nil {
		overridable = *policySet.Overridable
	}

	details := PolicySetDetails{
		ID:                policySet.ID,
		Name:              policySet.Name,
		Description:       policySet.Description,
		Kind:              string(policySet.Kind),
		Global:            policySet.Global,
		Overridable:       overridable,
		PoliciesPath:      policySet.PoliciesPath,
		PolicyToolVersion: policySet.PolicyToolVersion,
		Policies:          make([]PolicyInfo, 0),
		Workspaces:        make([]WorkspaceInfo, 0),
		Projects:          make([]ProjectInfo, 0),
	}

	// Add VCS repo information if available
	if policySet.VCSRepo != nil {
		details.VCSRepo = &VCSRepoInfo{
			Identifier:        policySet.VCSRepo.Identifier,
			Branch:            policySet.VCSRepo.Branch,
			IngressSubmodules: policySet.VCSRepo.IngressSubmodules,
			OAuthTokenID:      policySet.VCSRepo.OAuthTokenID,
		}
	}

	// Add policies information
	for _, policy := range policySet.Policies {
		enforcementLevel := ""
		if len(policy.Enforce) > 0 {
			enforcementLevel = string(policy.Enforce[0].Mode)
		}
		details.Policies = append(details.Policies, PolicyInfo{
			ID:              policy.ID,
			Name:            policy.Name,
			Description:     policy.Description,
			EnforcementLevel: enforcementLevel,
			PolicySetID:     policySetID,
			Kind:            string(policy.Kind),
		})
	}

	// Add workspaces information
	for _, workspace := range policySet.Workspaces {
		details.Workspaces = append(details.Workspaces, WorkspaceInfo{
			ID:   workspace.ID,
			Name: workspace.Name,
		})
	}

	// Add projects information
	for _, project := range policySet.Projects {
		details.Projects = append(details.Projects, ProjectInfo{
			ID:   project.ID,
			Name: project.Name,
		})
	}

	result, err := json.MarshalIndent(details, "", "  ")
	if err != nil {
		return ToolError(logger, "failed to marshal policy set details", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(string(result)),
		},
	}, nil
}


