// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetRunComments creates a tool to get all the comments for a given run.
func GetRunComments(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool(
			"get_run_comments",
			mcp.WithDescription(`Fetches comments about a specific Terraform run.`),
			mcp.WithTitleAnnotation("Get all comments for a given Terraform run."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("The ID of the run to get comments for"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getRunCommentsHandler(ctx, request, logger)
		},
	}
}

// getRunCommentsHandler handles tool logics and functionality
func getRunCommentsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {

	// init clint object
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	// Required Params
	runID, err := request.RequireString("run_id")
	if err != nil {
		return ToolError(logger, "missing required input: run_id", err)
	}
	runID = strings.TrimSpace(runID)

	// List Comments
	comments, err := tfeClient.Comments.List(ctx, runID)
	if err != nil {
		return ToolError(logger, "failed to list run comments", err)
	}

	//Comment Summaries
	commentSummaries := make([]*CommentsSummary, len(comments.Items))
	for i, o := range comments.Items {
		commentSummaries[i] = &CommentsSummary{
			ID:   o.ID,
			Body: o.Body,
		}
	}

	// Marshal JSON
	commentsJSON, err := json.Marshal(&CommentsSummaryList{
		Items: commentSummaries,
	})
	if err != nil {
		return ToolError(logger, "failed to serialize state version", err)
	}

	return mcp.NewToolResultText(string(commentsJSON)), nil
}

// CommentsSummary is a truncated summary of a Comments for top level listing
type CommentsSummary struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// CommentsSummaryList contains the list of Comments summaries and pagination details
type CommentsSummaryList struct {
	Items []*CommentsSummary `json:"items"`
	*tfe.Pagination
}
