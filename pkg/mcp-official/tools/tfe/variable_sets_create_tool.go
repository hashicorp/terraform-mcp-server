// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateVariableSetArguments holds the inputs for creating a variable set.
type CreateVariableSetArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Global           bool   `json:"global,omitempty"`
}

// CreateVariableSetResponse reports the variable set created by the tool.
type CreateVariableSetResponse struct {
	VariableSetID   string `json:"variable_set_id"`
	VariableSetName string `json:"variable_set_name"`
	Message         string `json:"message"`
}

// CreateVariableSetTool describes the create_variable_set tool.
func CreateVariableSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_variable_set",
		Description: "Create a new variable set in an organization.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"name": {
					Type:        "string",
					Description: "Variable set name",
				},
				"description": {
					Type:        "string",
					Description: "Optional variable set description",
				},
				"global": {
					Type:        "boolean",
					Description: "Whether the variable set applies globally",
					Default:     json.RawMessage("false"),
				},
			},
			PropertyOrder:        []string{"terraform_org_name", "name", "description", "global"},
			Required:             []string{"terraform_org_name", "name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a Terraform variable set",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func CreateVariableSetFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateVariableSetArguments) (*mcp.CallToolResult, *CreateVariableSetResponse, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	name := strings.TrimSpace(input.Name)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	if name == "" {
		return nil, nil, fmt.Errorf("name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	variableSet, err := tfeClient.VariableSets.Create(ctx, terraformOrgName, &tfe.VariableSetCreateOptions{
		Name:        tfe.String(name),
		Description: tfe.String(input.Description),
		Global:      tfe.Bool(input.Global),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating variable set %q in organization %q: %w", name, terraformOrgName, err)
	}

	return nil, &CreateVariableSetResponse{
		VariableSetID:   variableSet.ID,
		VariableSetName: variableSet.Name,
		Message:         "variable set created successfully",
	}, nil
}
