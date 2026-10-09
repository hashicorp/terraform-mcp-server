// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type StateVersionSummary struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	Serial           int64     `json:"serial"`
	TerraformVersion string    `json:"terraform_version"`
	VCSCommitSHA     string    `json:"vcs_commit_sha"`
	VCSCommitURL     string    `json:"vcs_commit_url"`
	StateVersion     int       `json:"state_version"`
}

type StateVersionSummaryList struct {
	Items []StateVersionSummary `json:"items"`
	PaginationDetails
}

type ListStateVersionsArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	WorkspaceName    string `json:"workspace_name"`
	Pagination
}

func ListStateVersionsTool() *mcp.Tool {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The Terraform organization name",
	}
	properties["workspace_name"] = &jsonschema.Schema{
		Type:        "string",
		Description: "The workspace name to list state versions for",
	}

	return &mcp.Tool{
		Name:        "list_state_versions",
		Description: "List all the state versions for a given Terraform workspace and organization.",
		InputSchema: &jsonschema.Schema{
			Type:                 "object",
			Properties:           properties,
			PropertyOrder:        []string{"terraform_org_name", "workspace_name", "page", "pageSize"},
			Required:             []string{"terraform_org_name", "workspace_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"items": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"id":                {Type: "string"},
							"created_at":        {Type: "string"},
							"serial":            {Type: "integer"},
							"terraform_version": {Type: "string"},
							"vcs_commit_sha":    {Type: "string"},
							"vcs_commit_url":    {Type: "string"},
							"state_version":     {Type: "integer"},
						},
						PropertyOrder:        []string{"id", "created_at", "serial", "terraform_version", "vcs_commit_sha", "vcs_commit_url", "state_version"},
						Required:             []string{"id", "created_at", "serial", "terraform_version", "vcs_commit_sha", "vcs_commit_url", "state_version"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
				"current-page": {Type: "integer"},
				"prev-page":    {Type: "integer"},
				"next-page":    {Type: "integer"},
				"total-count":  {Type: "integer"},
				"total-pages":  {Type: "integer"},
			},
			PropertyOrder:        []string{"items", "current-page", "prev-page", "next-page", "total-count", "total-pages"},
			Required:             []string{"items"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform state versions",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func ListStateVersionsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListStateVersionsArguments) (*mcp.CallToolResult, *StateVersionSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	stateVersions, err := tfeClient.StateVersions.List(ctx, &tfe.StateVersionListOptions{
		Organization: terraformOrgName,
		Workspace:    workspaceName,
		ListOptions:  input.ListOptions(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing state versions for workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}

	var result *mcp.CallToolResult
	if len(stateVersions.Items) == 0 {
		result = &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "workspace has no state versions to list"}},
		}
	}

	summaries := make([]StateVersionSummary, len(stateVersions.Items))
	for i, stateVersion := range stateVersions.Items {
		summaries[i] = StateVersionSummary{
			ID:               stateVersion.ID,
			CreatedAt:        stateVersion.CreatedAt,
			Serial:           stateVersion.Serial,
			TerraformVersion: stateVersion.TerraformVersion,
			VCSCommitSHA:     stateVersion.VCSCommitSHA,
			VCSCommitURL:     stateVersion.VCSCommitURL,
			StateVersion:     stateVersion.StateVersion,
		}
	}

	return result, &StateVersionSummaryList{
		Items:             summaries,
		PaginationDetails: paginationDetails(stateVersions.Pagination),
	}, nil
}
