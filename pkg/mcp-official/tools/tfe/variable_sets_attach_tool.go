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

// AttachVariableSetToWorkspacesArguments holds the inputs for attaching a variable set to workspaces.
type AttachVariableSetToWorkspacesArguments struct {
	VariableSetID string `json:"variable_set_id" jsonschema:"The ID of the variable set to attach"`
	WorkspaceIDs  string `json:"workspace_ids" jsonschema:"Comma-separated list of workspace IDs"`
}

// AttachVariableSetToWorkspacesResponse reports the completed attachment operation.
type AttachVariableSetToWorkspacesResponse struct {
	VariableSetID  string `json:"variable_set_id"`
	WorkspaceCount int    `json:"workspace_count"`
	Message        string `json:"message"`
}

// AttachVariableSetToWorkspacesTool describes the attach_variable_set_to_workspaces tool.
func AttachVariableSetToWorkspacesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "attach_variable_set_to_workspaces",
		Description: "Attach a variable set to one or more workspaces.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Attach a Terraform variable set to workspaces",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func AttachVariableSetToWorkspacesFunc(ctx context.Context, request *mcp.CallToolRequest, input AttachVariableSetToWorkspacesArguments) (*mcp.CallToolResult, *AttachVariableSetToWorkspacesResponse, error) {
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

	if err := tfeClient.VariableSets.ApplyToWorkspaces(ctx, variableSetID, &tfe.VariableSetApplyToWorkspacesOptions{
		Workspaces: workspaces,
	}); err != nil {
		return nil, nil, fmt.Errorf("attaching variable set %q to workspaces: %w", variableSetID, err)
	}

	return nil, &AttachVariableSetToWorkspacesResponse{
		VariableSetID:  variableSetID,
		WorkspaceCount: len(workspaces),
		Message:        "variable set attached to workspaces successfully",
	}, nil
}
