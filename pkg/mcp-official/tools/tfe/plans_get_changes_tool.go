// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/planchanges"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetPlanChangesArguments struct {
	RunID  string `json:"run_id,omitempty" jsonschema:"The ID of the run whose plan to read (run-...). Provide this or plan_id."`
	PlanID string `json:"plan_id,omitempty" jsonschema:"The ID of the plan to read (plan-...). Provide this or run_id."`
}

func GetPlanChangesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_plan_changes",
		Description: "Lists the resources and outputs a Terraform plan changes: action (delete, replace, forget, update, create, import, move) and address, plus counts per action and resources that drifted outside Terraform. Never returns attribute values. Reads the redacted JSON plan, so unlike get_plan_json_output it does not need workspace admin access. Pass the run ID or the plan ID.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get the changes in a Terraform plan",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetPlanChangesFunc(ctx context.Context, request *mcp.CallToolRequest, input GetPlanChangesArguments) (*mcp.CallToolResult, any, error) {
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}

	raw, err := planchanges.Fetch(ctx, tfeClient, input.RunID, input.PlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve plan changes: %w", err)
	}

	summary, err := planchanges.Summarize(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to summarize plan changes: %w", err)
	}

	out, err := json.Marshal(summary)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal plan changes: %w", err)
	}
	return textResult(string(out)), nil, nil
}
