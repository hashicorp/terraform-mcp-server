// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ListVariableSets creates a tool to list variable sets.
func ListVariableSets(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_variable_sets",
			mcp.WithDescription("List all variable sets in an organization. Returns all if query is empty."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("query", mcp.Description("Optional filter query for variable set names")),
			utils.WithPagination(),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			query := request.GetString("query", "")

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			pagination, err := utils.OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			varSets, err := tfeClient.VariableSets.List(ctx, orgName, &tfe.VariableSetListOptions{
				Query: query,
				ListOptions: tfe.ListOptions{
					PageNumber: pagination.Page,
					PageSize:   pagination.PageSize,
				},
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "listing variable sets", err)
			}

			buf := bytes.NewBuffer(nil)
			err = jsonapi.MarshalPayloadWithoutIncluded(buf, varSets.Items)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "marshalling variable set list result", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(buf.String()),
				},
			}, nil
		},
	}
}

// CreateVariableSet creates a tool to create a variable set.
func CreateVariableSet(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_variable_set",
			mcp.WithDescription("Create a new variable set in an organization."),
			mcp.WithString("terraform_org_name", mcp.Required(), mcp.Description("Organization name")),
			mcp.WithString("name", mcp.Required(), mcp.Description("Variable set name")),
			mcp.WithString("description", mcp.Description("Variable set description")),
			mcp.WithBoolean("global", mcp.Description("Whether variable set is global: true or false"), mcp.DefaultBool(false)),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := request.RequireString("terraform_org_name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraform_org_name is required", err)
			}
			name, err := request.RequireString("name")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "name is required", err)
			}
			description := request.GetString("description", "")
			global := request.GetBool("global", false)

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			varSet, err := tfeClient.VariableSets.Create(ctx, orgName, &tfe.VariableSetCreateOptions{
				Name:        &name,
				Description: &description,
				Global:      &global,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "creating variable set", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Successfully created variable set %s with ID %s", varSet.Name, varSet.ID)),
				},
			}, nil
		},
	}
}

// CreateVariableInVariableSet creates a tool to create a variable in a variable set.
func CreateVariableInVariableSet(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_variable_in_variable_set",
			mcp.WithDescription("Create a new variable in a variable set."),
			mcp.WithString("variable_set_id", mcp.Required(), mcp.Description("Variable set ID")),
			mcp.WithString("key", mcp.Required(), mcp.Description("Variable key/name")),
			mcp.WithString("value", mcp.Required(), mcp.Description("Variable value")),
			mcp.WithString("description", mcp.Description("Variable description")),
			mcp.WithString("category", mcp.Description("Variable category: terraform or env"), mcp.Enum("terraform", "env"), mcp.DefaultString("terraform")),
			mcp.WithBoolean("hcl", mcp.Description("Whether variable is HCL: true or false"), mcp.DefaultBool(false)),
			mcp.WithBoolean("sensitive", mcp.Description("Whether variable is sensitive: true or false"), mcp.DefaultBool(false)),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			varSetID, err := request.RequireString("variable_set_id")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "variable_set_id is required", err)
			}
			key, err := request.RequireString("key")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "key is required", err)
			}
			value, err := request.RequireString("value")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "value is required", err)
			}

			category := tfe.CategoryTerraform
			if request.GetString("category", "") == "env" {
				category = tfe.CategoryEnv
			}

			hcl := request.GetBool("hcl", false)
			sensitive := request.GetBool("sensitive", false)
			description := request.GetString("description", "")

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			variable, err := tfeClient.VariableSetVariables.Create(ctx, varSetID, &tfe.VariableSetVariableCreateOptions{
				Key:         &key,
				Value:       &value,
				Category:    &category,
				Sensitive:   &sensitive,
				HCL:         &hcl,
				Description: &description,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "creating variable in variable set", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Successfully created variable %s with ID %s in variable set %s", variable.Key, variable.ID, varSetID)),
				},
			}, nil
		},
	}
}

// DeleteVariableInVariableSet creates a tool to delete a variable from a variable set.
func DeleteVariableInVariableSet(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("delete_variable_in_variable_set",
			mcp.WithDescription("Delete a variable in a variable set."),
			mcp.WithString("variable_set_id", mcp.Required(), mcp.Description("Variable set ID")),
			mcp.WithString("variable_id", mcp.Required(), mcp.Description("Variable ID to delete")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			varSetID, err := request.RequireString("variable_set_id")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "variable_set_id is required", err)
			}
			variableID, err := request.RequireString("variable_id")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "variable_id is required", err)
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			err = tfeClient.VariableSetVariables.Delete(ctx, varSetID, variableID)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "deleting variable from variable set", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Successfully deleted variable %s from variable set %s", variableID, varSetID)),
				},
			}, nil
		},
	}
}

// AttachVariableSetToWorkspaces creates a tool to attach a variable set to workspaces.
func AttachVariableSetToWorkspaces(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("attach_variable_set_to_workspaces",
			mcp.WithDescription("Attach a variable set to one or more workspaces."),
			mcp.WithString("variable_set_id", mcp.Required(), mcp.Description("Variable set ID")),
			mcp.WithString("workspace_ids", mcp.Required(), mcp.Description("Comma-separated list of workspace IDs")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			varSetID, err := request.RequireString("variable_set_id")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "variable_set_id is required", err)
			}
			workspaceIDsStr, err := request.RequireString("workspace_ids")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_ids is required", err)
			}
			workspaceIDsList := strings.Split(workspaceIDsStr, ",")

			var workspaces []*tfe.Workspace
			for _, id := range workspaceIDsList {
				workspaces = append(workspaces, &tfe.Workspace{ID: strings.TrimSpace(id)})
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			err = tfeClient.VariableSets.ApplyToWorkspaces(ctx, varSetID, &tfe.VariableSetApplyToWorkspacesOptions{
				Workspaces: workspaces,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "attaching variable set to workspaces", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Successfully attached variable set %s to %d workspaces", varSetID, len(workspaces))),
				},
			}, nil
		},
	}
}

// DetachVariableSetFromWorkspaces creates a tool to detach a variable set from workspaces.
func DetachVariableSetFromWorkspaces(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("detach_variable_set_from_workspaces",
			mcp.WithDescription("Detach a variable set from one or more workspaces."),
			mcp.WithString("variable_set_id", mcp.Required(), mcp.Description("Variable set ID")),
			mcp.WithString("workspace_ids", mcp.Required(), mcp.Description("Comma-separated list of workspace IDs")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			varSetID, err := request.RequireString("variable_set_id")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "variable_set_id is required", err)
			}
			workspaceIDsStr, err := request.RequireString("workspace_ids")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "workspace_ids is required", err)
			}
			workspaceIDsList := strings.Split(workspaceIDsStr, ",")

			var workspaces []*tfe.Workspace
			for _, id := range workspaceIDsList {
				workspaces = append(workspaces, &tfe.Workspace{ID: strings.TrimSpace(id)})
			}

			tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
			}

			err = tfeClient.VariableSets.RemoveFromWorkspaces(ctx, varSetID, &tfe.VariableSetRemoveFromWorkspacesOptions{
				Workspaces: workspaces,
			})
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "detaching variable set from workspaces", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("Successfully detached variable set %s from %d workspaces", varSetID, len(workspaces))),
				},
			}, nil
		},
	}
}
