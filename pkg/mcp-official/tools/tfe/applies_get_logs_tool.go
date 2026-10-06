// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	tfe "github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetApplyLogsArguments struct {
	ApplyID string `json:"apply_id" jsonschema:"The ID of the apply to get logs for"`
}

func GetApplyLogsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_apply_logs",
		Description: "Retrieves the logs of a specific Terraform apply.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get logs for a Terraform apply",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}
func GetApplyLogsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetApplyLogsArguments) (*mcp.CallToolResult, any, error) {
	applyID := input.ApplyID
	if strings.TrimSpace(applyID) == "" {
		return nil, nil, fmt.Errorf("missing required input: apply_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}

	apply, err := tfeClient.Applies.Read(ctx, applyID)
	if err != nil {
		return nil, nil, fmt.Errorf("apply not found: %s: %w", applyID, err)
	}

	if apply.Status == tfe.ApplyUnreachable {
		return nil, nil, fmt.Errorf("apply %s is unreachable and has no logs: the run stopped before the apply phase (e.g. no changes, plan-only run, or failed plan), and this apply will never run. Check get_plan_logs for the reason.", applyID)
	}

	terminalStatuses := []tfe.ApplyStatus{
		tfe.ApplyErrored,
		tfe.ApplyFinished,
		tfe.ApplyCanceled,
	}

	if !slices.Contains(terminalStatuses, apply.Status) {
		return nil, nil, fmt.Errorf(
			"Apply %s is currently in status %q. Wait for the status to change to a terminal state (finished, errored, canceled) before calling again.",
			applyID, apply.Status,
		)
	}

	logReader, err := tfeClient.Applies.Logs(ctx, applyID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve apply logs: %s: %w", applyID, err)
	}

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read apply logs: %s: %w", applyID, err)
	}

	return textResult(string(logBytes)), nil, nil
}
