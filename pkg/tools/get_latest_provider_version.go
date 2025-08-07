// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
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
		return nil, utils.LogAndReturnError(logger, "namespace of the Terraform provider is required", err)
	}
	namespace = strings.ToLower(namespace)

	name, err := request.RequireString("name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "name of the Terraform provider is required", err)
	}
	name = strings.ToLower(name)

	// Get a simple http client to access the public Terraform registry from context
	terraformClients, err := client.GetTerraformClientFromContext(ctx, logger)
	if err != nil {
		logger.WithError(err).Error("failed to get http client for public Terraform registry")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
	}

	httpClient := terraformClients.HttpClient

	version, err := client.GetLatestProviderVersion(httpClient, namespace, name, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "error fetching latest provider version", err)
	}

	return mcp.NewToolResultText(version), nil
}
