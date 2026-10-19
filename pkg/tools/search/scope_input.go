// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"

	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func resolveMark3LabsScope(ctx context.Context, proposed SearchScope, role SearchScopeRole) (SearchScope, *mcp.CallToolResult) {
	if err := terraformclient.CheckOrganizationAllowlist(ctx, proposed.Organization.Value); err != nil {
		return SearchScope{}, mcp.NewToolResultError(err.Error())
	}
	if !proposed.NeedsInteraction() {
		return proposed, nil
	}
	return SearchScope{}, mcp.NewToolResultText(mark3LabsScopeInputRequest(proposed, role))
}

type mark3LabsScopeInput struct {
	Status        string `json:"status"`
	Message       string `json:"message"`
	ProposedScope struct {
		OrganizationName string `json:"terraform_org_name,omitempty"`
		WorkspaceName    string `json:"workspace_name,omitempty"`
	} `json:"proposed_scope"`
	MissingInputs []string `json:"missing_inputs"`
}

func mark3LabsScopeInputRequest(proposed SearchScope, role SearchScopeRole) string {
	result := mark3LabsScopeInput{
		Status:        "input_required",
		Message:       fmt.Sprintf("Confirm or supply the complete %s organization and workspace, then call again with explicit %s and %s. No scoped API request was made.", role, ScopeOrganizationInput, ScopeWorkspaceInput),
		MissingInputs: []string{},
	}
	result.ProposedScope.OrganizationName = proposed.Organization.Value
	result.ProposedScope.WorkspaceName = proposed.Workspace.Value
	if proposed.Organization.Value == "" {
		result.MissingInputs = append(result.MissingInputs, ScopeOrganizationInput)
	}
	if proposed.Workspace.Value == "" {
		result.MissingInputs = append(result.MissingInputs, ScopeWorkspaceInput)
	}
	payload, _ := json.Marshal(result)
	return string(payload)
}
