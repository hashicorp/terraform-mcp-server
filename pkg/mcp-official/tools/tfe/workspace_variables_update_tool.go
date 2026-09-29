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

type UpdateWorkspaceVariableArguments struct {
	TerraformOrgName string  `json:"terraform_org_name"`
	WorkspaceName    string  `json:"workspace_name"`
	VariableID       string  `json:"variable_id"`
	Key              string  `json:"variable_key,omitempty"`
	Value            *string `json:"variable_value,omitempty"`
	Description      *string `json:"description,omitempty"`
	Category         string  `json:"category,omitempty"`
	HCL              *bool   `json:"hcl,omitempty"`
	Sensitive        *bool   `json:"sensitive,omitempty"`
}

func UpdateWorkspaceVariableTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "update_workspace_variable",
		Description: "Updates an existing variable in a Terraform workspace. Only the fields you provide are changed. Use list_workspace_variables to find the variable_id.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"workspace_name": {
					Type:        "string",
					Description: "The name of the workspace the variable belongs to",
				},
				"variable_id": {
					Type:        "string",
					Description: "The ID of the variable to update (e.g. var-abc123def456)",
				},
				"variable_key": {
					Type:        "string",
					Description: "New name for the variable",
				},
				"variable_value": {
					Type:        "string",
					Description: "New value for the variable",
				},
				"description": {
					Type:        "string",
					Description: "New description for the variable",
				},
				"category": {
					Type:        "string",
					Description: "New category for the variable",
					Enum:        []any{variableCategoryTerraform, variableCategoryEnv},
				},
				"hcl": {
					Type:        "boolean",
					Description: "Whether the value is HCL syntax rather than a plain string",
				},
				"sensitive": {
					Type:        "boolean",
					Description: "Whether the value is sensitive. Once set to true this cannot be reverted",
				},
			},
			PropertyOrder:        []string{"terraform_org_name", "workspace_name", "variable_id", "variable_key", "variable_value", "description", "category", "hcl", "sensitive"},
			Required:             []string{"terraform_org_name", "workspace_name", "variable_id"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Update a variable in a Terraform workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func UpdateWorkspaceVariableFunc(ctx context.Context, request *mcp.CallToolRequest, input UpdateWorkspaceVariableArguments) (*mcp.CallToolResult, *VariableSummary, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}
	variableID := strings.TrimSpace(input.VariableID)
	if variableID == "" {
		return nil, nil, fmt.Errorf("variable_id must not be blank")
	}

	options := tfe.VariableUpdateOptions{}
	changed := false

	if key := strings.TrimSpace(input.Key); key != "" {
		options.Key = tfe.String(key)
		changed = true
	}
	if input.Value != nil {
		options.Value = input.Value
		changed = true
	}
	if input.Description != nil {
		options.Description = input.Description
		changed = true
	}
	if category := strings.ToLower(strings.TrimSpace(input.Category)); category != "" {
		if category != variableCategoryTerraform && category != variableCategoryEnv {
			return nil, nil, fmt.Errorf("invalid category %q, must be %q or %q", category, variableCategoryTerraform, variableCategoryEnv)
		}
		options.Category = tfe.Category(tfe.CategoryType(category))
		changed = true
	}
	if input.HCL != nil {
		options.HCL = input.HCL
		changed = true
	}
	if input.Sensitive != nil {
		options.Sensitive = input.Sensitive
		changed = true
	}
	if !changed {
		return nil, nil, fmt.Errorf("at least one of variable_key, variable_value, description, category, hcl or sensitive must be provided")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("reading workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}

	variable, err := tfeClient.Variables.Update(ctx, workspace.ID, variableID, options)
	if err != nil {
		return nil, nil, fmt.Errorf("updating variable %q in workspace %q: %w", variableID, workspaceName, err)
	}

	summary := variableToSummary(variable)
	return nil, &summary, nil
}
