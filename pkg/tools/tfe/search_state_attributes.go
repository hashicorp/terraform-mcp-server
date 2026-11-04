// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// stateAttributeMatch is one resource whose attributes matched a search query.
type stateAttributeMatch struct {
	Address string                 `json:"address"`
	Type    string                 `json:"type"`
	Module  string                 `json:"module"`
	Matches map[string]interface{} `json:"matches"`
}

// SearchStateAttributes creates a tool to search attribute values across all resources in
// a workspace's current state.
func SearchStateAttributes(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_state_attributes",
			mcp.WithDescription("Search for a substring across all resource attribute values in a workspace's current "+
				"Terraform state. Returns matching resources with just the attribute key/value pairs that matched, not "+
				"the full state. Useful for finding resources that reference a specific ARN, IP, name, or identifier "+
				"without fetching the entire state."),
			mcp.WithTitleAnnotation("Search Terraform State Attributes"),
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
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Substring to search for in attribute values (case-insensitive)"),
			),
			mcp.WithString("resource_type",
				mcp.Description("Restrict the search to resources of this type, e.g. 'aws_s3_bucket'"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return searchStateAttributesHandler(ctx, request, logger)
		},
	}
}

// searchStateAttributesHandler handles tool logics and functionality
func searchStateAttributesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	orgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_name", err)
	}
	query, err := request.RequireString("query")
	if err != nil {
		return ToolError(logger, "missing required input: query", err)
	}
	orgName = strings.TrimSpace(orgName)
	workspaceName = strings.TrimSpace(workspaceName)
	query = strings.ToLower(strings.TrimSpace(query))
	resourceType := GetTrimmedString(request, "resource_type", "")

	state, err := resolveWorkspaceState(ctx, orgName, workspaceName, logger)
	if err != nil {
		return ToolError(logger, "loading Terraform state", err)
	}

	var results []stateAttributeMatch
	for _, r := range extractResources(state) {
		if resourceType != "" && !strings.EqualFold(r.Type, resourceType) {
			continue
		}
		if matched := searchAttrValues(r.Attributes, query); len(matched) > 0 {
			results = append(results, stateAttributeMatch{
				Address: r.Address,
				Type:    r.Type,
				Module:  r.Module,
				Matches: matched,
			})
		}
	}
	if results == nil {
		results = []stateAttributeMatch{}
	}

	data, err := json.MarshalIndent(map[string]interface{}{
		"query":   query,
		"count":   len(results),
		"results": results,
	}, "", "  ")
	if err != nil {
		return ToolError(logger, "marshaling response", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// searchAttrValues recursively searches attribute values for query, returning matching key/value pairs.
func searchAttrValues(attrs map[string]interface{}, query string) map[string]interface{} {
	matches := make(map[string]interface{})
	for k, v := range attrs {
		if m := searchOneValue(v, query); m != nil {
			matches[k] = m
		}
	}
	return matches
}

// searchOneValue searches a single attribute value, descending into nested maps and slices.
func searchOneValue(v interface{}, query string) interface{} {
	switch val := v.(type) {
	case string:
		if strings.Contains(strings.ToLower(val), query) {
			return val
		}
	case map[string]interface{}:
		if sub := searchAttrValues(val, query); len(sub) > 0 {
			return sub
		}
	case []interface{}:
		var hits []interface{}
		for _, e := range val {
			if m := searchOneValue(e, query); m != nil {
				hits = append(hits, m)
			}
		}
		if len(hits) > 0 {
			return hits
		}
	default:
		if strings.Contains(strings.ToLower(fmt.Sprintf("%v", val)), query) {
			return val
		}
	}
	return nil
}
