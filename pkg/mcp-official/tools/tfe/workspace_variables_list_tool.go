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

type VariableSummary struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
	HCL         bool   `json:"hcl"`
	Sensitive   bool   `json:"sensitive"`
}

// variableSummarySchema is the output schema for a single VariableSummary.
// shared by list, create and update since all three return the same shape.
func variableSummarySchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"id":          {Type: "string"},
			"key":         {Type: "string"},
			"value":       {Type: "string"},
			"description": {Type: "string"},
			"category":    {Type: "string"},
			"hcl":         {Type: "boolean"},
			"sensitive":   {Type: "boolean"},
		},
		PropertyOrder:        []string{"id", "key", "value", "description", "category", "hcl", "sensitive"},
		Required:             []string{"id", "key", "category", "hcl", "sensitive"},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

// VariableSummaryList is a list of variable summaries and pagination details
type VariableSummaryList struct {
	Items []VariableSummary `json:"items"`
	PaginationDetails
}

type ListWorkspaceVariablesArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	WorkspaceName    string `json:"workspace_name"`
	Pagination
}

func ListWorkspaceVariablesTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the Terraform Cloud/Enterprise organization",
	}
	properties["workspace_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The name of the workspace to list variables for",
	}

	return &mcp.Tool{
		Name:        "list_workspace_variables",
		Description: `List all variables for a Terraform workspace, including variables inherited from attached variable sets. Sensitive variable values are not returned. Supports pagination for large result sets.`,
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			PropertyOrder:        []string{"terraform_org_name", "workspace_name", "page", "pageSize"},
			Required:             []string{"terraform_org_name", "workspace_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"items":        {Type: "array", Items: variableSummarySchema()},
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
			Title:           "List variables in a Terraform workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ListWorkspaceVariablesFunc(ctx context.Context, request *mcp.CallToolRequest, input ListWorkspaceVariablesArguments) (*mcp.CallToolResult, *VariableSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("reading workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}

	variables, err := tfeClient.Variables.ListAll(ctx, workspace.ID, &tfe.VariableListOptions{
		ListOptions: input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing variables for workspace %q: %w", workspaceName, err)
	}

	summaries := make([]VariableSummary, len(variables.Items))
	for i, v := range variables.Items {
		summaries[i] = variableToSummary(v)
	}

	return nil, &VariableSummaryList{
		Items:             summaries,
		PaginationDetails: paginationDetails(variables.Pagination),
	}, nil
}

// variableToSummary maps go-tfe's Variable onto VariableSummary. this will be shared by
// list, create and update so all three return the same shape.
func variableToSummary(v *tfe.Variable) VariableSummary {
	s := VariableSummary{
		ID:          v.ID,
		Key:         v.Key,
		Description: v.Description,
		Category:    string(v.Category),
		HCL:         v.HCL,
		Sensitive:   v.Sensitive,
	}
	// the API returns Value blank for sensitive variables. leave it out rather
	// than emit an empty string that looks like a real value.
	if !v.Sensitive {
		s.Value = v.Value
	}
	return s
}
