// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func SearchPolicies(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("searchPolicies",
			mcp.WithDescription(`Searches for Terraform policies based on a query string. This tool returns a list of matching policies, which can be used to retrieve detailed policy information using the 'policyDetails' tool. 
			You MUST call this function before 'policyDetails' to obtain a valid terraformPolicyID.
			When selecting the best match, consider: - Name similarity to the query - Title relevance - Verification status (verified) - Download counts (popularity) Return the selected policyID and explain your choice. 
			If there are multiple good matches, mention this but proceed with the most relevant one. If no policies were found, reattempt the search with a new policyQuery.`),
			mcp.WithTitleAnnotation("Search and match Terraform policies based on name and relevance"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("policyQuery",
				mcp.Required(),
				mcp.Description("The query to search for Terraform modules."),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var terraformPolicies client.TerraformPolicyList
			pq, err := request.RequireString("policyQuery")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "policyQuery is required", err)
			}
			if pq == "" {
				return nil, utils.LogAndReturnError(logger, "policyQuery cannot be empty", nil)
			}

			// static list of 100 is fine for now
			policyResp, err := client.SendRegistryCall(registryClient, "GET", "policies?page%5Bsize%5D=100&include=latest-version", logger, "v2")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "Failed to fetch policies: registry API did not return a successful response", err)
			}

			err = json.Unmarshal(policyResp, &terraformPolicies)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "Unmarshalling policy list", err)
			}

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("Matching Terraform Policies for query: %s\n\n", pq))
			builder.WriteString("Each result includes:\n- terraformPolicyID: Unique identifier to be used with policyDetails tool\n- Name: Policy name\n- Title: Policy description\n- Downloads: Policy downloads\n---\n\n")

			contentAvailable := false
			for _, policy := range terraformPolicies.Data {
				cs, err := utils.ContainsSlug(strings.ToLower(policy.Attributes.Title), strings.ToLower(pq))
				cs_pn, err_pn := utils.ContainsSlug(strings.ToLower(policy.Attributes.Name), strings.ToLower(pq))
				if (cs || cs_pn) && err == nil && err_pn == nil {
					contentAvailable = true
					ID := strings.ReplaceAll(policy.Relationships.LatestVersion.Links.Related, "/v2/", "")
					builder.WriteString(fmt.Sprintf(
						"- terraformPolicyID: %s\n- Name: %s\n- Title: %s\n- Downloads: %d\n---\n",
						ID,
						policy.Attributes.Name,
						policy.Attributes.Title,
						policy.Attributes.Downloads,
					))
				}
			}

			policyData := builder.String()
			if !contentAvailable {
				errMessage := fmt.Sprintf("No policies found matching the query: %s. Try a different policyQuery.", pq)
				return nil, utils.LogAndReturnError(logger, errMessage, nil)
			}

			return mcp.NewToolResultText(policyData), nil
		}
}
