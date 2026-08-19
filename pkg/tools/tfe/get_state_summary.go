// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetStateSummary creates a tool to generate a high-level summary of a workspace's current
// Terraform state.
func GetStateSummary(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_state_summary",
			mcp.WithDescription("Generate a high-level summary of a workspace's current Terraform state, including "+
				"resource counts by type and module, output names, Terraform version, and state serial number — "+
				"without the resources' attributes. Useful as a first pass before diving into specific resources."),
			mcp.WithTitleAnnotation("Summarize Terraform State"),
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
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getStateSummaryHandler(ctx, request, logger)
		},
	}
}

// getStateSummaryHandler handles tool logics and functionality
func getStateSummaryHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
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

	state, err := resolveWorkspaceState(ctx, orgName, workspaceName, logger)
	if err != nil {
		return ToolError(logger, "loading Terraform state", err)
	}

	resources := extractResources(state)

	typeCounts := make(map[string]int)
	moduleCounts := make(map[string]int)
	for _, r := range resources {
		typeCounts[r.Type]++
		moduleCounts[r.Module]++
	}

	types := make([]string, 0, len(typeCounts))
	for t := range typeCounts {
		types = append(types, t)
	}
	sort.Strings(types)

	modules := make([]string, 0, len(moduleCounts))
	for m := range moduleCounts {
		modules = append(modules, m)
	}
	sort.Strings(modules)

	var lines []string
	lines = append(lines, "# Terraform State Summary")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Workspace:         %s / %s", orgName, workspaceName))
	lines = append(lines, fmt.Sprintf("Terraform version: %s", state.TerraformVersion))
	lines = append(lines, fmt.Sprintf("State serial:      %d", state.Serial))
	lines = append(lines, fmt.Sprintf("State lineage:     %s", state.Lineage))
	lines = append(lines, fmt.Sprintf("Total resources:   %d", len(resources)))
	lines = append(lines, fmt.Sprintf("Total outputs:     %d", len(state.Outputs)))
	lines = append(lines, "")

	if len(types) > 0 {
		lines = append(lines, "## Resource Types")
		for _, t := range types {
			lines = append(lines, fmt.Sprintf("  %-50s %d", t, typeCounts[t]))
		}
		lines = append(lines, "")
	}

	if len(modules) > 1 || (len(modules) == 1 && modules[0] != "(root)") {
		lines = append(lines, "## Modules")
		for _, m := range modules {
			lines = append(lines, fmt.Sprintf("  %-50s %d resources", m, moduleCounts[m]))
		}
		lines = append(lines, "")
	}

	if len(state.Outputs) > 0 {
		lines = append(lines, "## Outputs")
		outputNames := make([]string, 0, len(state.Outputs))
		for name := range state.Outputs {
			outputNames = append(outputNames, name)
		}
		sort.Strings(outputNames)
		for _, name := range outputNames {
			out := state.Outputs[name]
			suffix := ""
			if out.Sensitive {
				suffix = " (sensitive)"
			}
			typeStr := ""
			if out.Type != nil {
				typeStr = fmt.Sprintf(" [%v]", out.Type)
			}
			lines = append(lines, fmt.Sprintf("  %s%s%s", name, typeStr, suffix))
		}
		lines = append(lines, "")
	}

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}
