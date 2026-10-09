// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultRunActionComment = "Triggered via Terraform MCP Server"

var runActions = []string{"apply", "discard", "cancel"}

// ActionRunArguments holds the inputs for acting on a run.
type ActionRunArguments struct {
	RunAction string `json:"run_action"`
	RunID     string `json:"run_id"`
	Comment   string `json:"comment,omitempty"`
}

// ActionRunTool describes the action_run tool.
func ActionRunTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "action_run",
		Description: "Applies, discards, or cancels a Terraform run.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"run_action": {
					Type:        "string",
					Description: "The action to perform on the run: " + strings.Join(runActions, ", "),
					Enum:        enumOf(runActions...),
				},
				"run_id": {
					Type:        "string",
					Description: "The ID of the run to act on",
				},
				"comment": {
					Type:        "string",
					Description: "Optional comment for the action",
					Default:     json.RawMessage(`"` + defaultRunActionComment + `"`),
				},
			},
			PropertyOrder:        []string{"run_action", "run_id", "comment"},
			Required:             []string{"run_action", "run_id"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Apply, discard, or cancel a Terraform run",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func ActionRunFunc(ctx context.Context, request *mcp.CallToolRequest, input ActionRunArguments) (*mcp.CallToolResult, any, error) {
	runAction := strings.TrimSpace(input.RunAction)
	if runAction == "" {
		return nil, nil, fmt.Errorf("run_action must not be blank")
	}
	if !slices.Contains(runActions, runAction) {
		return nil, nil, fmt.Errorf("run_action %q must be one of: %s", runAction, strings.Join(runActions, ", "))
	}

	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return nil, nil, fmt.Errorf("run_id must not be blank")
	}

	comment := input.Comment
	if comment == "" {
		comment = defaultRunActionComment
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	var message string
	switch runAction {
	case "apply":
		err = tfeClient.Runs.Apply(ctx, runID, tfe.RunApplyOptions{Comment: tfe.String(comment)})
		message = fmt.Sprintf("Apply request accepted for run %q. Use get_run_details to monitor its status.", runID)
	case "discard":
		err = tfeClient.Runs.Discard(ctx, runID, tfe.RunDiscardOptions{Comment: tfe.String(comment)})
		message = fmt.Sprintf("Discard request accepted for run %q.", runID)
	case "cancel":
		err = tfeClient.Runs.Cancel(ctx, runID, tfe.RunCancelOptions{Comment: tfe.String(comment)})
		message = fmt.Sprintf("Cancel request accepted for run %q.", runID)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("requesting %s for run %q: %w", runAction, runID, err)
	}

	return textResult(message), nil, nil
}
