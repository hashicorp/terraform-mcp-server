// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
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
			mcp.WithDestructiveHintAnnotation(false),
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
		return ToolError(logger, "missing required input: module_id", err)
	}
	if moduleID == "" {
		return ToolError(logger, "module_id cannot be empty", nil)
	}

	// Validate module ID format
	if err := validateModuleID(moduleID); err != nil {
		return ToolError(logger, err.Error(), nil)
	}

	moduleID = strings.ToLower(moduleID)

	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get http client for public Terraform registry", err)
	}

	response, err := getModuleDetails(httpClient, moduleID, 0, logger)
	if err != nil {
		return ToolErrorf(logger, "module not found: %s - use search_modules first to find valid module IDs", moduleID)
	}

	moduleData, err := unmarshalTerraformModule(response)
	if err != nil {
		return ToolError(logger, "failed to parse module details", err)
	}
	if moduleData == "" {
		return ToolErrorf(logger, "no module data returned for %s - try a different module_id", moduleID)
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
		return nil, fmt.Errorf("getting module(s) for: %v, please provide a different provider name like aws, azurerm or google etc", moduleID)
	}

	return response, nil
}

func unmarshalTerraformModule(response []byte) (string, error) {
	var terraformModules client.TerraformModuleVersionDetails
	err := json.Unmarshal(response, &terraformModules)
	if err != nil {
		return "", fmt.Errorf("unmarshalling module details: %w", err)
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
				input.Description,
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
				output.Description,
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
			if example.Readme != "" {
				builder.WriteString("**Readme:**\n\n")
				builder.WriteString(example.Readme)
				builder.WriteString("\n\n")
			}
		}
		builder.WriteString("\n")
	}

	content := builder.String()
	return content, nil
}

func validateModuleID(moduleID string) error {
	parts := strings.Split(moduleID, "/")
	if len(parts) != 4 {
		return fmt.Errorf("invalid module ID format '%s'. Expected format: namespace/name/provider/version (4 parts). Use search_modules to find valid module IDs", moduleID)
	}
	return nil
}
