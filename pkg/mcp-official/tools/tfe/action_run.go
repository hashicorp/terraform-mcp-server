// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Run actions supported by the action_run tool.
const (
	runActionApply   = "apply"
	runActionDiscard = "discard"
	runActionCancel  = "cancel"
)

// ActionRunArguments is the unmarshal target for action_run. The wire contract -
// descriptions, enum and which fields are required - is declared by the input
// schema in ActionRunTool, so these fields carry no jsonschema tags.
type ActionRunArguments struct {
	RunAction string `json:"run_action"`
	RunID     string `json:"run_id"`
	Comment   string `json:"comment,omitempty"`
}

// ActionRunTool creates a tool to apply, discard or cancel a Terraform run.
func ActionRunTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "action_run",
		Description: "Performs a variety of actions on a Terraform run. It can be used to approve and apply, discard or cancel a run.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Apply, Discard or Cancel a Terraform run",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"run_action": {
					Type:        "string",
					Description: "The action to perform on the run (e.g., 'apply', 'discard', 'cancel')",
					Enum:        []any{runActionApply, runActionDiscard, runActionCancel},
				},
				"run_id": {
					Type:        "string",
					Description: "The ID of the run to perform the action on",
				},
				"comment": {
					Type:        "string",
					Description: "Optional comment for the action",
					Default:     json.RawMessage(`"` + defaultRunComment + `"`),
				},
			},
			Required: []string{"run_action", "run_id"},
		},
	}
}

// ActionRunFunc applies, discards or cancels the given run.
func ActionRunFunc(ctx context.Context, request *mcp.CallToolRequest, input ActionRunArguments) (*mcp.CallToolResult, any, error) {
	runAction := strings.TrimSpace(input.RunAction)
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return nil, nil, fmt.Errorf("missing required input: run_id")
	}

	// Validate the action before reaching out to Terraform so an unsupported
	// action never costs a round trip.
	switch runAction {
	case runActionApply, runActionDiscard, runActionCancel:
	default:
		return nil, nil, fmt.Errorf("invalid run_action: %s - must be %q, %q, or %q",
			runAction, runActionApply, runActionDiscard, runActionCancel)
	}

	comment := input.Comment
	if comment == "" {
		comment = defaultRunComment
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	var msg string
	switch runAction {
	case runActionApply:
		err = tfeClient.Runs.Apply(ctx, runID, tfe.RunApplyOptions{Comment: &comment})
		msg = "Run approved and applied successfully, run the `get_run_details` tool to get more information about the run."
	case runActionDiscard:
		err = tfeClient.Runs.Discard(ctx, runID, tfe.RunDiscardOptions{Comment: &comment})
		msg = "Run discarded successfully"
	case runActionCancel:
		err = tfeClient.Runs.Cancel(ctx, runID, tfe.RunCancelOptions{Comment: &comment})
		msg = "Run canceled successfully"
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to %s run %s: %w", runAction, runID, err)
	}

	return textResult(msg), nil, nil
}
