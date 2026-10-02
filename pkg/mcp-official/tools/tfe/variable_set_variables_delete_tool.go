// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DeleteVariableInVariableSetArguments holds the inputs for deleting a variable from a variable set.
type DeleteVariableInVariableSetArguments struct {
	VariableSetID string `json:"variable_set_id" jsonschema:"The ID of the variable set"`
	VariableID    string `json:"variable_id" jsonschema:"The ID of the variable to delete"`
}

// DeleteVariableInVariableSetResponse reports the variable deleted by the tool.
type DeleteVariableInVariableSetResponse struct {
	VariableID    string `json:"variable_id"`
	VariableSetID string `json:"variable_set_id"`
	Message       string `json:"message"`
}

// DeleteVariableInVariableSetTool describes the delete_variable_in_variable_set tool.
func DeleteVariableInVariableSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "delete_variable_in_variable_set",
		Description: "Delete a variable in a variable set.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete a variable from a Terraform variable set",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func DeleteVariableInVariableSetFunc(ctx context.Context, request *mcp.CallToolRequest, input DeleteVariableInVariableSetArguments) (*mcp.CallToolResult, *DeleteVariableInVariableSetResponse, error) {
	variableSetID := strings.TrimSpace(input.VariableSetID)
	if variableSetID == "" {
		return nil, nil, fmt.Errorf("variable_set_id must not be blank")
	}

	variableID := strings.TrimSpace(input.VariableID)
	if variableID == "" {
		return nil, nil, fmt.Errorf("variable_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	if err := tfeClient.VariableSetVariables.Delete(ctx, variableSetID, variableID); err != nil {
		return nil, nil, fmt.Errorf("deleting variable %q from variable set %q: %w", variableID, variableSetID, err)
	}

	return nil, &DeleteVariableInVariableSetResponse{
		VariableID:    variableID,
		VariableSetID: variableSetID,
		Message:       "variable deleted successfully",
	}, nil
}
