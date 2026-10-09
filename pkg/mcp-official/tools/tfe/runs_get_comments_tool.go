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

// GetRunCommentsArguments holds the run ID whose comments will be listed.
type GetRunCommentsArguments struct {
	RunID string `json:"run_id" jsonschema:"The ID of the Terraform run to retrieve comments for (e.g., run-Nj2MTonBKmtmceGE)"`
}

type RunCommentSummary struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

type RunCommentsResponse struct {
	Items []RunCommentSummary `json:"items"`
}

// GetRunCommentsTool describes the get_run_comments tool.
func GetRunCommentsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_run_comments",
		Description: "Lists the user-submitted comments returned by the Terraform run comments endpoint.",
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"items": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"id":   {Type: "string"},
							"body": {Type: "string"},
						},
						PropertyOrder:        []string{"id", "body"},
						Required:             []string{"id", "body"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
			},
			PropertyOrder:        []string{"items"},
			Required:             []string{"items"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get comments for a Terraform run",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetRunCommentsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetRunCommentsArguments) (*mcp.CallToolResult, *RunCommentsResponse, error) {
	runID := strings.TrimLeft(strings.TrimSpace(input.RunID), "#")
	if runID == "" {
		return nil, nil, fmt.Errorf("run_id must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	comments, err := tfeClient.Comments.List(ctx, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("listing comments for run %q: %w", runID, err)
	}
	if comments == nil {
		return nil, nil, fmt.Errorf("listing comments for run %q: Terraform returned an empty response", runID)
	}

	summaries := make([]RunCommentSummary, 0, len(comments.Items))
	for _, comment := range comments.Items {
		if comment != nil {
			summaries = append(summaries, RunCommentSummary{ID: comment.ID, Body: comment.Body})
		}
	}

	return nil, &RunCommentsResponse{Items: summaries}, nil
}
