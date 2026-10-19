// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func resolveMark3LabsScope(ctx context.Context, mcpServer *server.MCPServer, proposed SearchScope, role SearchScopeRole) (SearchScope, *mcp.CallToolResult) {
	if err := terraformclient.CheckOrganizationAllowlist(ctx, proposed.Organization.Value); err != nil {
		return SearchScope{}, mcp.NewToolResultError(err.Error())
	}
	if !proposed.NeedsInteraction() {
		return proposed, nil
	}

	if mcpServer == nil {
		return SearchScope{}, mcp.NewToolResultError(scopeInteractionUnavailableMessage())
	}
	result, err := mcpServer.RequestElicitation(ctx, mcp.ElicitationRequest{
		Request: mcp.Request{Method: string(mcp.MethodElicitationCreate)},
		Params: mcp.ElicitationParams{
			Mode:            mcp.ElicitationModeForm,
			Message:         proposed.InteractionMessage(role),
			RequestedSchema: mark3LabsScopeSchema(proposed),
		},
	})
	if err != nil {
		return SearchScope{}, mcp.NewToolResultError(scopeInteractionUnavailableMessage())
	}

	switch result.Action {
	case mcp.ElicitationResponseActionDecline:
		return SearchScope{}, mcp.NewToolResultText(scopeInteractionStoppedText("declined", role))
	case mcp.ElicitationResponseActionCancel:
		return SearchScope{}, mcp.NewToolResultText(scopeInteractionStoppedText("cancelled", role))
	case mcp.ElicitationResponseActionAccept:
		content, ok := result.Content.(map[string]any)
		if !ok {
			return SearchScope{}, mcp.NewToolResultError("invalid scope response: expected a complete organization and workspace")
		}
		accepted, err := AcceptedSearchScope(content)
		if err != nil {
			return SearchScope{}, mcp.NewToolResultError(fmt.Sprintf("invalid scope response: %v", err))
		}
		if err := terraformclient.CheckOrganizationAllowlist(ctx, accepted.Organization.Value); err != nil {
			return SearchScope{}, mcp.NewToolResultError(err.Error())
		}
		return accepted, nil
	default:
		return SearchScope{}, mcp.NewToolResultError("invalid scope response action")
	}
}

func mark3LabsScopeSchema(proposed SearchScope) map[string]any {
	properties := map[string]any{
		ScopeOrganizationInput: map[string]any{
			"type":        "string",
			"title":       "Terraform organization",
			"description": "Complete organization for the requested Search scope.",
		},
		ScopeWorkspaceInput: map[string]any{
			"type":        "string",
			"title":       "Terraform workspace",
			"description": "Complete workspace for the requested Search scope.",
		},
	}
	if proposed.Organization.Value != "" {
		properties[ScopeOrganizationInput].(map[string]any)["default"] = proposed.Organization.Value
	}
	if proposed.Workspace.Value != "" {
		properties[ScopeWorkspaceInput].(map[string]any)["default"] = proposed.Workspace.Value
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []string{ScopeOrganizationInput, ScopeWorkspaceInput},
		"additionalProperties": false,
	}
}

func scopeInteractionUnavailableMessage() string {
	return "this client cannot collect the required Search scope; confirm it with the user and call again with explicit terraform_org_name and workspace_name"
}

func scopeInteractionStoppedText(status string, role SearchScopeRole) string {
	payload, _ := json.Marshal(map[string]string{
		"status":  status,
		"message": fmt.Sprintf("%s confirmation %s; no scoped API request was made", role, status),
	})
	return string(payload)
}
