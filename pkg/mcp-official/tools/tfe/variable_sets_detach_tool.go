// Copyright IBM Corp. 2025, 2026
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

// DetachVariableSetFromWorkspacesArguments holds the inputs for detaching a variable set from workspaces.
type DetachVariableSetFromWorkspacesArguments struct {
	VariableSetID string `json:"variable_set_id" jsonschema:"The ID of the variable set to detach"`
	WorkspaceIDs  string `json:"workspace_ids" jsonschema:"Comma-separated list of workspace IDs"`
}

// DetachVariableSetFromWorkspacesResponse reports the completed detachment operation.
type DetachVariableSetFromWorkspacesResponse struct {
	VariableSetID  string `json:"variable_set_id"`
	WorkspaceCount int    `json:"workspace_count"`
	Message        string `json:"message"`
}

// DetachVariableSetFromWorkspacesTool describes the detach_variable_set_from_workspaces tool.
func DetachVariableSetFromWorkspacesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "detach_variable_set_from_workspaces",
		Description: "Detach a variable set from one or more workspaces.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Detach a Terraform variable set from workspaces",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func DetachVariableSetFromWorkspacesFunc(ctx context.Context, request *mcp.CallToolRequest, input DetachVariableSetFromWorkspacesArguments) (*mcp.CallToolResult, *DetachVariableSetFromWorkspacesResponse, error) {
	variableSetID := strings.TrimSpace(input.VariableSetID)
	if variableSetID == "" {
		return nil, nil, fmt.Errorf("variable_set_id must not be blank")
	}

	workspaceIDs := strings.TrimSpace(input.WorkspaceIDs)
	if workspaceIDs == "" {
		return nil, nil, fmt.Errorf("workspace_ids must not be blank")
	}

	workspaces := make([]*tfe.Workspace, 0)
	for _, id := range strings.Split(workspaceIDs, ",") {
		if id := strings.TrimSpace(id); id != "" {
			workspaces = append(workspaces, &tfe.Workspace{ID: id})
		}
	}
	if len(workspaces) == 0 {
		return nil, nil, fmt.Errorf("workspace_ids must contain at least one workspace ID")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	if err := tfeClient.VariableSets.RemoveFromWorkspaces(ctx, variableSetID, &tfe.VariableSetRemoveFromWorkspacesOptions{
		Workspaces: workspaces,
	}); err != nil {
		return nil, nil, fmt.Errorf("detaching variable set %q from workspaces: %w", variableSetID, err)
	}

	return nil, &DetachVariableSetFromWorkspacesResponse{
		VariableSetID:  variableSetID,
		WorkspaceCount: len(workspaces),
		Message:        "variable set detached from workspaces successfully",
	}, nil
}
