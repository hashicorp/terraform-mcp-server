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

type StateVersionDetails struct {
	ID                        string                 `json:"id"`
	CreatedAt                 time.Time              `json:"created_at"`
	DownloadURL               string                 `json:"hosted_state_download_url,omitempty"`
	Status                    string                 `json:"status"`
	JSONDownloadURL           string                 `json:"hosted_json_state_download_url,omitempty"`
	Serial                    int64                  `json:"serial"`
	Size                      int64                  `json:"size"`
	VCSCommitSHA              string                 `json:"vcs_commit_sha"`
	VCSCommitURL              string                 `json:"vcs_commit_url"`
	BillableRUMCount          *uint32                `json:"billable_rum_count,omitempty"`
	EncryptedStateDownloadURL *string                `json:"encrypted_state_download_url,omitempty"`
	SanitizedStateDownloadURL *string                `json:"sanitized_state_download_url,omitempty"`
	ResourcesProcessed        bool                   `json:"resources_processed"`
	StateVersion              int                    `json:"state_version"`
	TerraformVersion          string                 `json:"terraform_version"`
	Resources                 []StateVersionResource `json:"resources"`
	RunID                     string                 `json:"run_id,omitempty"`
	OutputIDs                 []string               `json:"output_ids"`
}

type StateVersionResource struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Type     string `json:"type"`
	Module   string `json:"module"`
	Provider string `json:"provider"`
}

type GetStateVersionArguments struct {
	StateVersionID string `json:"state_version_id,omitempty" jsonschema:"Optional ID of a specific state version to fetch (e.g., 'sv-abc123def456')"`
	WorkspaceID    string `json:"workspace_id,omitempty" jsonschema:"Optional ID of a workspace whose latest state version to fetch (e.g., 'ws-abc123def456')"`
}

func GetStateVersionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_state_version",
		Description: "Retrieves a Terraform state version. If state_version_id is provided, retrieves that specific state version. Otherwise, retrieves the latest state version for the specified workspace. One of state_version_id or workspace_id must be provided.",
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"id":                             {Type: "string"},
				"created_at":                     {Type: "string", Format: "date-time"},
				"hosted_state_download_url":      {Type: "string"},
				"status":                         {Type: "string"},
				"hosted_json_state_download_url": {Type: "string"},
				"serial":                         {Type: "integer"},
				"size":                           {Type: "integer"},
				"vcs_commit_sha":                 {Type: "string"},
				"vcs_commit_url":                 {Type: "string"},
				"billable_rum_count":             {Type: "integer"},
				"encrypted_state_download_url":   {Type: "string"},
				"sanitized_state_download_url":   {Type: "string"},
				"resources_processed":            {Type: "boolean"},
				"state_version":                  {Type: "integer"},
				"terraform_version":              {Type: "string"},
				"resources": {
					Type: "array",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"name":     {Type: "string"},
							"count":    {Type: "integer"},
							"type":     {Type: "string"},
							"module":   {Type: "string"},
							"provider": {Type: "string"},
						},
						Required:             []string{"name", "count", "type", "module", "provider"},
						AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
					},
				},
				"run_id":     {Type: "string"},
				"output_ids": {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			},
			Required:             []string{"id", "created_at", "status", "serial", "size", "vcs_commit_sha", "vcs_commit_url", "resources_processed", "state_version", "terraform_version", "resources", "output_ids"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Terraform state version",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetStateVersionFunc(ctx context.Context, request *mcp.CallToolRequest, input GetStateVersionArguments) (*mcp.CallToolResult, *StateVersionDetails, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	stateVersionID := strings.TrimSpace(input.StateVersionID)

	if workspaceID == "" && stateVersionID == "" {
		return nil, nil, fmt.Errorf("One of state_version_id or workspace_id must be provided")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	var sv *tfe.StateVersion
	if stateVersionID != "" {
		sv, err = tfeClient.StateVersions.Read(ctx, stateVersionID)
		if err != nil {
			return nil, nil, fmt.Errorf("reading state version %q: %w", stateVersionID, err)
		}
	} else {
		sv, err = tfeClient.StateVersions.ReadCurrent(ctx, workspaceID)
		if err != nil {
			return nil, nil, fmt.Errorf("reading current state version for workspace %q: %w", workspaceID, err)
		}
	}

	return nil, stateVersionToDetails(sv), nil
}

func stateVersionToDetails(sv *tfe.StateVersion) *StateVersionDetails {
	if sv == nil {
		return nil
	}

	details := &StateVersionDetails{
		ID:                        sv.ID,
		CreatedAt:                 sv.CreatedAt,
		DownloadURL:               sv.DownloadURL,
		Status:                    string(sv.Status),
		JSONDownloadURL:           sv.JSONDownloadURL,
		Serial:                    sv.Serial,
		Size:                      sv.Size,
		VCSCommitSHA:              sv.VCSCommitSHA,
		VCSCommitURL:              sv.VCSCommitURL,
		BillableRUMCount:          sv.BillableRUMCount,
		EncryptedStateDownloadURL: sv.EncryptedStateDownloadURL,
		SanitizedStateDownloadURL: sv.SanitizedStateDownloadURL,
		ResourcesProcessed:        sv.ResourcesProcessed,
		StateVersion:              sv.StateVersion,
		TerraformVersion:          sv.TerraformVersion,
		Resources:                 make([]StateVersionResource, 0, len(sv.Resources)),
		OutputIDs:                 make([]string, 0, len(sv.Outputs)),
	}

	for _, resource := range sv.Resources {
		if resource == nil {
			continue
		}
		details.Resources = append(details.Resources, StateVersionResource{
			Name:     resource.Name,
			Count:    resource.Count,
			Type:     resource.Type,
			Module:   resource.Module,
			Provider: resource.Provider,
		})
	}
	for _, output := range sv.Outputs {
		if output == nil {
			continue
		}
		details.OutputIDs = append(details.OutputIDs, output.ID)
	}
	if sv.Run != nil {
		details.RunID = sv.Run.ID
	}

	return details
}
