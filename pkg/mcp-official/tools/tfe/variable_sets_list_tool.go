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

// ListVariableSetsArguments holds the inputs for listing variable sets in an organization.
type ListVariableSetsArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	Query            string `json:"query,omitempty"`
	Pagination
}

// VariableSetSummary holds a trimmed view of a single variable set for listing.
type VariableSetSummary struct {
	ID          string `json:"variable_set_id"`
	Name        string `json:"variable_set_name"`
	Description string `json:"description"`
	Global      bool   `json:"global"`
	Priority    bool   `json:"priority"`
}

// VariableSetSummaryList contains the list of variable set summaries and pagination details.
type VariableSetSummaryList struct {
	Items []VariableSetSummary `json:"items"`
	PaginationDetails
}

// ListVariableSetsTool describes the list_variable_sets tool.
func ListVariableSetsTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the Terraform Cloud/Enterprise organization",
	}
	properties["query"] = &jsonschema.Schema{
		Type:        "string",
		Description: "Optional filter query for variable set names",
	}

	return &mcp.Tool{
		Name:        "list_variable_sets",
		Description: "List variable sets in an organization. Returns all variable sets when query is empty.",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			PropertyOrder:        []string{"terraform_org_name", "query", "page", "pageSize"},
			Required:             []string{"terraform_org_name"},
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
							"variable_set_id":   {Type: "string"},
							"variable_set_name": {Type: "string"},
							"description":       {Type: "string"},
							"global":            {Type: "boolean"},
							"priority":          {Type: "boolean"},
						},
						PropertyOrder: []string{
							"variable_set_id", "variable_set_name", "description", "global", "priority",
						},
						Required: []string{
							"variable_set_id", "variable_set_name", "description", "global", "priority",
						},
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
			Title:           "List Terraform variable sets",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ListVariableSetsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListVariableSetsArguments) (*mcp.CallToolResult, *VariableSetSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	variableSets, err := tfeClient.VariableSets.List(ctx, terraformOrgName, &tfe.VariableSetListOptions{
		Query:       input.Query,
		ListOptions: input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing variable sets in organization %q: %w", terraformOrgName, err)
	}

	summaries := make([]VariableSetSummary, len(variableSets.Items))
	for i, vs := range variableSets.Items {
		summaries[i] = VariableSetSummary{
			ID:          vs.ID,
			Name:        vs.Name,
			Description: vs.Description,
			Global:      vs.Global,
			Priority:    vs.Priority,
		}
	}

	return nil, &VariableSetSummaryList{
		Items:             summaries,
		PaginationDetails: paginationDetails(variableSets.Pagination),
	}, nil
}
