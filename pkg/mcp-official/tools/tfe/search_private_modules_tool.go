// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchPrivateModulesArguments holds the filters for searching private modules.
type SearchPrivateModulesArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	SearchQuery      string `json:"search_query,omitempty"`
	Pagination
}

// PrivateModuleSummary contains the module fields returned by a search.
type PrivateModuleSummary struct {
	PrivateModuleID string   `json:"private_module_id"`
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Provider        string   `json:"provider"`
	RegistryName    string   `json:"registry_name"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	NoCode          bool     `json:"no_code"`
	NoCodeModuleIDs []string `json:"no_code_module_ids"`
}

// PrivateModuleSummaryList contains matching modules and pagination details.
type PrivateModuleSummaryList struct {
	Items []*PrivateModuleSummary `json:"items"`
	PaginationDetails
}

// SearchPrivateModulesTool describes the search_private_modules tool.
func SearchPrivateModulesTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The Terraform organization name to search for private modules in",
	}
	properties["search_query"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional search query to filter modules by name or namespace. If omitted, all modules are returned",
	}

	return &mcp.Tool{
		Name:        "search_private_modules",
		Description: "This tool searches for private modules in your Terraform Cloud/Enterprise organization. It retrieves a list of private modules that match the search criteria. This tool requires a valid Terraform token to be configured.",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			Required:             []string{"terraform_org_name"},
			PropertyOrder:        []string{"terraform_org_name", "search_query", "page", "pageSize"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search for private modules in Terraform Cloud/Enterprise",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

// SearchPrivateModulesFunc searches private modules using the caller's Terraform session.
func SearchPrivateModulesFunc(
	ctx context.Context,
	request *mcp.CallToolRequest,
	input SearchPrivateModulesArguments,
) (*mcp.CallToolResult, *PrivateModuleSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	options := &tfe.RegistryModuleListOptions{
		ListOptions: input.ListOptions(),
		Search:      strings.TrimSpace(input.SearchQuery),
		Include:     []tfe.RegistryModuleListIncludeOpt{tfe.IncludeNoCodeModules},
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	modules, err := tfeClient.RegistryModules.List(ctx, terraformOrgName, options)
	if err != nil {
		return nil, nil, fmt.Errorf("listing private modules in organization %q: %w", terraformOrgName, err)
	}

	return nil, privateModuleSummaryList(modules), nil
}

func privateModuleSummaryList(modules *tfe.RegistryModuleList) *PrivateModuleSummaryList {
	items := make([]*PrivateModuleSummary, len(modules.Items))
	for i, module := range modules.Items {
		noCodeModuleIDs := make([]string, 0, len(module.RegistryNoCodeModule))
		if module.NoCode {
			for _, noCodeModule := range module.RegistryNoCodeModule {
				if noCodeModule != nil {
					noCodeModuleIDs = append(noCodeModuleIDs, noCodeModule.ID)
				}
			}
		}

		items[i] = &PrivateModuleSummary{
			PrivateModuleID: module.Namespace + "/" + module.Name + "/" + module.Provider,
			Name:            module.Name,
			Namespace:       module.Namespace,
			Provider:        module.Provider,
			RegistryName:    string(module.RegistryName),
			CreatedAt:       module.CreatedAt,
			UpdatedAt:       module.UpdatedAt,
			NoCode:          module.NoCode,
			NoCodeModuleIDs: noCodeModuleIDs,
		}
	}

	return &PrivateModuleSummaryList{
		Items:             items,
		PaginationDetails: paginationDetails(modules.Pagination),
	}
}
