// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	_ "embed"
	"strings"

	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetStackTools creates a tool to get detailed information about a specific Terraform Stack.
func GetStackDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_stack_details",
			mcp.WithDescription(`Fetches detailed information about a specific Terraform Stack.`),
			mcp.WithTitleAnnotation("Get detailed information about a Terraform Stack"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("stack_id",
				mcp.Required(),
				mcp.Description("The ID of the stack to get details for"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getStackDetailsHandler(ctx, request, logger)
		},
	}
}

func getStackDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	stackID, err := request.RequireString("stack_id")
	if err != nil {
		return ToolError(logger, "missing required input: stack_id", err)
	}
	stackID = strings.TrimSpace(stackID)

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	stack, err := tfeClient.Stacks.Read(ctx, stackID)
	if err != nil {
		return ToolErrorf(logger, "stack %q not found in org %q", stackID, terraformOrgName)
	}

	buf := bytes.NewBuffer(nil)
	err = jsonapi.MarshalPayloadWithoutIncluded(buf, stack)
	if err != nil {
		return ToolError(logger, "failed to marshal stack details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}
