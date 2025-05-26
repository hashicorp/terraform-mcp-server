// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package organization

import (
	"fmt"
	"github.com/hashicorp/go-tfe"
	"strings"
	"time"
)

// renderOrganizationsSummary returns a formatted summary of TFE organizations, including key details. Returns an error if none are found.
func renderOrganizationsSummary(organizations []*tfe.Organization, query string) (string, error) {
	if len(organizations) == 0 {
		return "", fmt.Errorf("no organizations found")
	}

	var builder strings.Builder
	if query != "" {
		builder.WriteString(fmt.Sprintf("Available Terraform Organizations for query %s:\n\nEach result includes:\n", query))
	} else {
		builder.WriteString("Available Terraform Organizations:\n\nEach result includes:\n")
	}
	builder.WriteString("- Name: The organization's name (used in API calls)\n")
	builder.WriteString("- Email: The associated email address\n")
	builder.WriteString("- CreatedAt: When the organization was created\n")
	builder.WriteString("- DefaultExecutionMode: The mode used for Terraform runs (e.g., remote, local)\n")
	builder.WriteString("- SAML Enabled: Whether SAML authentication is enabled\n")
	builder.WriteString("- 2FA Conformant: Whether users must use two-factor authentication\n")
	builder.WriteString("\n\n---\n\n")

	for _, org := range organizations {
		builder.WriteString(fmt.Sprintf("- Name: %s\n", org.Name))
		builder.WriteString(fmt.Sprintf("- Email: %s\n", org.Email))
		builder.WriteString(fmt.Sprintf("- CreatedAt: %s\n", org.CreatedAt.Format(time.RFC3339)))
		builder.WriteString(fmt.Sprintf("- DefaultExecutionMode: %s\n", org.DefaultExecutionMode))
		builder.WriteString(fmt.Sprintf("- SAML Enabled: %t\n", org.SAMLEnabled))
		builder.WriteString(fmt.Sprintf("- 2FA Conformant: %t\n", org.TwoFactorConformant))
		builder.WriteString("---\n\n")
	}

	return builder.String(), nil
}

// formatOrganization returns a formatted summary of a TFE Organization.
func formatOrganization(org *tfe.Organization) string {
	if org == nil {
		return "Organization: <nil>"
	}

	return fmt.Sprintf(`Organization Details
---------------------
Name:                   %s
Email:                  %s
External ID:            %s
Created At:             %s
Trial Expires At:       %s
Default Execution Mode: %s

Security & Authentication
-------------------------
Assessments Enforced:     %t
Cost Estimation Enabled:  %t
Is Unified:               %t
SAML Enabled:             %t
Collaborator Auth Policy: %s
Two-Factor Conformant:    %t

Session Management
------------------
Session Remember (mins): %d
Session Timeout (mins):  %d

Advanced Features
-----------------
Send Passing Statuses for Untriggered Plans: %t
Speculative Plan Management Enabled:         %t
Aggregated Commit Status Enabled:            %t
Allow Force Delete Workspaces:               %t
Remaining Testable Count:                    %d

Permissions
-----------%s`,
		org.Name,
		org.Email,
		org.ExternalID,
		formatTime(org.CreatedAt),
		formatTime(org.TrialExpiresAt),
		org.DefaultExecutionMode,
		org.AssessmentsEnforced,
		org.CostEstimationEnabled,
		org.IsUnified,
		org.SAMLEnabled,
		org.CollaboratorAuthPolicy,
		org.TwoFactorConformant,
		org.SessionRemember,
		org.SessionTimeout,
		org.SendPassingStatusesForUntriggeredSpeculativePlans,
		org.SpeculativePlanManagementEnabled,
		org.AggregatedCommitStatusEnabled,
		org.AllowForceDeleteWorkspaces,
		org.RemainingTestableCount,
		formatOrganizationPermissions(org.Permissions),
	)
}

// formatOrganizationPermissions formats the organization permissions for display.
func formatOrganizationPermissions(p *tfe.OrganizationPermissions) string {
	if p == nil {
		return "<nil>"
	}

	return fmt.Sprintf(`
Can Create Team:                %t
Can Create Workspace:           %t
Can Create Workspace Migration: %t
Can Deploy No-Code Modules:     %t
Can Destroy:                    %t
Can Manage No-Code Modules:     %t
Can Manage Run Tasks:           %t
Can Traverse:                   %t
Can Update:                     %t
Can Update API Token:           %t
Can Update OAuth:               %t
Can Update Sentinel:            %t`,
		p.CanCreateTeam,
		p.CanCreateWorkspace,
		p.CanCreateWorkspaceMigration,
		p.CanDeployNoCodeModules,
		p.CanDestroy,
		p.CanManageNoCodeModules,
		p.CanManageRunTasks,
		p.CanTraverse,
		p.CanUpdate,
		p.CanUpdateAPIToken,
		p.CanUpdateOAuth,
		p.CanUpdateSentinel,
	)
}

// formatTime safely formats time.Time, returning "N/A" if it's zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	return t.Format(time.RFC3339)
}
