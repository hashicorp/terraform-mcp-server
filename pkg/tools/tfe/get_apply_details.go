// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"

	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetApplyDetails creates a tool to get detailed information about a specific Terraform apply.
func GetApplyDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_apply_details",
			mcp.WithDescription(`Fetches detailed information about a specific Terraform apply.`),
			mcp.WithTitleAnnotation("Get detailed information about a Terraform apply"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("apply_id",
				mcp.Required(),
				mcp.Description("The ID of the apply to get details for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getApplyDetailsHandler(ctx, req, logger)
		},
	}
}

func getApplyDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	applyID, err := request.RequireString("apply_id")
	if err != nil {
		return ToolError(logger, "missing required input: apply_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	apply, err := tfeClient.Applies.Read(ctx, applyID)
	if err != nil {
		return ToolErrorf(logger, "apply not found: %s", applyID)
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, apply)
	if err != nil {
		return ToolError(logger, "failed to marshal apply details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
