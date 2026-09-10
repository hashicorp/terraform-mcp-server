// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WorkspaceSummary holds a trimmed view of a single Terraform workspace.
type WorkspaceSummary struct {
	ID            string    `json:"id"`
	Name          string    `json:"workspace_name"`
	Description   string    `json:"description"`
	Environment   string    `json:"environment"`
	CreatedAt     time.Time `json:"created_at"`
	ExecutionMode string    `json:"execution_mode"`
}

// WorkspaceSummaryList contains the list of workspace summaries and pagination details
type WorkspaceSummaryList struct {
	Items []*WorkspaceSummary `json:"items"`
	PaginationDetails
}

// ListWorkspacesArguments holds the input parameters for listing workspaces within an organization.
type ListWorkspacesArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name"`

	// Optional fields (will be empty strings if not provided)
	ProjectID    string `json:"project_id,omitempty"`
	SearchQuery  string `json:"search_query,omitempty"`
	Tags         string `json:"tags,omitempty"`
	ExcludeTags  string `json:"exclude_tags,omitempty"`
	WildcardName string `json:"wildcard_name,omitempty"`

	// Optional pagination fields (will be zero values if not provided)
	Pagination
}

func ListWorkspacesTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the Terraform Cloud/Enterprise organization",
	}
	properties["project_id"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional project ID to filter workspaces",
	}
	properties["search_query"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional search query to filter workspaces by name",
	}
	properties["tags"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional comma-separated list of tags to filter workspaces",
	}
	properties["exclude_tags"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional comma-separated list of tags to exclude from results",
	}
	properties["wildcard_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional wildcard pattern to match workspace names",
	}

	return &mcp.Tool{
		Name:        "list_workspaces",
		Description: "Search and list Terraform workspaces within a specified organization. Returns all workspaces when no filters are applied, or filters results based on name patterns, tags, or search queries. Supports pagination for large result sets. Returns a truncated summary of the workspace, use get_workspace_details to get the full details for a specific workspace.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: properties,
			PropertyOrder: []string{
				"terraform_org_name",
				"project_id",
				"search_query",
				"tags",
				"exclude_tags",
				"wildcard_name",
				"page",
				"pageSize",
			},
			Required:             []string{"terraform_org_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: outputSchema[WorkspaceSummaryList]("list_workspaces"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform workspaces with queries",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListWorkspacesFunc() mcp.ToolHandlerFor[ListWorkspacesArguments, *WorkspaceSummaryList] {
	return func(ctx context.Context, request *mcp.CallToolRequest, input ListWorkspacesArguments) (*mcp.CallToolResult, *WorkspaceSummaryList, error) {
		terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
		if terraformOrgName == "" {
			return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
		}

		tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
		if err != nil {
			return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
		}

		workspaces, err := tfeClient.Workspaces.List(ctx, terraformOrgName, &tfe.WorkspaceListOptions{
			ProjectID:    input.ProjectID,
			Search:       input.SearchQuery,
			Tags:         joinTrimmedCSV(input.Tags),
			ExcludeTags:  joinTrimmedCSV(input.ExcludeTags),
			WildcardName: input.WildcardName,
			ListOptions:  input.ListOptions(),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("listing workspaces in organization %q: %w", terraformOrgName, err)
		}
		if len(workspaces.Items) == 0 {
			return nil, nil, fmt.Errorf("no workspaces to list in organization %q", terraformOrgName)
		}

		summaries := make([]*WorkspaceSummary, len(workspaces.Items))
		for i, w := range workspaces.Items {
			summaries[i] = &WorkspaceSummary{
				ID:            w.ID,
				Name:          w.Name,
				Description:   w.Description,
				Environment:   w.Environment,
				CreatedAt:     w.CreatedAt,
				ExecutionMode: w.ExecutionMode,
			}
		}

		return nil, &WorkspaceSummaryList{
			Items:             nonNilSlice(summaries),
			PaginationDetails: paginationDetails(workspaces.Pagination),
		}, nil
	}
}

// joinTrimmedCSV normalizes a comma-separated list by trimming whitespace around
// each element, so "a, b , c" is sent to the API as "a,b,c".
func joinTrimmedCSV(csv string) string {
	if strings.TrimSpace(csv) == "" {
		return ""
	}

	parts := strings.Split(csv, ",")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return strings.Join(parts, ",")
}
