// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchPrivateModulesArguments holds the input parameters for searching private modules.
type SearchPrivateModulesArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name to search for private modules in"`

	// Optional fields (will be zero values if not provided)
	SearchQuery string `json:"search_query,omitempty" jsonschema:"Optional search query to filter modules by name or namespace. If not provided, all modules will be returned"`
	PageSize    int    `json:"page_size,omitempty" jsonschema:"Number of results to return per page (min 1, max 100)"`
	PageNumber  int    `json:"page_number,omitempty" jsonschema:"Page number for pagination (starts at 1)"`
}

func SearchPrivateModulesTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "search_private_modules",
		Description: `This tool searches for private modules in your Terraform Cloud/Enterprise organization.
It retrieves a list of private modules that match the search criteria. This tool requires a valid Terraform token to be configured.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search for private modules in Terraform Cloud/Enterprise",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func SearchPrivateModulesFunc(ctx context.Context, request *mcp.CallToolRequest, input SearchPrivateModulesArguments) (*mcp.CallToolResult, *string, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	searchQuery := strings.TrimSpace(input.SearchQuery)
	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = 100
	}
	pageNumber := input.PageNumber
	if pageNumber == 0 {
		pageNumber = 1
	}

	if pageSize < 1 || pageSize > 100 {
		return nil, nil, fmt.Errorf("page_size must be between 1 and 100")
	}
	if pageNumber < 1 {
		return nil, nil, fmt.Errorf("page_number must be at least 1")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	listOptions := &tfe.RegistryModuleListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pageNumber,
			PageSize:   pageSize,
		},
		Include: []tfe.RegistryModuleListIncludeOpt{tfe.IncludeNoCodeModules},
	}

	if searchQuery != "" {
		listOptions.Search = searchQuery
	}

	moduleList, err := tfeClient.RegistryModules.List(ctx, terraformOrgName, listOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list private modules in org '%s': %w", terraformOrgName, err)
	}

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
		result := builder.String()
		return nil, &result, nil
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

		if module.NoCode {
			for _, noCodeModule := range module.RegistryNoCodeModule {
				builder.WriteString(fmt.Sprintf("     - no_code_module_id: %s\n", noCodeModule.ID))
			}
		}

		builder.WriteString("\n")
	}

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

	result := builder.String()
	return nil, &result, nil
}
