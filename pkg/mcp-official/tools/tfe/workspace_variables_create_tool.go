// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// the var categories accepted by create_workspace_variable.
const (
	variableCategoryTerraform = "terraform"
	variableCategoryEnv       = "env"
)

type CreateWorkspaceVariableArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	WorkspaceName    string `json:"workspace_name"`
	Key              string `json:"variable_key"`
	Value            string `json:"variable_value"`
	Category         string `json:"category"`
	Description      string `json:"description,omitempty"`
	HCL              bool   `json:"hcl,omitempty"`
	Sensitive        bool   `json:"sensitive,omitempty"`
}

func CreateWorkspaceVariableTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_workspace_variable",
		Description: "Creates a new variable in a Terraform workspace. Use category terraform for Terraform input variables and env for environment variables. Set hcl to true only when the value is HCL syntax rather than a plain string.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"workspace_name": {
					Type:        "string",
					Description: "The name of the workspace to create the variable in",
				},
				"variable_key": {
					Type:        "string",
					Description: "The name of the variable",
				},
				"variable_value": {
					Type:        "string",
					Description: "The value of the variable",
				},
				"category": {
					Type:        "string",
					Description: "Whether this is a Terraform input variable or an environment variable",
					Enum:        enumOf(variableCategories...),
				},
				"description": {
					Type:        "string",
					Description: "Optional description of the variable",
				},
				"hcl": {
					Type:        "boolean",
					Description: "Whether the value is HCL syntax (for example a list or map) rather than a plain string",
					Default:     json.RawMessage("false"),
				},
				"sensitive": {
					Type:        "boolean",
					Description: "Whether the value is sensitive. Sensitive values are write-only and not returned by the API",
					Default:     json.RawMessage("false"),
				},
			},
			PropertyOrder:        []string{"terraform_org_name", "workspace_name", "variable_key", "variable_value", "category", "description", "hcl", "sensitive"},
			Required:             []string{"terraform_org_name", "workspace_name", "variable_key", "variable_value", "category"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: variableSummarySchema(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a variable in a Terraform workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func CreateWorkspaceVariableFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateWorkspaceVariableArguments) (*mcp.CallToolResult, *VariableSummary, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return nil, nil, fmt.Errorf("variable_key must not be blank")
	}
	category := strings.ToLower(strings.TrimSpace(input.Category))
	if !slices.Contains(variableCategories, category) {
		return nil, nil, fmt.Errorf("invalid category %q, must be one of: %s", category, strings.Join(variableCategories, ", "))
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("reading workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}

	options := tfe.VariableCreateOptions{
		Key:       tfe.String(key),
		Value:     tfe.String(input.Value),
		Category:  tfe.Category(tfe.CategoryType(category)),
		HCL:       tfe.Bool(input.HCL),
		Sensitive: tfe.Bool(input.Sensitive),
	}
	if description := strings.TrimSpace(input.Description); description != "" {
		options.Description = tfe.String(description)
	}

	variable, err := tfeClient.Variables.Create(ctx, workspace.ID, options)
	if err != nil {
		return nil, nil, fmt.Errorf("creating variable %q in workspace %q: %w", key, workspaceName, err)
	}

	summary := variableToSummary(variable)
	return nil, &summary, nil
}
