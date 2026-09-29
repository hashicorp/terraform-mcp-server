// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ListVariableSetsArguments holds the inputs for listing variable sets in an organization.
type ListVariableSetsArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	Query            string `json:"query,omitempty"`
	Pagination
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
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform variable sets",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ListVariableSetsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListVariableSetsArguments) (*mcp.CallToolResult, any, error) {
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

	var buf bytes.Buffer
	if err := jsonapi.MarshalPayloadWithoutIncluded(&buf, variableSets.Items); err != nil {
		return nil, nil, fmt.Errorf("marshalling variable sets: %w", err)
	}

	return textResult(buf.String()), nil, nil
}
