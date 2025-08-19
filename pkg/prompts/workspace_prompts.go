// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package prompts

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// RegisterWorkspacePrompts adds workspace-related prompt resources to the MCP server.
// This function registers all workspace prompt resources including workspace analysis
// prompts that can be accessed by MCP clients.
func RegisterWorkspacePrompts(hcServer *server.MCPServer, logger *log.Logger) {
	hcServer.AddResource(WorkspaceAnalysisPromptResource(logger))
}

// WorkspaceAnalysisPromptResource returns the resource and handler for workspace analysis prompts.
// It creates an MCP resource that provides access to the workspace analysis prompt template
// with the specified URI and description. The handler returns the prompt content in markdown format.
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
					Text:     GetWorkspaceAnalysisPrompt(),
				},
			}, nil
		}
}
