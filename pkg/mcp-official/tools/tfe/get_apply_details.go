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

type GetApplyDetailsArguments struct {
	ApplyID string `json:"apply_id" jsonschema:"The ID of the apply to get details for"`
}

func GetApplyDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_apply_details",
		Description: "Fetches detailed information about a specific Terraform apply.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a Terraform apply",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetApplyDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetApplyDetailsArguments) (*mcp.CallToolResult, any, error) {
	applyID := strings.TrimSpace(input.ApplyID)
	if applyID == "" {
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

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, apply)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal apply details: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: buf.String()}},
	}, nil, nil
}
