// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
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
	Items []OrganizationSummary `json:"items"`
	PaginationDetails
}

// ListOrganizationsArguments holds the optional pagination input for listing organizations.
type ListOrganizationsArguments struct {
	Pagination
}

func ListTerraformOrganizationsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_terraform_orgs",
		Description: "Fetches a list of all Terraform organizations. Supports Pagination for large result sets.",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           paginationSchemaProperties(),
			PropertyOrder:        []string{"page", "pageSize"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"items": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"organization_name":  {Type: "string"},
							"organization_email": {Type: "string"},
							"created_at":         {Type: "string"},
						},
						PropertyOrder:        []string{"organization_name", "organization_email", "created_at"},
						Required:             []string{"organization_name", "organization_email", "created_at"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
				"current-page": {Type: "integer"},
				"prev-page":    {Type: "integer"},
				"next-page":    {Type: "integer"},
				"total-count":  {Type: "integer"},
				"total-pages":  {Type: "integer"},
			},
			PropertyOrder:        []string{"items", "current-page", "prev-page", "next-page", "total-count", "total-pages"},
			Required:             []string{"items"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "List all Terraform organizations",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
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

	// Keep the allocated slice non-nil so an empty page marshals as [] rather than null.
	summaries := make([]OrganizationSummary, len(orgs.Items))
	for i, o := range orgs.Items {
		summaries[i] = OrganizationSummary{
			Name:      o.Name,
			Email:     o.Email,
			CreatedAt: o.CreatedAt,
		}
	}

	return nil, &OrganizationSummaryList{
		Items:             summaries,
		PaginationDetails: paginationDetails(orgs.Pagination),
	}, nil
}
