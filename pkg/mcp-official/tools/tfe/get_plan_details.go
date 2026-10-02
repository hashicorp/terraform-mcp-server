// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetPlanDetailsArguments struct {
	PlanID string `json:"plan_id" jsonschema:"The ID of the plan to get details for"`
}

func GetPlanDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_plan_details",
		Description: "Fetches detailed information about a specific Terraform plan.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get details for a Terraform plan",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetPlanDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetPlanDetailsArguments) (*mcp.CallToolResult, any, error) {
	planID := strings.TrimSpace(input.PlanID)
	if planID == "" {
		return nil, nil, fmt.Errorf("missing required input: plan_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}

	plan, err := tfeClient.Plans.Read(ctx, planID)
	if err != nil {
		return nil, nil, fmt.Errorf("plan not found: %s: %w", planID, err)
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, plan)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal plan details: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: buf.String()}},
	}, nil, nil
}
