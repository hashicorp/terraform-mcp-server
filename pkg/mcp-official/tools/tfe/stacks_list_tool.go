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

// StackSummary is a truncated set of information about a stack for listing
type StackSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// StackSummaryList is a list of stack summaries and pagination details
type StackSummaryList struct {
	Items []*StackSummary `json:"items"`
	PaginationDetails
}

// ListStacksArguments is the unmarshal target for list_stacks. schema is
// declared in ListStacksTool, so no jsonschema tags here.
type ListStacksArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	ProjectID        string `json:"project_id,omitempty"`
	SearchQuery      string `json:"search_query,omitempty"`
	Pagination
}

func ListStacksTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the Terraform Cloud/Enterprise organization",
	}
	properties["project_id"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional project ID to filter stacks (e.g., 'prj-abc123def456')",
	}
	properties["search_query"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional search query to filter stacks by name",
	}

	return &mcp.Tool{
		Name:        "list_stacks",
		Description: `List Terraform Stacks within a specified organization. Returns all stacks when no project or search query is supplied. Supports pagination for large result sets. Returns a truncated summary of each stack, use "get_stack_details" to get the full details for a specific stack.`,
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			PropertyOrder:        []string{"terraform_org_name", "project_id", "search_query", "page", "pageSize"},
			Required:             []string{"terraform_org_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform Stacks in an organization",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListStacksFunc(ctx context.Context, request *mcp.CallToolRequest, input ListStacksArguments) (*mcp.CallToolResult, *StackSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	stacks, err := tfeClient.Stacks.List(ctx, terraformOrgName, &tfe.StackListOptions{
		ProjectID:    strings.TrimSpace(input.ProjectID),
		SearchByName: strings.TrimSpace(input.SearchQuery),
		ListOptions:  input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing stacks in organization %q: %w", terraformOrgName, err)
	}
	if len(stacks.Items) == 0 {
		return nil, nil, fmt.Errorf("no stacks to list in organization %q", terraformOrgName)
	}

	summaries := make([]*StackSummary, len(stacks.Items))
	for i, s := range stacks.Items {
		summaries[i] = &StackSummary{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
		}
	}

	return nil, &StackSummaryList{
		Items:             summaries,
		PaginationDetails: paginationDetails(stacks.Pagination),
	}, nil
}
