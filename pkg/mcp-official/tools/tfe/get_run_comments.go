// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CommentsSummary is a truncated summary of a Comments for top level listing
type CommentsSummary struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// CommentsSummaryList contains the list of Comments summaries
type CommentsSummaryList struct {
	Items []*CommentsSummary `json:"items"`
}

// GetRunCommentsArguments holds the input parameters for listing run comments.
type GetRunCommentsArguments struct {
	// Required field
	RunID string `json:"run_id" jsonschema:"The ID of the Terraform run to retrieve comments for (format: run-<alphanumeric>, e.g. run-Nj2MTonBKmtmceGE."`
}

// GetRunCommentsTool creates a tool to get all the comments for a given run.
func GetRunCommentsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_run_comments",
		Description: "Lists all user-submitted comments on a specific Terraform run. Use this tool when you need to review feedback, approvals, or notes that collaborators have left on a run. Returns each comment's ID and body text. To get the run's status, plan, and apply details instead, use get_run_details.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get all comments for a given Terraform run.",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

// GetRunCommentsFunc lists the comments left on the requested run.
func GetRunCommentsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetRunCommentsArguments) (*mcp.CallToolResult, *CommentsSummaryList, error) {
	runID := strings.TrimLeft(strings.TrimSpace(input.RunID), "#")
	if runID == "" {
		return nil, nil, fmt.Errorf("missing required input: run_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	comments, err := tfeClient.Comments.List(ctx, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list run comments: %w", err)
	}

	summaries := make([]*CommentsSummary, len(comments.Items))
	for i, c := range comments.Items {
		summaries[i] = &CommentsSummary{
			ID:   c.ID,
			Body: c.Body,
		}
	}

	return nil, &CommentsSummaryList{Items: summaries}, nil
}
