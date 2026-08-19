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

// resourceSummary is a slimmed-down resource listing (no attributes) for token efficiency.
type resourceSummary struct {
	Address      string   `json:"address"`
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Module       string   `json:"module"`
	Mode         string   `json:"mode"`
	Provider     string   `json:"provider"`
	Dependencies []string `json:"dependencies"`
}

// ListStateResources creates a tool to list resource addresses in a workspace's current
// state, optionally filtered by type or module, without dumping full attributes — the
// gap identified in issue #437 for tools that otherwise return the entire state blob.
func ListStateResources(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_state_resources",
			mcp.WithDescription("List resources in a workspace's current Terraform state with their addresses, types, "+
				"modules, and dependency counts, without their attributes. Supports optional filtering by resource type "+
				"or module path, e.g. to answer \"list all the S3 buckets\" without dumping the whole state."),
			mcp.WithTitleAnnotation("List Terraform State Resources"),
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
			mcp.WithString("type_filter",
				mcp.Description("Only include resources whose type contains this substring (case-insensitive), e.g. 'aws_s3_bucket'"),
			),
			mcp.WithString("module_filter",
				mcp.Description("Only include resources in a specific module path, e.g. 'module.vpc'"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listStateResourcesHandler(ctx, request, logger)
		},
	}
}

func listStateResourcesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	orgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_name", err)
	}
	orgName = strings.TrimSpace(orgName)
	workspaceName = strings.TrimSpace(workspaceName)
	typeFilter := GetTrimmedString(request, "type_filter", "")
	moduleFilter := GetTrimmedString(request, "module_filter", "")

	state, pattern, err := resolveWorkspaceState(ctx, orgName, workspaceName, logger)
	if err != nil {
		return ToolError(logger, "loading Terraform state", err)
	}

	resources := extractResources(state, pattern)

	summaries := make([]resourceSummary, 0, len(resources))
	for _, r := range resources {
		if typeFilter != "" && !strings.Contains(strings.ToLower(r.Type), strings.ToLower(typeFilter)) {
			continue
		}
		if moduleFilter != "" && !strings.Contains(r.Module, moduleFilter) {
			continue
		}
		summaries = append(summaries, resourceSummary{
			Address:      r.Address,
			Type:         r.Type,
			Name:         r.Name,
			Module:       r.Module,
			Mode:         r.Mode,
			Provider:     r.Provider,
			Dependencies: r.Dependencies,
		})
	}

	data, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return ToolError(logger, "marshaling response", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}
