// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetLatestModuleVersion creates a tool to get the latest module version from the public registry.
func GetLatestModuleVersion(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_latest_module_version",
			mcp.WithDescription("Fetches the latest version of a Terraform module from the public registry"),
			mcp.WithTitleAnnotation("Get Latest Module Version"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("module_publisher",
				mcp.Required(),
				mcp.Description("The publisher of the module, e.g., 'hashicorp', 'aws-ia', 'terraform-google-modules', 'Azure' etc.")),
			mcp.WithString("module_name",
				mcp.Required(),
				mcp.Description("The name of the module, this is usually the service or group of service the user is deploying e.g., 'security-group', 'secrets-manager' etc.")),
			mcp.WithString("module_provider",
				mcp.Required(),
				mcp.Description("The name of the Terraform provider for the module, e.g., 'aws', 'google', 'azurerm' etc.")),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getLatestModuleVersionHandler(ctx, req, logger)
		},
	}
}

func getLatestModuleVersionHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	modulePublisher, err := request.RequireString("module_publisher")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "required input: 'module_publisher' (the publisher of the module)", err)
	}
	modulePublisher = strings.ToLower(modulePublisher)

	moduleName, err := request.RequireString("module_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "required input: 'module_name' (the name of the module)", err)
	}
	moduleName = strings.ToLower(moduleName)

	moduleProvider, err := request.RequireString("module_provider")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "required input: 'module_provider' (the provider of the module)", err)
	}
	moduleProvider = strings.ToLower(moduleProvider)

	// Get a simple http client to access the public Terraform registry from context
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		logger.WithError(err).Error("failed to get http client for public Terraform registry")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
	}
	uri := fmt.Sprintf("modules/%s/%s/%s", modulePublisher, moduleName, moduleProvider)
	response, err := client.SendRegistryCall(httpClient, http.MethodGet, uri, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("fetching module information for %s/%s from the %s provider", modulePublisher, moduleName, moduleProvider), err)
	}

	var moduleVersionDetails client.TerraformModuleVersionDetails
	if err := json.Unmarshal(response, &moduleVersionDetails); err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("unmarshalling module information for %s/%s from the %s provider", modulePublisher, moduleName, moduleProvider), err)
	}

	return mcp.NewToolResultText(moduleVersionDetails.Version), nil
}
