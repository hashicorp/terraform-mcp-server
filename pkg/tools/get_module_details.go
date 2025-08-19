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
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const MODULE_BASE_PATH = "registry://modules"

func ModuleDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_module_details",
			mcp.WithDescription(`Fetches up-to-date documentation on how to use a Terraform module. You must call 'search_modules' first to obtain the exact valid and compatible module_id required to use this tool.`),
			mcp.WithTitleAnnotation("Retrieve documentation for a specific Terraform module"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("module_id",
				mcp.Required(),
				mcp.Description("Exact valid and compatible module_id retrieved from search_modules (e.g., 'squareops/terraform-kubernetes-mongodb/mongodb/2.1.1', 'GoogleCloudPlatform/vertex-ai/google/0.2.0')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getModuleDetailsHandler(ctx, request, logger)
		},
	}
}

func getModuleDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	moduleID, err := request.RequireString("module_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "module_id is required", err)
	}
	if moduleID == "" {
		return nil, utils.LogAndReturnError(logger, "module_id cannot be empty", nil)
	}
	moduleID = strings.ToLower(moduleID)

	// Get a simple http client to access the public Terraform registry from context
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		logger.WithError(err).Error("failed to get http client for public Terraform registry")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
	}

	var errMsg string
	response, err := getModuleDetails(httpClient, moduleID, 0, logger)
	if err != nil {
		errMsg = fmt.Sprintf("no module(s) found for %v,", moduleID)
		return nil, utils.LogAndReturnError(logger, errMsg, nil)
	}
	moduleData, err := unmarshalTerraformModule(response)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "unmarshalling module details", err)
	}
	if moduleData == "" {
		errMsg = fmt.Sprintf("getting module(s), none found! %s please provider a different moduleProvider", errMsg)
		return nil, utils.LogAndReturnError(logger, errMsg, nil)
	}
	return mcp.NewToolResultText(moduleData), nil
}

func getModuleDetails(httpClient *http.Client, moduleID string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleID != "" {
		uri = fmt.Sprintf("modules/%s", moduleID)
	}

	uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	response, err := client.SendRegistryCall(httpClient, "GET", uri, logger)
	if err != nil {
		// We shouldn't log the error here because we might hit a namespace that doesn't exist, it's better to let the caller handle it.
		return nil, fmt.Errorf("getting module(s) for: %v, please provide a different provider name like aws, azurerm or google etc", moduleID)
	}

	// Return the filtered JSON as a string
	return response, nil
}

func unmarshalTerraformModule(response []byte) (string, error) {
	// Handles one module
	var terraformModules client.TerraformModuleVersionDetails
	err := json.Unmarshal(response, &terraformModules)
	if err != nil {
		return "", utils.LogAndReturnError(nil, "unmarshalling module details", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# %s/%s/%s\n\n", MODULE_BASE_PATH, terraformModules.Namespace, terraformModules.Name))
	builder.WriteString(fmt.Sprintf("**Description:** %s\n\n", terraformModules.Description))
	builder.WriteString(fmt.Sprintf("**Module Version:** %s\n\n", terraformModules.Version))
	builder.WriteString(fmt.Sprintf("**Namespace:** %s\n\n", terraformModules.Namespace))
	builder.WriteString(fmt.Sprintf("**Source:** %s\n\n", terraformModules.Source))

	// Format Inputs
	if len(terraformModules.Root.Inputs) > 0 {
		builder.WriteString("### Inputs\n\n")
		builder.WriteString("| Name | Type | Description | Default | Required |\n")
		builder.WriteString("|---|---|---|---|---|\n")
		for _, input := range terraformModules.Root.Inputs {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | `%v` | %t |\n",
				input.Name,
				input.Type,
				input.Description, // Consider cleaning potential newlines/markdown
				input.Default,
				input.Required,
			))
		}
		builder.WriteString("\n")
	}

	// Format Outputs
	if len(terraformModules.Root.Outputs) > 0 {
		builder.WriteString("### Outputs\n\n")
		builder.WriteString("| Name | Description |\n")
		builder.WriteString("|---|---|\n")
		for _, output := range terraformModules.Root.Outputs {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n",
				output.Name,
				output.Description, // Consider cleaning potential newlines/markdown
			))
		}
		builder.WriteString("\n")
	}

	// Format Provider Dependencies
	if len(terraformModules.Root.ProviderDependencies) > 0 {
		builder.WriteString("### Provider Dependencies\n\n")
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|---|---|---|---|\n")
		for _, dep := range terraformModules.Root.ProviderDependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				dep.Name,
				dep.Namespace,
				dep.Source,
				dep.Version,
			))
		}
		builder.WriteString("\n")
	}

	// Format Examples
	if len(terraformModules.Examples) > 0 {
		builder.WriteString("### Examples\n\n")
		for _, example := range terraformModules.Examples {
			builder.WriteString(fmt.Sprintf("#### %s\n\n", example.Name))
			// Optionally, include more details from example if needed, like inputs/outputs
			// For now, just listing the name.
			if example.Readme != "" {
				builder.WriteString("**Readme:**\n\n")
				// Append readme content, potentially needs markdown escaping/sanitization depending on source
				builder.WriteString(example.Readme)
				builder.WriteString("\n\n")
			}
		}
		builder.WriteString("\n")
	}

	content := builder.String()
	return content, nil
}
