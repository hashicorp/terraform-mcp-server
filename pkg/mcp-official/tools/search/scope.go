// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func resolveOfficialScope(_ context.Context, request *mcp.CallToolRequest, proposed searchlogic.SearchScope, role searchlogic.SearchScopeRole) (searchlogic.SearchScope, *mcp.CallToolResult, error) {
	if !proposed.NeedsInteraction() {
		return proposed, nil, nil
	}

	if request == nil || request.Params == nil || len(request.Params.InputResponses) == 0 {
		return searchlogic.SearchScope{}, &mcp.CallToolResult{
			InputRequests: mcp.InputRequestMap{
				searchlogic.ScopeInputRequestID: &mcp.ElicitParams{
					Mode:            "form",
					Message:         proposed.InteractionMessage(role),
					RequestedSchema: officialScopeSchema(proposed),
				},
			},
		}, nil
	}

	response, ok := request.Params.InputResponses[searchlogic.ScopeInputRequestID].(*mcp.ElicitResult)
	if !ok || response == nil {
		return searchlogic.SearchScope{}, nil, fmt.Errorf("invalid scope response")
	}
	switch response.Action {
	case "decline":
		return searchlogic.SearchScope{}, textResult(scopeStoppedText("declined", role)), nil
	case "cancel":
		return searchlogic.SearchScope{}, textResult(scopeStoppedText("cancelled", role)), nil
	case "accept":
		accepted, err := searchlogic.AcceptedSearchScope(response.Content)
		if err != nil {
			return searchlogic.SearchScope{}, nil, fmt.Errorf("invalid scope response: %w", err)
		}
		return accepted, nil, nil
	default:
		return searchlogic.SearchScope{}, nil, fmt.Errorf("invalid scope response action")
	}
}

func officialScopeSchema(proposed searchlogic.SearchScope) *jsonschema.Schema {
	organization := &jsonschema.Schema{
		Type:        "string",
		Title:       "Terraform organization",
		Description: "Complete organization for the requested Search scope.",
	}
	workspace := &jsonschema.Schema{
		Type:        "string",
		Title:       "Terraform workspace",
		Description: "Complete workspace for the requested Search scope.",
	}
	if proposed.Organization.Value != "" {
		organization.Default, _ = json.Marshal(proposed.Organization.Value)
	}
	if proposed.Workspace.Value != "" {
		workspace.Default, _ = json.Marshal(proposed.Workspace.Value)
	}
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			searchlogic.ScopeOrganizationInput: organization,
			searchlogic.ScopeWorkspaceInput:    workspace,
		},
		PropertyOrder:        []string{searchlogic.ScopeOrganizationInput, searchlogic.ScopeWorkspaceInput},
		Required:             []string{searchlogic.ScopeOrganizationInput, searchlogic.ScopeWorkspaceInput},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func scopeStoppedText(status string, role searchlogic.SearchScopeRole) string {
	payload, _ := json.Marshal(map[string]string{
		"status":  status,
		"message": fmt.Sprintf("%s confirmation %s; no scoped API request was made", role, status),
	})
	return string(payload)
}
