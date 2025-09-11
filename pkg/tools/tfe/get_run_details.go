// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"

	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetRunDetails creates a tool to get detailed information about a specific Terraform run.
func GetRunDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_run_details",
			mcp.WithDescription(`Fetches detailed information about a specific Terraform run.`),
			mcp.WithTitleAnnotation("Get detailed information about a Terraform run"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("The ID of the run to get details for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getRunDetailsHandler(ctx, req, logger)
		},
	}
}

func getRunDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runID, err := request.RequireString("run_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'run_id' parameter is required", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}

	run, err := tfeClient.Runs.Read(ctx, runID)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading run details", err)
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, run)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "marshalling run details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
