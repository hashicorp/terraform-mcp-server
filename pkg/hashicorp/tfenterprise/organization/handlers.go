// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package organization

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"terraform-mcp-server/pkg/hashicorp/tfenterprise/util"
)

func SearchOrganizations(tfeClient *tfe.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("searchOrganizations",
			mcp.WithDescription("This tool searches for all organizations the authenticated user has access to in Terraform cloud/ enterprise. An optional query can be passed, which helps match organizations by name or email."),
			mcp.WithTitleAnnotation("Search for organizations accessible to the authenticated Terraform user"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("query", mcp.Description("Optional: Filter organizations by name or email")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var query string
			if queryVal, ok := request.Params.Arguments["query"]; ok {
				if queryStr, ok := queryVal.(string); ok {
					query = queryStr
				}
			}

			var allOrgs []*tfe.Organization
			pageNumber := 1
			totalCount := 0

			// Iterate through all pages to collect all organizations
			for {
				options := &tfe.OrganizationListOptions{
					ListOptions: tfe.ListOptions{
						PageNumber: pageNumber,
						PageSize:   100,
					},
					Query: query,
				}
				orgList, err := tfeClient.Organizations.List(ctx, options)
				if err != nil {
					return nil, util.LogAndWrapError(logger, "listing organizations", err)
				}
				allOrgs = append(allOrgs, orgList.Items...)
				totalCount = orgList.TotalCount

				if len(allOrgs) >= totalCount || len(orgList.Items) == 0 {
					break
				}
				pageNumber++
			}
			orgSummary, err := renderOrganizationsSummary(allOrgs, query)
			if err != nil {
				return nil, util.LogAndWrapError(logger, fmt.Sprintf("getting organizations(s), none found! query used: %s", query), nil)
			}
			return mcp.NewToolResultText(orgSummary), nil
		}
}

func GetOrganizationDetails(tfeClient *tfe.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("getOrganizationDetails",
			mcp.WithDescription("This tool retrieves details about a specific organization the authenticated user has access to in Terraform cloud/ enterprise. "+
				"If no such organization is found, search for a relevant organization using the `SearchOrganizations` tool."),
			mcp.WithTitleAnnotation("Get details of a specific organization accessible to the authenticated Terraform user"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithString("organizationName", mcp.Required(), mcp.Description("Organization name for which details are required")),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

			organizationName, ok := request.Params.Arguments["organizationName"].(string)
			if !ok || organizationName == "" {
				return nil, util.LogAndWrapError(logger, "organizationName is required and must be a string", nil)
			}

			orgDetails, err := tfeClient.Organizations.Read(ctx, organizationName)
			if err != nil {
				if errors.Is(err, tfe.ErrResourceNotFound) {
					return nil, util.LogAndWrapError(logger, fmt.Sprintf("organizationName %s not found, search for a relevant organization using the `SearchOrganizations` tool with the provided organizationName as query", organizationName), nil)
				}
				return nil, util.LogAndWrapError(logger, fmt.Sprintf("getting organization details for %s", organizationName), err)
			}
			return mcp.NewToolResultText(formatOrganization(orgDetails)), nil
		}
}
