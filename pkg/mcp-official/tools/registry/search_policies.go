// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type SearchPoliciesArguments struct {
	PolicyQuery string `json:"policy_query"`
}

func SearchPoliciesTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "search_policies",
		Description: `Searches for Terraform policies based on a query string.
This tool returns a list of matching policies, which can be used to retrieve detailed policy information using the 'get_policy_details' tool.
You MUST call this function before 'get_policy_details' to obtain a valid terraform_policy_id.
When selecting the best match, consider the following:
	- Name similarity to the query
	- Title relevance
	- Verification status (verified)
	- Download counts (popularity)
Return the selected policyID and explain your choice. If there are multiple good matches, mention this but proceed with the most relevant one.
If no policies were found, reattempt the search with a new policy_query.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search and match Terraform policies based on name and relevance",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"policy_query": {
					Type:        "string",
					Description: "The query to search for Terraform policies",
				},
			},
			Required: []string{"policy_query"},
		},
	}
}

func SearchPoliciesFunc(ctx context.Context, request *mcp.CallToolRequest, input SearchPoliciesArguments) (*mcp.CallToolResult, any, error) {
	logger := log.StandardLogger()

	policyQuery := strings.TrimSpace(input.PolicyQuery)
	if policyQuery == "" {
		return nil, nil, fmt.Errorf("policy_query cannot be empty")
	}
	policyQuery = strings.ToLower(policyQuery)

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	uri := (&url.URL{
		Path: "policies",
		RawQuery: url.Values{
			"page[size]": {"100"},
			"include":    {"latest-version"},
		}.Encode(),
	}).String()

	policyResponse, err := registryapi.SendRegistryCall(ctx, httpClient, http.MethodGet, uri, logger, "v2")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch policies from registry: %w", err)
	}

	var terraformPolicies registryapi.TerraformPolicyList
	if err := json.Unmarshal(policyResponse, &terraformPolicies); err != nil {
		return nil, nil, fmt.Errorf("failed to parse policy list: %w", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Matching Terraform Policies for query: %s\n\n", policyQuery))
	builder.WriteString("Each result includes:\n- terraform_policy_id: Unique identifier to be used with get_policy_details tool\n- Name: Policy name\n- Title: Policy description\n- Downloads: Policy downloads\n---\n\n")

	contentAvailable := false
	for _, policy := range terraformPolicies.Data {
		titleMatches, titleErr := utils.ContainsSlug(strings.ToLower(policy.Attributes.Title), policyQuery)
		nameMatches, nameErr := utils.ContainsSlug(strings.ToLower(policy.Attributes.Name), policyQuery)
		if (titleMatches || nameMatches) && titleErr == nil && nameErr == nil {
			contentAvailable = true
			policyID := strings.ReplaceAll(policy.Relationships.LatestVersion.Links.Related, "/v2/", "")
			builder.WriteString(fmt.Sprintf(
				"- terraform_policy_id: %s\n- Name: %s\n- Title: %s\n- Downloads: %d\n---\n",
				policyID,
				policy.Attributes.Name,
				policy.Attributes.Title,
				policy.Attributes.Downloads,
			))
		}
	}

	if !contentAvailable {
		return nil, nil, fmt.Errorf("no policies found matching query: %s - try a different search term", policyQuery)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: builder.String()}},
	}, nil, nil
}
