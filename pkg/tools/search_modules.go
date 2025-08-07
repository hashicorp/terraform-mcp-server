// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func SearchModules(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_modules",
			mcp.WithDescription(`Resolves a Terraform module name to obtain a compatible module_id for the get_module_details tool and returns a list of matching Terraform modules.
You MUST call this function before 'get_module_details' to obtain a valid and compatible module_id.
When selecting the best match, consider the following:
	- Name similarity to the query
	- Description relevance
	- Verification status (verified)
	- Download counts (popularity)
Return the selected module_id and explain your choice. If there are multiple good matches, mention this but proceed with the most relevant one.
If no modules were found, reattempt the search with a new moduleName query.`),
			mcp.WithTitleAnnotation("Search and match Terraform modules based on name and relevance"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("module_query",
				mcp.Required(),
				mcp.Description("The query to search for Terraform modules."),
			),
			mcp.WithNumber("current_offset",
				mcp.Description("Current offset for pagination"),
				mcp.Min(0),
				mcp.DefaultNumber(0),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getSearchModulesHandler(ctx, request, logger)
		},
	}
}

func getSearchModulesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	moduleQuery, err := request.RequireString("module_query")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "module_query is required", err)
	}
	moduleQuery = strings.ToLower(moduleQuery)
	currentOffsetValue := request.GetInt("current_offset", 0)

	// Get a simple http client to access the public Terraform registry from context
	terraformClients, err := client.GetTerraformClientFromContext(ctx, logger)
	if err != nil {
		logger.WithError(err).Error("failed to get http client for public Terraform registry")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get http client for public Terraform registry: %v", err)), nil
	}

	httpClient := terraformClients.HttpClient

	var modulesData, errMsg string
	response, err := sendSearchModulesCall(httpClient, moduleQuery, currentOffsetValue, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("no module(s) found for moduleName: %s", moduleQuery), err)
	} else {
		modulesData, err = unmarshalTerraformModules(response, moduleQuery)
		if err != nil {
			return nil, utils.LogAndReturnError(logger, fmt.Sprintf("unmarshalling modules for moduleName: %s", moduleQuery), err)
		}
	}

	if modulesData == "" {
		errMsg = fmt.Sprintf("getting module(s), none found! query used: %s; error: %s", moduleQuery, errMsg)
		return nil, utils.LogAndReturnError(logger, errMsg, nil)
	}
	return mcp.NewToolResultText(modulesData), nil
}

func sendSearchModulesCall(providerClient *http.Client, moduleQuery string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleQuery != "" {
		uri = fmt.Sprintf("%s/search?q='%s'&offset=%v", uri, url.PathEscape(moduleQuery), currentOffset)
	} else {
		uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	}

	response, err := client.SendRegistryCall(providerClient, "GET", uri, logger)
	if err != nil {
		// We shouldn't log the error here because we might hit a namespace that doesn't exist, it's better to let the caller handle it.
		return nil, fmt.Errorf("getting module(s) for: %v, call error: %v", moduleQuery, err)
	}

	// Return the filtered JSON as a string
	return response, nil
}

func unmarshalTerraformModules(response []byte, moduleQuery string) (string, error) {
	// Get the list of modules
	var terraformModules client.TerraformModules
	err := json.Unmarshal(response, &terraformModules)
	if err != nil {
		return "", utils.LogAndReturnError(nil, "unmarshalling modules", err)
	}

	if len(terraformModules.Data) == 0 {
		return "", fmt.Errorf("no modules found for query: %s", moduleQuery)
	}

	// Sort by most downloaded
	sort.Slice(terraformModules.Data, func(i, j int) bool {
		return terraformModules.Data[i].Downloads > terraformModules.Data[j].Downloads
	})

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Terraform Modules (top matches) for %s\n\n Each result includes:\n", moduleQuery))
	builder.WriteString("- module_id: The module ID (format: namespace/name/provider-name/module-version)\n")
	builder.WriteString("- Name: The name of the module\n")
	builder.WriteString("- Description: A short description of the module\n")
	builder.WriteString("- Downloads: The total number of times the module has been downloaded\n")
	builder.WriteString("- Verified: Verification status of the module\n")
	builder.WriteString("- Published: The date and time when the module was published\n")
	builder.WriteString("\n\n---\n\n")
	for _, module := range terraformModules.Data {
		builder.WriteString(fmt.Sprintf("- module_id: %s\n", module.ID))
		builder.WriteString(fmt.Sprintf("- Name: %s\n", module.Name))
		builder.WriteString(fmt.Sprintf("- Description: %s\n", module.Description))
		builder.WriteString(fmt.Sprintf("- Downloads: %d\n", module.Downloads))
		builder.WriteString(fmt.Sprintf("- Verified: %t\n", module.Verified))
		builder.WriteString(fmt.Sprintf("- Published: %s\n", module.PublishedAt))
		builder.WriteString("---\n\n")
	}
	return builder.String(), nil
}
