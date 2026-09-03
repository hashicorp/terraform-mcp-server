// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OrganizationSummary holds a trimmed view of a single Terraform organization.
type OrganizationSummary struct {
	Name      string    `json:"organization_name"`
	Email     string    `json:"organization_email"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationSummaryList contains the list of organization summaries and pagination details
type OrganizationSummaryList struct {
	Items []*OrganizationSummary `json:"items"`
	PaginationDetails
}

// ListOrganizationsArguments holds the optional pagination input for listing organizations.
type ListOrganizationsArguments struct {
	Pagination
}

func ListTerraformOrganizationsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:         "list_terraform_orgs",
		Description:  "Fetches a list of all Terraform organizations. Supports Pagination for large result sets.",
		InputSchema:  withPaginationConstraints(inferSchema[ListOrganizationsArguments]("list_terraform_orgs")),
		OutputSchema: outputSchema[OrganizationSummaryList]("list_terraform_orgs"),
		Annotations: &mcp.ToolAnnotations{
			Title:           "List all Terraform organizations",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListTerraformOrganizationsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListOrganizationsArguments) (*mcp.CallToolResult, *OrganizationSummaryList, error) {
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	orgs, err := tfeClient.Organizations.List(ctx, &tfe.OrganizationListOptions{
		ListOptions: input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing Terraform organizations: %w", err)
	}
	if len(orgs.Items) == 0 {
		return nil, nil, fmt.Errorf("no organizations to list")
	}

	summaries := make([]*OrganizationSummary, len(orgs.Items))
	for i, o := range orgs.Items {
		summaries[i] = &OrganizationSummary{
			Name:      o.Name,
			Email:     o.Email,
			CreatedAt: o.CreatedAt,
		}
	}

	return nil, &OrganizationSummaryList{
		Items:             nonNilSlice(summaries),
		PaginationDetails: paginationDetails(orgs.Pagination),
	}, nil
}
