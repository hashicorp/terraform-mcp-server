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

// SearchPrivateProvidersArguments holds the input parameters for searching private providers.
type SearchPrivateProvidersArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name to search for private providers in"`

	// Optional fields (will be zero values if not provided)
	SearchQuery  string `json:"search_query,omitempty" jsonschema:"Optional search query to filter providers by name or namespace. If not provided, all providers will be returned"`
	RegistryName string `json:"registry_name,omitempty" jsonschema:"The type of Terraform registry to search within Terraform Cloud/Enterprise ('private' or 'public'). Defaults to 'private'"`
	PageSize     int    `json:"page_size,omitempty" jsonschema:"Number of results to return per page (min 1, max 100)"`
	PageNumber   int    `json:"page_number,omitempty" jsonschema:"Page number for pagination (starts at 1)"`
}

func SearchPrivateProvidersTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "search_private_providers",
		Description: `This tool searches for private providers in your Terraform Cloud/Enterprise organization.
It retrieves a list of private providers that match the search criteria. This tool requires a valid Terraform token to be configured.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search for private providers in Terraform Cloud/Enterprise",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func SearchPrivateProvidersFunc(ctx context.Context, request *mcp.CallToolRequest, input SearchPrivateProvidersArguments) (*mcp.CallToolResult, any, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name is required")
	}
	searchQuery := strings.TrimSpace(input.SearchQuery)
	registryName := strings.TrimSpace(input.RegistryName)
	if registryName == "" {
		registryName = "private"
	}
	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = 20
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

	listOptions := &tfe.RegistryProviderListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pageNumber,
			PageSize:   pageSize,
		},
	}

	if registryName != "" {
		listOptions.RegistryName = tfe.RegistryName(registryName)
	}

	if searchQuery != "" {
		listOptions.Search = searchQuery
	}

	includeOpts := []tfe.RegistryProviderIncludeOps{tfe.RegistryProviderVersionsInclude}
	listOptions.Include = &includeOpts

	providerList, err := tfeClient.RegistryProviders.List(ctx, terraformOrgName, listOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list private providers in org '%s': %w", terraformOrgName, err)
	}

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
		result := builder.String()
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result}}}, nil, nil
	}

	builder.WriteString(fmt.Sprintf("Found %d provider(s):\n\n", len(providerList.Items)))

	for i, provider := range providerList.Items {
		builder.WriteString(fmt.Sprintf("%d. Provider: %s/%s\n", i+1, provider.Namespace, provider.Name))
		builder.WriteString(fmt.Sprintf("   ID: %s\n", provider.ID))
		builder.WriteString(fmt.Sprintf("   Registry: %s\n", provider.RegistryName))
		builder.WriteString(fmt.Sprintf("   Created: %s\n", provider.CreatedAt))
		builder.WriteString(fmt.Sprintf("   Updated: %s\n", provider.UpdatedAt))

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

	result := builder.String()
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result}}}, nil, nil
}
