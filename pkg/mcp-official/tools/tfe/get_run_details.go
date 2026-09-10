// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetRunDetailsArguments holds the input parameters for reading a single run.
type GetRunDetailsArguments struct {
	// Required field
	RunID string `json:"run_id" jsonschema:"The ID of the run to get details for"`
}

// GetRunDetailsTool creates a tool to get detailed information about a specific
// Terraform run.
func GetRunDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_run_details",
		Description: "Fetches detailed information about a specific Terraform run.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a Terraform run",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

// GetRunDetailsFunc returns the jsonapi payload for the requested run.
func GetRunDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetRunDetailsArguments) (*mcp.CallToolResult, any, error) {
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return nil, nil, fmt.Errorf("missing required input: run_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	run, err := tfeClient.Runs.Read(ctx, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("run not found: %s: %w", runID, err)
	}

	buf := bytes.NewBuffer(nil)
	if err := jsonapi.MarshalPayloadWithoutIncluded(buf, run); err != nil {
		return nil, nil, fmt.Errorf("failed to marshal run details: %w", err)
	}

	return textResult(buf.String()), nil, nil
}
