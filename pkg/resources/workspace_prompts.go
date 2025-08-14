// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

//go:embed prompts/workspace_analysis.md
var workspaceAnalysisPrompt string

// RegisterWorkspacePrompts adds workspace-related prompt resources to the MCP server
func RegisterWorkspacePrompts(hcServer *server.MCPServer, logger *log.Logger) {
	hcServer.AddResource(WorkspaceAnalysisPromptResource(logger))
}

// WorkspaceAnalysisPromptResource returns the resource and handler for workspace analysis prompts
func WorkspaceAnalysisPromptResource(logger *log.Logger) (mcp.Resource, server.ResourceHandlerFunc) {
	resourceURI := "/terraform/prompts/workspace-analysis"
	description := "Workspace analysis prompt for comprehensive workspace details retrieval"

	return mcp.NewResource(
			resourceURI,
			description,
			mcp.WithMIMEType("text/markdown"),
			mcp.WithResourceDescription(description),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					MIMEType: "text/markdown",
					URI:      resourceURI,
					Text:     workspaceAnalysisPrompt,
				},
			}, nil
		}
}
