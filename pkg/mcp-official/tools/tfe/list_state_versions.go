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
	*tfe.Pagination
}

type ListStateVersionsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The workspace name to list state versions for"`
	Page             int    `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize         int    `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

func ListStateVersionsTool() *mcp.Tool {
	input, err := jsonschema.For[ListStateVersionsArguments](nil)
	if err != nil {
		panic(err)
	}
	input.Properties["page"].Minimum = jsonschema.Ptr(1.0)
	input.Properties["pageSize"].Minimum = jsonschema.Ptr(1.0)
	input.Properties["pageSize"].Maximum = jsonschema.Ptr(100.0)

	output, err := jsonschema.For[StateVersionSummaryList](nil)
	if err != nil {
		panic(err)
	}
	items := output.Properties["items"]
	items.Types, items.Type = nil, "array"

	return &mcp.Tool{
		Name:         "list_state_versions",
		Description:  "List all the state versions for a given Terraform workspace and organization.",
		InputSchema:  input,
		OutputSchema: output,
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform state versions",
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func ListStateVersionsFunc(ctx context.Context, request *mcp.CallToolRequest, input ListStateVersionsArguments) (*mcp.CallToolResult, *StateVersionSummaryList, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	workspaceName := strings.TrimSpace(input.WorkspaceName)

	listOptions := tfe.ListOptions{PageNumber: input.Page, PageSize: input.PageSize}
	if listOptions.PageNumber == 0 {
		listOptions.PageNumber = 1
	}
	if listOptions.PageSize == 0 {
		listOptions.PageSize = 30
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	stateVersions, err := tfeClient.StateVersions.List(ctx, &tfe.StateVersionListOptions{
		Organization: terraformOrgName,
		Workspace:    workspaceName,
		ListOptions:  listOptions,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list workspace state versions: %w", err)
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
		Items:      summaries,
		Pagination: stateVersions.Pagination,
	}, nil
}
