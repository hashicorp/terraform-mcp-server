// Copyright IBM Corp. 2025
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

var variableCategories = []string{string(tfe.CategoryTerraform), string(tfe.CategoryEnv)}

// CreateVariableInVariableSetArguments holds the inputs for creating a variable in a variable set.
type CreateVariableInVariableSetArguments struct {
	VariableSetID string `json:"variable_set_id"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Description   string `json:"description,omitempty"`
	Category      string `json:"category,omitempty"`
	HCL           bool   `json:"hcl,omitempty"`
	Sensitive     bool   `json:"sensitive,omitempty"`
}

// CreateVariableInVariableSetResponse reports the variable created by the tool.
type CreateVariableInVariableSetResponse struct {
	VariableID    string `json:"variable_id"`
	VariableKey   string `json:"variable_key"`
	VariableSetID string `json:"variable_set_id"`
	Message       string `json:"message"`
}

// CreateVariableInVariableSetTool describes the create_variable_in_variable_set tool.
func CreateVariableInVariableSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_variable_in_variable_set",
		Description: "Create a new variable in a variable set.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"variable_set_id": {
					Type:        "string",
					Description: "The ID of the variable set",
				},
				"key": {
					Type:        "string",
					Description: "Variable key or name",
				},
				"value": {
					Type:        "string",
					Description: "Variable value",
				},
				"description": {
					Type:        "string",
					Description: "Optional variable description",
				},
				"category": {
					Type:        "string",
					Description: "Variable category: terraform or env",
					Enum:        enumOf(variableCategories...),
					Default:     json.RawMessage(`"terraform"`),
				},
				"hcl": {
					Type:        "boolean",
					Description: "Whether the variable value contains HCL",
					Default:     json.RawMessage("false"),
				},
				"sensitive": {
					Type:        "boolean",
					Description: "Whether the variable value is sensitive",
					Default:     json.RawMessage("false"),
				},
			},
			PropertyOrder: []string{
				"variable_set_id", "key", "value", "description", "category", "hcl", "sensitive",
			},
			Required:             []string{"variable_set_id", "key", "value"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a variable in a Terraform variable set",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func CreateVariableInVariableSetFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateVariableInVariableSetArguments) (*mcp.CallToolResult, *CreateVariableInVariableSetResponse, error) {
	variableSetID := strings.TrimSpace(input.VariableSetID)
	if variableSetID == "" {
		return nil, nil, fmt.Errorf("variable_set_id must not be blank")
	}

	key := strings.TrimSpace(input.Key)
	if key == "" {
		return nil, nil, fmt.Errorf("key must not be blank")
	}

	category := tfe.CategoryTerraform
	switch strings.TrimSpace(input.Category) {
	case "", string(tfe.CategoryTerraform):
	case string(tfe.CategoryEnv):
		category = tfe.CategoryEnv
	default:
		return nil, nil, fmt.Errorf("category must be one of: terraform, env")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	variable, err := tfeClient.VariableSetVariables.Create(ctx, variableSetID, &tfe.VariableSetVariableCreateOptions{
		Key:         tfe.String(key),
		Value:       tfe.String(input.Value),
		Description: tfe.String(input.Description),
		Category:    &category,
		HCL:         tfe.Bool(input.HCL),
		Sensitive:   tfe.Bool(input.Sensitive),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating variable %q in variable set %q: %w", key, variableSetID, err)
	}

	return nil, &CreateVariableInVariableSetResponse{
		VariableID:    variable.ID,
		VariableKey:   variable.Key,
		VariableSetID: variableSetID,
		Message:       "variable created successfully",
	}, nil
}
