// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
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
	Items []*StateVersionSummary `json:"items"`
	*tfe.Pagination
}

// StateVersionDetails holds the complete JSON:API state version payload.
type StateVersionDetails map[string]any

type ListStateVersionsArguments struct {
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	WorkspaceName    string `json:"workspace_name" jsonschema:"The workspace name to list state versions for"`
	Page             int    `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize         int    `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

type GetStateVersionArguments struct {
	StateVersionID string `json:"state_version_id,omitempty" jsonschema:"Optional StateVersion id to fetch exact version"`
	WorkspaceID    string `json:"workspace_id,omitempty" jsonschema:"Optional Workspace id to fetch latest version"`
}

func ListStateVersionsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "list_state_versions",
		Description: "List all the state versions for a given Terraform workspace and organization.",
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

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	stateVersions, err := tfeClient.StateVersions.List(ctx, &tfe.StateVersionListOptions{
		Organization: terraformOrgName,
		Workspace:    workspaceName,
		ListOptions: tfe.ListOptions{
			PageNumber: input.Page,
			PageSize:   input.PageSize,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list workspace state versions: %w", err)
	}
	if len(stateVersions.Items) == 0 {
		return nil, nil, fmt.Errorf("workspace has no state versions to list")
	}

	summaries := make([]*StateVersionSummary, len(stateVersions.Items))
	for i, stateVersion := range stateVersions.Items {
		summaries[i] = &StateVersionSummary{
			ID:               stateVersion.ID,
			CreatedAt:        stateVersion.CreatedAt,
			Serial:           stateVersion.Serial,
			TerraformVersion: stateVersion.TerraformVersion,
			VCSCommitSHA:     stateVersion.VCSCommitSHA,
			VCSCommitURL:     stateVersion.VCSCommitURL,
			StateVersion:     stateVersion.StateVersion,
		}
	}

	return nil, &StateVersionSummaryList{
		Items:      summaries,
		Pagination: stateVersions.Pagination,
	}, nil
}

func GetStateVersionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_state_version",
		Description: "Retrieves a Terraform state version. If state_version_id is provided, retrieves that specific state version. Otherwise, retrieves the latest state version for the specified workspace. One of state_version_id or workspace_id must be provided.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Terraform state version",
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetStateVersionFunc(ctx context.Context, request *mcp.CallToolRequest, input GetStateVersionArguments) (*mcp.CallToolResult, StateVersionDetails, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	stateVersionID := strings.TrimSpace(input.StateVersionID)

	if workspaceID == "" && stateVersionID == "" {
		return nil, nil, fmt.Errorf("One of state_version_id or workspace_id must be provided")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	var sv *tfe.StateVersion
	if stateVersionID != "" {
		sv, err = tfeClient.StateVersions.Read(ctx, stateVersionID)
	} else {
		sv, err = tfeClient.StateVersions.ReadCurrent(ctx, workspaceID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get state version: %w", err)
	}

	var buf bytes.Buffer
	if err := jsonapi.MarshalPayloadWithoutIncluded(&buf, sv); err != nil {
		return nil, nil, fmt.Errorf("failed to marshal state version: %w", err)
	}

	var output StateVersionDetails
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		return nil, nil, fmt.Errorf("failed to decode state version payload: %w", err)
	}

	return nil, output, nil
}
