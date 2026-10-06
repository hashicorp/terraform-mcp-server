// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetPlanLogsArguments struct {
	PlanID string `json:"plan_id" jsonschema:"The ID of the plan to get logs for"`
}

func GetPlanLogsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_plan_logs",
		Description: "Retrieves the logs of a specific Terraform plan.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get logs for a Terraform plan",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetPlanLogsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetPlanLogsArguments) (*mcp.CallToolResult, any, error) {
	planID := strings.TrimSpace(input.PlanID)
	if planID == "" {
		return nil, nil, fmt.Errorf("missing required input: plan_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}

	logReader, err := tfeClient.Plans.Logs(ctx, planID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve plan logs: %s: %w", planID, err)
	}

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read plan logs: %w", err)
	}

	return textResult(string(logBytes)), nil, nil
}
