// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// ListTerraformOrgs creates a tool to get terraform organizations.
func ListTerraformOrgs(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("list_terraform_orgs",
			mcp.WithDescription(`Fetches a list of all Terraform organizations.`),
			mcp.WithTitleAnnotation("List all Terraform organizations"),
			mcp.WithReadOnlyHintAnnotation(true),
			utils.WithPagination(),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listTerraformOrgsHandler(ctx, req, logger)
		},
	}
}

func listTerraformOrgsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return nil, utils.LogAndReturnError(logger, "TFE client is not available - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", nil)
	}

	pagination, err := utils.OptionalPaginationParams(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	orgs, err := tfeClient.Organizations.List(ctx, &tfe.OrganizationListOptions{
		ListOptions: tfe.ListOptions{
			PageNumber: pagination.Page,
			PageSize:   pagination.PageSize,
		},
	})

	if err != nil {
		logger.WithError(err).Error("failed to list Terraform organizations")
		return nil, utils.LogAndReturnError(logger, "failed to list Terraform organizations", err)
	}

	orgNames := make([]string, 0, len(orgs.Items))
	for _, org := range orgs.Items {
		orgNames = append(orgNames, org.Name)
	}

	orgsJSON, err := json.Marshal(orgNames)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "failed to marshal organization names", err)
	}

	return mcp.NewToolResultText(string(orgsJSON)), nil
}
