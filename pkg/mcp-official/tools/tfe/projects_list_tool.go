package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProjectSummary is a truncated set of information about a project for listing
type ProjectSummary struct {
	ID   string `json:"project_id"`
	Name string `json:"project_name"`
}

// ProjectSummaryList is a list of project summaries and pagination details
type ProjectSummaryList struct {
	Items []*ProjectSummary `json:"items"`
	*tfe.Pagination
}

// ListProjectsArguments holds the input parameters for listing projects within an organization.
type ListProjectsArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`

	// Optional fields (will be empty strings if not provided)
	Page     int `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize int `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

func ListProjectsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_terraform_projects",
		Description: `Search and list Terraform projects within a specified organization. Supports pagination for large result sets. Returns a truncated summary of the project, use "get_project" to get the full details for a specific project.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "List all Terraform projects",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListProjectsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListProjectsArguments) (*mcp.CallToolResult, *ProjectSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)

	tfeClient, err := client.GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	projects, err := tfeClient.Projects.List(ctx, terraformOrgName, &tfe.ProjectListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list projects in org %q: %w", terraformOrgName, err)
	}
	if len(projects.Items) == 0 {
		return nil, nil, fmt.Errorf("no projects to list in organization %q", terraformOrgName)
	}

	summaries := make([]*ProjectSummary, len(projects.Items))
	for i, p := range projects.Items {
		summaries[i] = &ProjectSummary{
			ID:   p.ID,
			Name: p.Name,
		}
	}
	return nil, &ProjectSummaryList{
		Items:      summaries,
		Pagination: projects.Pagination,
	}, nil
}
