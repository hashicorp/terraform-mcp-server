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

// ProjectSummary is a truncated set of information about a project for listing
type ProjectSummary struct {
	ID   string `json:"project_id"`
	Name string `json:"project_name"`
}

// ProjectSummaryList is a list of project summaries and pagination details
type ProjectSummaryList struct {
	Items []*ProjectSummary `json:"items"`
	PaginationDetails
}

// ListProjectsArguments holds the input parameters for listing projects within an organization.
type ListProjectsArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`

	// Optional pagination fields (will be zero values if not provided)
	Pagination
}

func ListProjectsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "list_terraform_projects",
		Description:  `Search and list Terraform projects within a specified organization. Supports pagination for large result sets. Returns a truncated summary of the project, use "get_project" to get the full details for a specific project.`,
		InputSchema:  withPaginationConstraints(inferSchema[ListProjectsArguments]("list_terraform_projects")),
		OutputSchema: outputSchema[ProjectSummaryList]("list_terraform_projects"),
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
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	projects, err := tfeClient.Projects.List(ctx, terraformOrgName, &tfe.ProjectListOptions{
		ListOptions: input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing projects in organization %q: %w", terraformOrgName, err)
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
		Items:             nonNilSlice(summaries),
		PaginationDetails: paginationDetails(projects.Pagination),
	}, nil
}
