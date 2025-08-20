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

// SearchPrivateProviders creates a tool to search for private providers in Terraform Cloud/Enterprise.
func SearchPrivateProviders(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("search_private_providers",
			mcp.WithDescription(`This tool searches for private providers in your Terraform Cloud/Enterprise organization.
It retrieves a list of private providers that match the search criteria. This tool requires a valid Terraform token to be configured.`),
			mcp.WithTitleAnnotation("Search for private providers in Terraform Cloud/Enterprise"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name to search within"),
			),
			mcp.WithString("search_query",
				mcp.Description("Optional search query to filter providers by name or namespace. If not provided, all providers will be returned"),
			),
			mcp.WithString("registry_name",
				mcp.Description("The type of Terraform registry to search within Terraform Cloud/Enterprise (e.g., 'private', 'public')"),
				mcp.Enum("private", "public"),
				mcp.DefaultString("private"),
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
			return searchPrivateProvidersHandler(ctx, request, logger)
		},
	}
}

func searchPrivateProvidersHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get required parameters
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required for the Terraform Cloud/Enterprise organization.", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	// Get optional parameters
	searchQuery := strings.TrimSpace(request.GetString("search_query", ""))
	registryName := strings.TrimSpace(request.GetString("registry_name", "private"))
	pageSize := request.GetInt("page_size", 20)
	pageNumber := request.GetInt("page_number", 1)

	// Validate page size
	if pageSize < 1 || pageSize > 100 {
		return nil, utils.LogAndReturnError(logger, "page_size must be between 1 and 100", nil)
	}

	// Validate page number
	if pageNumber < 1 {
		return nil, utils.LogAndReturnError(logger, "page_number must be greater than 0", nil)
	}

	// Get the terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		err = utils.LogAndReturnError(logger, "failed to get terraform client for TFE, ensure TFE_TOKEN and TFE_ADDRESS are properly set.", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to get terraform client for TFE: %v", err)), nil
	}

	// Prepare list options
	listOptions := &tfe.RegistryProviderListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pageNumber,
			PageSize:   pageSize,
		},
	}

	// Set registry name filter
	if registryName != "" {
		listOptions.RegistryName = tfe.RegistryName(registryName)
	}

	// Set search query if provided
	if searchQuery != "" {
		listOptions.Search = searchQuery
	}

	// Include provider versions in the response
	includeOpts := []tfe.RegistryProviderIncludeOps{tfe.RegistryProviderVersionsInclude}
	listOptions.Include = &includeOpts

	logger.WithFields(log.Fields{
		"organization":  terraformOrgName,
		"search_query":  searchQuery,
		"registry_name": registryName,
		"page_size":     pageSize,
		"page_number":   pageNumber,
	}).Info("Searching for private providers")

	// Call the TFE API to list providers
	providerList, err := tfeClient.RegistryProviders.List(ctx, terraformOrgName, listOptions)
	if err != nil {
		logger.WithError(err).Error("failed to list private providers")
		return mcp.NewToolResultError(fmt.Sprintf("failed to list private providers: %v", err)), nil
	}

	// Build response
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Private Providers in Organization: %s\n", terraformOrgName))
	if searchQuery != "" {
		builder.WriteString(fmt.Sprintf("Search Query: %s\n", searchQuery))
	}
	builder.WriteString(fmt.Sprintf("Registry: %s\n", registryName))
	builder.WriteString(fmt.Sprintf("Page: %d, Size: %d\n\n", pageNumber, pageSize))

	if len(providerList.Items) == 0 {
		builder.WriteString("No private providers found matching the search criteria.\n")
		if searchQuery != "" {
			builder.WriteString("Try:\n")
			builder.WriteString("- Using a broader search query\n")
			builder.WriteString("- Checking the organization name\n")
			builder.WriteString("- Verifying that private providers exist in this organization\n")
		}
		return mcp.NewToolResultText(builder.String()), nil
	}

	builder.WriteString(fmt.Sprintf("Found %d provider(s):\n\n", len(providerList.Items)))

	for i, provider := range providerList.Items {
		builder.WriteString(fmt.Sprintf("%d. Provider: %s/%s\n", i+1, provider.Namespace, provider.Name))
		builder.WriteString(fmt.Sprintf("   ID: %s\n", provider.ID))
		builder.WriteString(fmt.Sprintf("   Registry: %s\n", provider.RegistryName))
		builder.WriteString(fmt.Sprintf("   Created: %s\n", provider.CreatedAt))
		builder.WriteString(fmt.Sprintf("   Updated: %s\n", provider.UpdatedAt))

		// Show available versions if included
		if len(provider.RegistryProviderVersions) > 0 {
			builder.WriteString("   Versions: ")
			versions := make([]string, len(provider.RegistryProviderVersions))
			for j, version := range provider.RegistryProviderVersions {
				versions[j] = version.Version
			}
			builder.WriteString(strings.Join(versions, ", "))
			builder.WriteString("\n")
		}

		builder.WriteString("\n")
	}

	// Add pagination information
	if providerList.Pagination != nil {
		builder.WriteString("Pagination:\n")
		builder.WriteString(fmt.Sprintf("- Current Page: %d\n", providerList.Pagination.CurrentPage))
		builder.WriteString(fmt.Sprintf("- Total Pages: %d\n", providerList.Pagination.TotalPages))
		builder.WriteString(fmt.Sprintf("- Total Count: %d\n", providerList.Pagination.TotalCount))

		if providerList.Pagination.NextPage > 0 {
			builder.WriteString(fmt.Sprintf("- Next Page: %d\n", providerList.Pagination.NextPage))
		}
		if providerList.Pagination.PreviousPage > 0 {
			builder.WriteString(fmt.Sprintf("- Previous Page: %d\n", providerList.Pagination.PreviousPage))
		}
	}

	logger.WithFields(log.Fields{
		"organization":    terraformOrgName,
		"providers_found": len(providerList.Items),
	}).Info("Successfully retrieved private providers")

	return mcp.NewToolResultText(builder.String()), nil
}
