package mcpofficial

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type OrganizationSummary struct {
	Name      string    `json:"organization_name"`
	Email     string    `json:"organization_email"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationSummaryList contains the list of organization summaries and pagination details
type OrganizationSummaryList struct {
	Items []*OrganizationSummary `json:"items"`
	*tfe.Pagination
}

type ListOrganizationsArguments struct {
	// Optional fields (will be zero values if not provided)
	Page     int `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize int `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

func ListTerraformOrganizationsTool() *mcp.Tool {
	trueVal := true
	falseVal := false
	return &mcp.Tool{
		Name:        "list_terraform_orgs",
		Description: "Fetches a list of all Terraform organizations. Supports Pagination for large result sets.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "List all Terraform organizations",
			ReadOnlyHint:    trueVal,
			DestructiveHint: &falseVal,
		},
	}
}

func ListTerraformOrganizationsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListOrganizationsArguments) (*mcp.CallToolResult, *OrganizationSummaryList, error) {
	log.Info("ListTerraformOrganizations for official mcp go-sdk called..")

	tfeClient, err := GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	orgs, err := tfeClient.Organizations.List(ctx, &tfe.OrganizationListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list Terraform organizations: %w", err)
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
		Items:      summaries,
		Pagination: orgs.Pagination,
	}, nil
}
