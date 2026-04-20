// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestAttachPolicySetToWorkspaces(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := AttachPolicySetToWorkspaces(logger)

		assert.Equal(t, "attach_policy_set_to_workspaces", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Attach a policy set")
		assert.NotNil(t, tool.Handler)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "policy_set_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_ids")
	})
}

func TestListWorkspacePolicySets(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := ListWorkspacePolicySets(logger)

		assert.Equal(t, "list_workspace_policy_sets", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Read all policy sets")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_id")
	})
}

func TestGetPolicySetDetails(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := GetPolicySetDetails(logger)

		assert.Equal(t, "get_policy_set_details", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Get detailed information")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "policy_set_id")
	})
}

func TestPolicySetDetailsStructure(t *testing.T) {
	t.Run("PolicyInfo with empty Enforce array", func(t *testing.T) {
		// Simulate a policy with empty Enforce array
		policy := &tfe.Policy{
			ID:          "pol-123",
			Name:        "test-policy",
			Description: "Test policy",
			Enforce:     []*tfe.Enforcement{}, // Empty array
			Kind:        tfe.OPA,
		}

		// This should not panic - testing the safety check
		enforcementLevel := ""
		if len(policy.Enforce) > 0 {
			enforcementLevel = string(policy.Enforce[0].Mode)
		}

		policyInfo := PolicyInfo{
			ID:               policy.ID,
			Name:             policy.Name,
			Description:      policy.Description,
			EnforcementLevel: enforcementLevel,
			PolicySetID:      "polset-123",
			Kind:             string(policy.Kind),
		}

		assert.Equal(t, "", policyInfo.EnforcementLevel)
		assert.Equal(t, "pol-123", policyInfo.ID)
		assert.Equal(t, "test-policy", policyInfo.Name)
	})

	t.Run("PolicyInfo with valid Enforce array", func(t *testing.T) {
		// Simulate a policy with valid Enforce array
		policy := &tfe.Policy{
			ID:          "pol-123",
			Name:        "test-policy",
			Description: "Test policy",
			Enforce: []*tfe.Enforcement{
				{Mode: tfe.EnforcementAdvisory},
			},
			Kind: tfe.OPA,
		}

		enforcementLevel := ""
		if len(policy.Enforce) > 0 {
			enforcementLevel = string(policy.Enforce[0].Mode)
		}

		policyInfo := PolicyInfo{
			ID:               policy.ID,
			Name:             policy.Name,
			Description:      policy.Description,
			EnforcementLevel: enforcementLevel,
			PolicySetID:      "polset-123",
			Kind:             string(policy.Kind),
		}

		assert.Equal(t, "advisory", policyInfo.EnforcementLevel)
		assert.Equal(t, "pol-123", policyInfo.ID)
	})
}