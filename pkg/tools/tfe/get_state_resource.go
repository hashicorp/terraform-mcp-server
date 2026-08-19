// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetStateResource creates a tool to fetch a single resource's full attributes from a
// workspace's current state, addressing issue #437: fetching the entire state to answer
// "what's the config of resource X" floods the model's context window. This returns just
// the one resource block instead.
func GetStateResource(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_state_resource",
			mcp.WithDescription("Get complete details for a single resource in a workspace's current Terraform state, "+
				"identified by its state address (e.g. \"aws_s3_bucket.assets\" or \"module.vpc.aws_vpc.main\"). "+
				"Values flagged sensitive in state, plus those whose key matches the configured redaction pattern, "+
				"are redacted; unflagged secrets may still appear, so review output before sharing. "+
				"Use list_state_resources first if you don't know the exact address."),
			mcp.WithTitleAnnotation("Get Terraform State Resource"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description(terraformOrgNameDescription),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to read state from"),
			),
			mcp.WithString("address",
				mcp.Required(),
				mcp.Description(`Full resource address from state, e.g. "aws_s3_bucket.assets" or "module.vpc.aws_vpc.main"`),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getStateResourceHandler(ctx, request, logger)
		},
	}
}

func getStateResourceHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	orgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_name", err)
	}
	address, err := request.RequireString("address")
	if err != nil {
		return ToolError(logger, "missing required input: address", err)
	}
	orgName = strings.TrimSpace(orgName)
	workspaceName = strings.TrimSpace(workspaceName)
	address = strings.TrimSpace(address)

	state, pattern, err := resolveWorkspaceState(ctx, orgName, workspaceName, logger)
	if err != nil {
		return ToolError(logger, "loading Terraform state", err)
	}

	for _, r := range extractResources(state, pattern) {
		if r.Address == address {
			data, err := json.MarshalIndent(r, "", "  ")
			if err != nil {
				return ToolError(logger, "marshaling resource", err)
			}
			return mcp.NewToolResultText(string(data)), nil
		}
	}

	return ToolErrorf(logger, "resource %q not found in workspace '%s' state — use list_state_resources to see available addresses", address, workspaceName)
}
