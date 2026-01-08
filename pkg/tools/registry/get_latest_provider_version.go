// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetLatestProviderVersion creates a tool to get the latest provider version from the public registry.
func GetLatestProviderVersion(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_latest_provider_version",
			mcp.WithDescription("Fetches the latest version of a Terraform provider from the public registry"),
			mcp.WithTitleAnnotation("Get Latest Provider Version"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("namespace",
				mcp.Required(),
				mcp.Description("The namespace of the Terraform provider, typically the name of the company, or their GitHub organization name that created the provider e.g., 'hashicorp'")),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("The name of the Terraform provider, e.g., 'aws', 'azurerm', 'google', etc.")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getLatestProviderVersionHandler(ctx, req, logger)
		},
	}
}

func getLatestProviderVersionHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return ToolError(logger, "missing required input: namespace", err)
	}
	namespace = strings.ToLower(namespace)

	name, err := request.RequireString("name")
	if err != nil {
		return ToolError(logger, "missing required input: name", err)
	}
	name = strings.ToLower(name)

	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get http client for public Terraform registry", err)
	}

	version, err := client.GetLatestProviderVersion(httpClient, namespace, name, logger)
	if err != nil {
		return ToolErrorf(logger, "provider not found: %s/%s - verify the namespace and provider name are correct", namespace, name)
	}

	return mcp.NewToolResultText(version), nil
}
