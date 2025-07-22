// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-mcp-server/pkg/internal/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func SearchModules(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("searchModules",
			mcp.WithDescription(`Resolves a Terraform module name to obtain a compatible moduleID for the moduleDetails tool and returns a list of matching Terraform modules. You MUST call this function before 'moduleDetails' to obtain a valid and compatible moduleID. When selecting the best match, consider: - Name similarity to the query - Description relevance - Verification status (verified) - Download counts (popularity) Return the selected moduleID and explain your choice. If there are multiple good matches, mention this but proceed with the most relevant one. If no modules were found, reattempt the search with a new moduleName query.`),
			mcp.WithTitleAnnotation("Search and match Terraform modules based on name and relevance"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("moduleQuery",
				mcp.Required(),
				mcp.Description("The query to search for Terraform modules."),
			),
			mcp.WithNumber("currentOffset",
				mcp.Description("Current offset for pagination"),
				mcp.Min(0),
				mcp.DefaultNumber(0),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			moduleQuery, err := request.RequireString("moduleQuery")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "moduleQuery is required", err)
			}
			currentOffsetValue := request.GetInt("currentOffset", 0)

			var modulesData, errMsg string
			response, err := sendSearchModulesCall(registryClient, moduleQuery, currentOffsetValue, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf("no module(s) found for moduleName: %s", moduleQuery), err)
			} else {
				modulesData, err = utils.UnmarshalTFModulePlural(response, moduleQuery)
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
}

func sendSearchModulesCall(providerClient *http.Client, moduleQuery string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleQuery != "" {
		uri = fmt.Sprintf("%s/search?q='%s'&offset=%v", uri, url.PathEscape(moduleQuery), currentOffset)
	} else {
		uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	}

	response, err := utils.SendRegistryCall(providerClient, "GET", uri, logger)
	if err != nil {
		// We shouldn't log the error here because we might hit a namespace that doesn't exist, it's better to let the caller handle it.
		return nil, fmt.Errorf("getting module(s) for: %v, call error: %v", moduleQuery, err)
	}

	// Return the filtered JSON as a string
	return response, nil
}
