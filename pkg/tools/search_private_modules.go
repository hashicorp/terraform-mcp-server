// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SearchPrivateModules creates a tool to search for private modules in Terraform Cloud/Enterprise.
func SearchPrivateModules(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_private_modules",
			mcp.WithDescription(`This tool searches for private modules in your Terraform Cloud/Enterprise organization.
It retrieves a list of private modules that match the search criteria. This tool requires a valid Terraform token to be configured.`),
			mcp.WithTitleAnnotation("Search for private modules in Terraform Cloud/Enterprise"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name to search within"),
			),
			mcp.WithString("search_query",
				mcp.Description("Optional search query to filter modules by name or namespace. If not provided, all modules will be returned"),
			),
			mcp.WithNumber("page_size",
				mcp.Description("Number of results to return per page (max 100)"),
				mcp.Min(1),
				mcp.Max(100),
			),
			mcp.WithNumber("page_number",
				mcp.Description("Page number for pagination (starts at 1)"),
				mcp.Min(1),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return searchPrivateModulesHandler(ctx, request, logger)
		},
	}
}

func searchPrivateModulesHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get required parameters
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
	}
	searchQuery := request.GetString("search_query", "")
	pageSize := request.GetInt("page_size", 100)
	pageNumber := request.GetInt("page_number", 1)

	// Validate page size and number
	if pageSize < 1 || pageSize > 100 {
		return mcp.NewToolResultError("page_size must be between 1 and 100"), nil
	}
	if pageNumber < 1 {
		return mcp.NewToolResultError("page_number must be at least 1"), nil
	}

	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		err = utils.LogAndReturnError(logger, "failed to get terraform client for TFE, ensure TFE_TOKEN and TFE_ADDRESS are properly set.", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to get terraform client for TFE: %v", err)), nil
	}

	// Prepare list options
	listOptions := &tfe.RegistryModuleListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pageNumber,
			PageSize:   pageSize,
		},
	}

	// Set search query if provided
	if searchQuery != "" {
		listOptions.Search = searchQuery
	}

	// Include No Code modules in the response
	includeOpts := []tfe.RegistryModuleListIncludeOpt{tfe.IncludeNoCodeModules}
	listOptions.Include = includeOpts

	logger.WithFields(log.Fields{
		"organization": terraformOrgName,
		"search_query": searchQuery,
		"page_size":    pageSize,
		"page_number":  pageNumber,
	}).Info("Searching for private modules")

	// Call the TFE API to list modules
	moduleList, err := tfeClient.RegistryModules.List(ctx, terraformOrgName, listOptions)
	if err != nil {
		logger.WithError(err).Error("failed to list private modules")
		return mcp.NewToolResultError(fmt.Sprintf("failed to list private modules: %v", err)), nil
	}

	// Build response
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Private Modules in Organization: %s\n", terraformOrgName))
	if searchQuery != "" {
		builder.WriteString(fmt.Sprintf("Search Query: %s\n", searchQuery))
	}
	builder.WriteString(fmt.Sprintf("Page: %d, Size: %d\n\n", pageNumber, pageSize))

	if len(moduleList.Items) == 0 {
		builder.WriteString("No private modules found matching the search criteria.\n")
		if searchQuery != "" {
			builder.WriteString("Try:\n")
			builder.WriteString("- Using a broader search query\n")
			builder.WriteString("- Checking the organization name\n")
			builder.WriteString("- Verifying that private modules exist in this organization\n")
		}
		return mcp.NewToolResultText(builder.String()), nil
	}

	builder.WriteString(fmt.Sprintf("Found %d module(s):\n", len(moduleList.Items)))
	builder.WriteString("(Use the 'private_module_id' value with get_private_module_details tool)\n\n")

	for i, module := range moduleList.Items {
		moduleID := fmt.Sprintf("%s/%s/%s", module.Namespace, module.Name, module.Provider)
		builder.WriteString(fmt.Sprintf("%d  private_module_id: %s\n", i+1, moduleID))
		builder.WriteString(fmt.Sprintf("   Module Name: %s\n", module.Name))
		builder.WriteString(fmt.Sprintf("   Module Namespace: %s\n", module.Namespace))
		builder.WriteString(fmt.Sprintf("   Registry: %s\n", module.RegistryName))
		builder.WriteString(fmt.Sprintf("   Created: %s\n", module.CreatedAt))
		builder.WriteString(fmt.Sprintf("   Updated: %s\n", module.UpdatedAt))
		builder.WriteString(fmt.Sprintf("   Provider: %s\n", module.Provider))
		builder.WriteString(fmt.Sprintf("   No Code Module: %t\n", module.NoCode))

		builder.WriteString("\n")
	}

	// Add pagination information
	if moduleList.Pagination != nil {
		builder.WriteString("Pagination:\n")
		builder.WriteString(fmt.Sprintf("- Current Page: %d\n", moduleList.Pagination.CurrentPage))
		builder.WriteString(fmt.Sprintf("- Total Pages: %d\n", moduleList.Pagination.TotalPages))
		builder.WriteString(fmt.Sprintf("- Total Count: %d\n", moduleList.Pagination.TotalCount))

		if moduleList.Pagination.NextPage > 0 {
			builder.WriteString(fmt.Sprintf("- Next Page: %d\n", moduleList.Pagination.NextPage))
		}
		if moduleList.Pagination.PreviousPage > 0 {
			builder.WriteString(fmt.Sprintf("- Previous Page: %d\n", moduleList.Pagination.PreviousPage))
		}
	}

	logger.WithFields(log.Fields{
		"organization":  terraformOrgName,
		"modules_found": len(moduleList.Items),
	}).Info("Successfully retrieved private modules")

	return mcp.NewToolResultText(builder.String()), nil
}
