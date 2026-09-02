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

// AttachPolicySetToWorkspacesArguments holds the required inputs for attaching a policy set to workspaces.
type AttachPolicySetToWorkspacesArguments struct {
	// Required field
	PolicySetID  string `json:"policy_set_id" jsonschema:"The ID of the policy set to attach (e.g., polset-3yVQZvHzf5j3WRJ1)"`
	WorkspaceIDs string `json:"workspace_ids" jsonschema:"Comma-separated list of workspace IDs to attach the policy set to"`
}

func AttachPolicySetToWorkspacesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "attach_policy_set_to_workspaces",
		Description: "Attach a policy set to one or more workspaces. Note: Policy sets marked as global cannot be attached to individual workspaces.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Attach policy set to workspaces",
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
	}
}

func AttachPolicySetToWorkspacesFunc(ctx context.Context, request *mcp.CallToolRequest, input AttachPolicySetToWorkspacesArguments) (*mcp.CallToolResult, any, error) {
	policySetID := strings.TrimSpace(input.PolicySetID)
	if policySetID == "" {
		return nil, nil, fmt.Errorf("policy_set_id must not be blank")
	}

	workspaceIDsStr := strings.TrimSpace(input.WorkspaceIDs)
	if workspaceIDsStr == "" {
		return nil, nil, fmt.Errorf("workspace_ids must not be blank")
	}

	var workspaces []*tfe.Workspace
	for _, id := range strings.Split(workspaceIDsStr, ",") {
		trimmedID := strings.TrimSpace(id)
		if trimmedID != "" {
			workspaces = append(workspaces, &tfe.Workspace{ID: trimmedID})
		}
	}

	if len(workspaces) == 0 {
		return nil, nil, fmt.Errorf("no valid workspace IDs provided")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	err = tfeClient.PolicySets.AddWorkspaces(ctx, policySetID, tfe.PolicySetAddWorkspacesOptions{
		Workspaces: workspaces,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to attach policy set %q to workspaces: %w", policySetID, err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: fmt.Sprintf("Successfully attached policy set %q to %d workspace(s)", policySetID, len(workspaces)),
		}},
	}, nil, nil
}
