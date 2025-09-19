// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ListWorkspaceVariables creates a tool to list workspace variables.
func ListWorkspaceVariables(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_workspace_variables",
			mcp.WithDescription("List all variables in a Terraform workspace. Returns all variables if query is empty."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
			utils.WithPagination(),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			pagination, err := utils.OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			vars, err := tfeClient.Variables.List(ctx, workspace.ID, &tfe.VariableListOptions{
				ListOptions: tfe.ListOptions{
					PageNumber: pagination.Page,
					PageSize:   pagination.PageSize,
				},
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "listing variables", err)
			}

			buf := bytes.NewBuffer(nil)
			err = jsonapi.MarshalPayload(buf, vars.Items)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "marshalling variables to JSON", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(buf.String()),
				},
			}, nil
		},
	}
}

// CreateWorkspaceVariable creates a tool to create a workspace variable.
func CreateWorkspaceVariable(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_workspace_variable",
			mcp.WithDescription("Create a new variable in a Terraform workspace."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
			mcp.WithString("key", mcp.Required(), mcp.Description("Variable key/name")),
			mcp.WithString("value", mcp.Required(), mcp.Description("Variable value")),
			mcp.WithString("description", mcp.Description("Variable description"), mcp.DefaultString("")),
			mcp.WithString("category",
				mcp.Description("Variable category: terraform or env"),
				mcp.Enum("terraform", "env"),
				mcp.DefaultString("env"),
			),
			mcp.WithBoolean("sensitive", mcp.Description("Whether variable is sensitive: true or false"), mcp.DefaultBool(false)),
			mcp.WithBoolean("hcl", mcp.Description("Whether variable is HCL: true or false"), mcp.DefaultBool(false)),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}
			key, err := request.RequireString("key")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "key is required", err)
			}
			value, err := request.RequireString("value")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "value is required", err)
			}

			category := tfe.CategoryEnv
			if request.GetString("category", "") == "terraform" {
				category = tfe.CategoryTerraform
			}

			sensitive := request.GetBool("sensitive", false)
			hcl := request.GetBool("hcl", false)
			description := request.GetString("description", "")

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			variable, err := tfeClient.Variables.Create(ctx, workspace.ID, tfe.VariableCreateOptions{
				Key:         &key,
				Value:       &value,
				Category:    &category,
				Sensitive:   &sensitive,
				HCL:         &hcl,
				Description: &description,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "creating variable", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Created variable %s with ID %s", variable.Key, variable.ID)),
				},
			}, nil
		},
	}
}

// UpdateWorkspaceVariable creates a tool to update a workspace variable.
func UpdateWorkspaceVariable(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("update_workspace_variable",
			mcp.WithDescription("Update an existing variable in a Terraform workspace."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Workspace name")),
			mcp.WithString("variable_id", mcp.Required(), mcp.Description("Variable ID to update")),
			mcp.WithString("key", mcp.Required(), mcp.Description("Variable key/name")),
			mcp.WithString("value", mcp.Required(), mcp.Description("Variable value")),
			mcp.WithBoolean("sensitive", mcp.Description("Whether variable is sensitive: true or false"), mcp.DefaultBool(false)),
			mcp.WithBoolean("hcl", mcp.Description("Whether variable is HCL: true or false"), mcp.DefaultBool(false)),
			mcp.WithString("description", mcp.Description("Variable description")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			workspaceName, err := request.RequireString("workspace_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_name is required", err)
			}
			variableID, err := request.RequireString("variable_id")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "variable_id is required", err)
			}
			key, err := request.RequireString("key")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "key is required", err)
			}
			value, err := request.RequireString("value")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "value is required", err)
			}

			options := tfe.VariableUpdateOptions{
				Key:   &key,
				Value: &value,
			}
			if sensitiveStr := request.GetString("sensitive", ""); sensitiveStr != "" {
				sensitive := sensitiveStr == "true"
				options.Sensitive = &sensitive
			}
			if hclStr := request.GetString("hcl", ""); hclStr != "" {
				hcl := hclStr == "true"
				options.HCL = &hcl
			}
			if description := request.GetString("description", ""); description != "" {
				options.Description = &description
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading workspace", err)
			}

			variable, err := tfeClient.Variables.Update(ctx, workspace.ID, variableID, options)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "updating variable", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Updated variable %s with ID %s", variable.Key, variable.ID)),
				},
			}, nil
		},
	}
}
