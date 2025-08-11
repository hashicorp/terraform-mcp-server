// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
)

// formatOrganizationsResponse formats the organizations response into a user-friendly format
func formatOrganizationsResponse(response *hcp_terraform.OrganizationResponse) string {
	// Create a simplified response structure
	simplifiedResponse := map[string]interface{}{
		"organizations": make([]map[string]interface{}, len(response.Data)),
		"pagination": map[string]interface{}{
			"current_page": response.Meta.Pagination.CurrentPage,
			"total_pages":  response.Meta.Pagination.TotalPages,
			"total_count":  response.Meta.Pagination.TotalCount,
			"page_size":    response.Meta.Pagination.PageSize,
		},
	}

	// Process each organization
	for i, org := range response.Data {
		simplifiedOrg := map[string]interface{}{
			"id":          org.ID,
			"name":        org.Attributes.Name,
			"email":       org.Attributes.Email,
			"external_id": org.Attributes.ExternalID,
			"created_at":  org.Attributes.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"plan": map[string]interface{}{
				"identifier":    org.Attributes.PlanIdentifier,
				"is_trial":      org.Attributes.PlanIsTrial,
				"is_enterprise": org.Attributes.PlanIsEnterprise,
				"expired":       org.Attributes.PlanExpired,
			},
			"permissions": map[string]interface{}{
				"can_create_workspace":        org.Attributes.Permissions.CanCreateWorkspace,
				"can_manage_users":            org.Attributes.Permissions.CanManageUsers,
				"can_update":                  org.Attributes.Permissions.CanUpdate,
				"can_destroy":                 org.Attributes.Permissions.CanDestroy,
				"can_create_team":             org.Attributes.Permissions.CanCreateTeam,
				"can_manage_subscription":     org.Attributes.Permissions.CanManageSubscription,
				"can_manage_sso":              org.Attributes.Permissions.CanManageSSO,
				"can_create_project":          org.Attributes.Permissions.CanCreateProject,
				"can_manage_public_modules":   org.Attributes.Permissions.CanManagePublicModules,
				"can_manage_public_providers": org.Attributes.Permissions.CanManagePublicProviders,
			},
			"settings": map[string]interface{}{
				"collaborator_auth_policy":      org.Attributes.CollaboratorAuthPolicy,
				"cost_estimation_enabled":       org.Attributes.CostEstimationEnabled,
				"two_factor_conformant":         org.Attributes.TwoFactorConformant,
				"assessments_enforced":          org.Attributes.AssessmentsEnforced,
				"default_execution_mode":        org.Attributes.DefaultExecutionMode,
				"fair_run_queuing_enabled":      org.Attributes.FairRunQueuingEnabled,
				"saml_enabled":                  org.Attributes.SAMLEnabled,
				"allow_force_delete_workspaces": org.Attributes.AllowForceDeleteWorkspaces,
			},
		}

		// Add expiration date if present
		if org.Attributes.PlanExpiresAt != nil {
			planMap := simplifiedOrg["plan"].(map[string]interface{})
			planMap["expires_at"] = org.Attributes.PlanExpiresAt.Format("2006-01-02T15:04:05Z")
		}

		simplifiedResponse["organizations"].([]map[string]interface{})[i] = simplifiedOrg
	}

	// Convert to JSON
	jsonBytes, err := json.MarshalIndent(simplifiedResponse, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting response: %v", err)
	}

	return string(jsonBytes)
}

// formatErrorResponse formats error responses consistently
func formatErrorResponse(err error) string {
	errorResponse := map[string]interface{}{
		"error":   true,
		"message": err.Error(),
	}

	// Add additional context for HCP Terraform errors
	if hcpErr, ok := err.(*hcp_terraform.HCPTerraformError); ok {
		errorResponse["error_type"] = string(hcpErr.Type)
		if hcpErr.StatusCode > 0 {
			errorResponse["status_code"] = hcpErr.StatusCode
		}
		if hcpErr.RetryAfter != nil {
			errorResponse["retry_after_seconds"] = int(hcpErr.RetryAfter.Seconds())
		}
	}

	jsonBytes, err := json.MarshalIndent(errorResponse, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": true, "message": "Error formatting error response: %v"}`, err)
	}

	return string(jsonBytes)
}
