// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ListRunComments creates a tool that lists every comment attached to a
// specific Terraform Cloud / Enterprise run. Closes the gap described in
// https://github.com/hashicorp/terraform-mcp-server/issues/347 — `action_run`
// already supports posting a comment, but until now there was no read path,
// so an agent inspecting a run via `get_run_details` could see that comments
// existed but couldn't read their bodies.
//
// Backed by the upstream TFE Comments API:
//
//	GET /runs/{run_id}/comments
//	https://developer.hashicorp.com/terraform/cloud-docs/api-docs/comments
func ListRunComments(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_run_comments",
			mcp.WithDescription(`List all comments attached to a specific Terraform run. Useful for reading context left by humans or automation on a run before deciding next steps.`),
			mcp.WithTitleAnnotation("List Terraform run comments"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("ID of the Terraform run whose comments should be listed (e.g. 'run-CZcmD7eagjhyX0vN')."),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listRunCommentsHandler(ctx, req, logger)
		},
	}
}

func listRunCommentsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runID, err := request.RequireString("run_id")
	if err != nil {
		return ToolError(logger, "missing required input: run_id", err)
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ToolErrorf(logger, "run_id must be non-empty")
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	commentList, err := tfeClient.Comments.List(ctx, runID)
	if err != nil {
		return ToolErrorf(logger, "failed to list comments for run '%s': %s", runID, err.Error())
	}

	summaries := make([]*RunCommentSummary, len(commentList.Items))
	for i, c := range commentList.Items {
		summaries[i] = &RunCommentSummary{
			ID:   c.ID,
			Body: c.Body,
		}
	}

	buf, err := json.Marshal(&RunCommentList{
		Items: summaries,
	})
	if err != nil {
		return ToolError(logger, "failed to marshal comments", err)
	}

	return mcp.NewToolResultText(string(buf)), nil
}

// RunCommentSummary is the shape returned per-comment. The upstream TFE
// Comment type carries only ID and Body, so we project both — adding
// future fields here is a passthrough exercise.
type RunCommentSummary struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// RunCommentList wraps the per-run comment summaries. No pagination
// fields today because the upstream Comments.List endpoint does not
// expose a paging cursor (CommentList embeds *Pagination but TFE
// returns the full set in one response). Wrapping in a struct keeps
// the tool output forward-compatible if pagination is later wired up.
type RunCommentList struct {
	Items []*RunCommentSummary `json:"items"`
}
