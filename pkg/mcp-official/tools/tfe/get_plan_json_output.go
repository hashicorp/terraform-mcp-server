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

type GetPlanJSONOutputArguments struct {
	PlanID string `json:"plan_id" jsonschema:"The ID of the plan to get JSON output for"`
}

func GetPlanJSONOutputTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_plan_json_output",
		Description: "Retrieves the JSON output of a specific Terraform plan. This includes detailed information about resource changes (create, update, delete), attribute values before and after, and plan metadata. This is more structured and easier to parse than plain logs.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get JSON output for a Terraform plan",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetPlanJSONOutputFunc(ctx context.Context, request *mcp.CallToolRequest, input GetPlanJSONOutputArguments) (*mcp.CallToolResult, any, error) {
	planID := strings.TrimSpace(input.PlanID)
	if planID == "" {
		return nil, nil, fmt.Errorf("missing required input: plan_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}

	jsonBytes, err := tfeClient.Plans.ReadJSONOutput(ctx, planID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve plan JSON output: %s", planID)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(jsonBytes)}},
	}, nil, nil
}
