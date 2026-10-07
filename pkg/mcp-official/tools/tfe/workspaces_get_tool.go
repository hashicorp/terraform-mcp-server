// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed static/default-cli-run.md
var defaultWorkspaceReadme string

// GetWorkspaceDetailsArguments holds the inputs for reading workspace details.
type GetWorkspaceDetailsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The name of the workspace to get details for"`
}

// GetWorkspaceDetailsResponse contains workspace configuration, variables, and README content.
type GetWorkspaceDetailsResponse struct {
	WorkspaceDetails
	Variables []VariableSummary `json:"variables"`
	Readme    string            `json:"readme"`
}

// GetWorkspaceDetailsTool describes the get_workspace_details tool.
func GetWorkspaceDetailsTool() *mcp.Tool {
	outputSchema := workspaceDetailsSchema()
	outputSchema.Properties["variables"] = &jsonschema.Schema{Type: "array", Items: variableSummarySchema()}
	outputSchema.Properties["readme"] = &jsonschema.Schema{Type: "string"}
	outputSchema.PropertyOrder = append(outputSchema.PropertyOrder, "variables", "readme")
	outputSchema.Required = append(outputSchema.Required, "variables", "readme")

	return &mcp.Tool{
		Name:         "get_workspace_details",
		Description:  "Fetches detailed information about a specific Terraform workspace, including configuration, variables, and README content.",
		OutputSchema: outputSchema,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a Terraform workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetWorkspaceDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetWorkspaceDetailsArguments) (*mcp.CallToolResult, *GetWorkspaceDetailsResponse, error) {
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

	variables := []VariableSummary{}
	variableList, err := tfeClient.Variables.List(ctx, workspace.ID, &tfe.VariableListOptions{})
	if err == nil && variableList != nil {
		variables = make([]VariableSummary, 0, len(variableList.Items))
		for _, variable := range variableList.Items {
			if variable != nil {
				variables = append(variables, variableToSummary(variable))
			}
		}
	}

	readme := defaultWorkspaceReadmeFor(terraformOrgName, workspace.Name)
	readmeReader, err := tfeClient.Workspaces.Readme(ctx, workspace.ID)
	if err == nil && readmeReader != nil {
		if content, err := io.ReadAll(readmeReader); err == nil && len(content) > 0 {
			readme = string(content)
		}
	}

	return nil, &GetWorkspaceDetailsResponse{
		WorkspaceDetails: workspaceToDetails(workspace),
		Variables:        variables,
		Readme:           readme,
	}, nil
}

func defaultWorkspaceReadmeFor(organizationName, workspaceName string) string {
	readme := strings.ReplaceAll(defaultWorkspaceReadme, "<<your-terraform-org>>", organizationName)
	return strings.ReplaceAll(readme, "<<your-terraform-workspace>>", workspaceName)
}
