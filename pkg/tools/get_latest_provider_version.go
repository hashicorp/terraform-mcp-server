// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetLatestProviderVersion creates a tool to get the latest provider version from the public registry.
func GetLatestProviderVersion(registryClient *http.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_latest_provider_version",
			mcp.WithDescription("Fetches the latest version of a Terraform provider from the publi registry"),
			mcp.WithTitleAnnotation("Get Latest Provider Version"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("namespace",
				mcp.Required(),
				mcp.Description("The namespace of the provider, e.g., 'hashicorp'")),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("The name of the provider, e.g., 'aws'")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getLatestProviderVersionHandler(registryClient, req, logger)
		},
	}
}

func getLatestProviderVersionHandler(registryClient *http.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	namespace, err := request.RequireString("namespace")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "namespace is required", err)
	}
	name, err := request.RequireString("name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "name is required", err)
	}

	version, err := client.GetLatestProviderVersion(registryClient, namespace, name, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "error fetching latest provider version", err)
	}

	return mcp.NewToolResultText(version), nil
}
