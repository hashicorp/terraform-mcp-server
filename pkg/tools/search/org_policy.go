// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
)

// readQueryRunWorkspaceWithOrgPolicy resolves backend-owned QueryRun provenance
// and checks any request-scoped organization allowlist policy.
func readQueryRunWorkspaceWithOrgPolicy(policyCtx, requestCtx context.Context, c *tfe.Client, queryRunID string, run *tfe.QueryRun) (*tfe.Workspace, error) {
	if run == nil || run.ID != queryRunID || run.Workspace == nil || strings.TrimSpace(run.Workspace.ID) == "" {
		return nil, searchDiagnosticError(queryOwnershipUnverifiedCode)
	}

	workspaceID := run.Workspace.ID
	workspace, err := c.Workspaces.ReadByID(requestCtx, workspaceID)
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, context.Cause(requestCtx)
		}
		return nil, searchDiagnosticError(queryOwnershipUnverifiedCode)
	}
	if workspace == nil || workspace.ID != workspaceID || workspace.Organization == nil || strings.TrimSpace(workspace.Organization.Name) == "" {
		return nil, searchDiagnosticError(queryOwnershipUnverifiedCode)
	}
	if err := terraformclient.CheckOrganizationAllowlist(policyCtx, workspace.Organization.Name); err != nil {
		return nil, err
	}
	return workspace, nil
}
