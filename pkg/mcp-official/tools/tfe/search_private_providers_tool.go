// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	privateRegistry = "private"
	publicRegistry  = "public"
)

var validRegistries = []string{privateRegistry, publicRegistry}

// SearchPrivateProvidersArguments holds the filters for searching private providers.
type SearchPrivateProvidersArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	SearchQuery      string `json:"search_query,omitempty"`
	RegistryName     string `json:"registry_name,omitempty"`
	Pagination
}

// PrivateProviderSummary contains the provider fields returned by a search.
type PrivateProviderSummary struct {
	ID              string   `json:"id"`
	ProviderAddress string   `json:"provider_address"`
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	RegistryName    string   `json:"registry_name"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	Versions        []string `json:"versions"`
}

// PrivateProviderSummaryList contains matching providers and pagination details.
type PrivateProviderSummaryList struct {
	Items []*PrivateProviderSummary `json:"items"`
	PaginationDetails
}

// SearchPrivateProvidersTool describes the search_private_providers tool.
func SearchPrivateProvidersTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The Terraform organization name to search for private providers in",
	}
	properties["search_query"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional search query to filter providers by name or namespace. If omitted, all providers are returned",
	}
	properties["registry_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The Terraform registry to search",
		Enum:        enumOf(validRegistries...),
		Default:     json.RawMessage(`"private"`),
	}

	return &mcp.Tool{
		Name:        "search_private_providers",
		Description: `This tool searches for private providers in your Terraform Cloud/Enterprise organization. It retrieves a list of private providers that match the search criteria. This tool requires a valid Terraform token to be configured.`,
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			Required:             []string{"terraform_org_name"},
			PropertyOrder:        []string{"terraform_org_name", "search_query", "registry_name", "page", "pageSize"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search for private providers in Terraform Cloud/Enterprise",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

// SearchPrivateProvidersFunc searches private providers using the caller's Terraform session.
func SearchPrivateProvidersFunc(
	ctx context.Context,
	request *mcp.CallToolRequest,
	input SearchPrivateProvidersArguments,
) (*mcp.CallToolResult, *PrivateProviderSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	registryName := strings.TrimSpace(input.RegistryName)
	if registryName == "" {
		registryName = privateRegistry
	}
	if !slices.Contains(validRegistries, registryName) {
		return nil, nil, fmt.Errorf("registry_name must be one of %q", validRegistries)
	}

	include := []tfe.RegistryProviderIncludeOps{tfe.RegistryProviderVersionsInclude}
	options := &tfe.RegistryProviderListOptions{
		ListOptions:  input.ListOptions(),
		RegistryName: tfe.RegistryName(registryName),
		Search:       strings.TrimSpace(input.SearchQuery),
		Include:      &include,
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	providers, err := tfeClient.RegistryProviders.List(ctx, terraformOrgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("listing private providers in organization %q: %w", terraformOrgName, err)
	}

	return nil, privateProviderSummaryList(providers), nil
}

func privateProviderSummaryList(providers *tfe.RegistryProviderList) *PrivateProviderSummaryList {
	items := make([]*PrivateProviderSummary, len(providers.Items))
	for i, provider := range providers.Items {
		versions := make([]string, 0, len(provider.RegistryProviderVersions))
		for _, version := range provider.RegistryProviderVersions {
			if version != nil {
				versions = append(versions, version.Version)
			}
		}

		items[i] = &PrivateProviderSummary{
			ID:              provider.ID,
			ProviderAddress: provider.Namespace + "/" + provider.Name,
			Name:            provider.Name,
			Namespace:       provider.Namespace,
			RegistryName:    string(provider.RegistryName),
			CreatedAt:       provider.CreatedAt,
			UpdatedAt:       provider.UpdatedAt,
			Versions:        versions,
		}
	}

	return &PrivateProviderSummaryList{
		Items:             items,
		PaginationDetails: paginationDetails(providers.Pagination),
	}
}
