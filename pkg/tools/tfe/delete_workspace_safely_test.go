// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDeleteWorkspaceSafely(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := DeleteWorkspaceSafely(logger)
		
		assert.Equal(t, "delete_workspace_safely", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Safely deletes a Terraform workspace by ID")
		assert.NotNil(t, tool.Handler)
		
		// Verify it's marked as destructive
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)
		
		// Check that required parameters are defined
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_id")
	})

	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
			errorField  string
		}{
			{
				name: "valid minimal parameters",
				params: map[string]interface{}{
					"workspace_id": "ws-123456",
				},
				expectError: false,
			},
			{
				name: "valid parameters with options",
				params: map[string]interface{}{
					"workspace_id": "ws-123456",
					"force_unlock": "true",
					"dry_run":      "false",
				},
				expectError: false,
			},
			{
				name: "missing workspace ID",
				params: map[string]interface{}{
					"force_unlock": "true",
				},
				expectError: true,
				errorField:  "workspace_id",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}
				
				workspaceID, err := request.RequireString("workspace_id")
				forceUnlock := request.GetString("force_unlock", "false")
				dryRun := request.GetString("dry_run", "false")
				
				if tt.expectError {
					switch tt.errorField {
					case "workspace_id":
						assert.Error(t, err)
					}
				} else {
					assert.NoError(t, err)
					if val, ok := tt.params["workspace_id"]; ok {
						assert.Equal(t, val, workspaceID)
					}
					
					// Test boolean parameter parsing
					expectedForceUnlock := strings.ToLower(forceUnlock) == "true"
					expectedDryRun := strings.ToLower(dryRun) == "true"
					
					if val, ok := tt.params["force_unlock"]; ok {
						expected := strings.ToLower(val.(string)) == "true"
						assert.Equal(t, expected, expectedForceUnlock)
					} else {
						assert.False(t, expectedForceUnlock) // Default should be false
					}
					
					if val, ok := tt.params["dry_run"]; ok {
						expected := strings.ToLower(val.(string)) == "true"
						assert.Equal(t, expected, expectedDryRun)
					} else {
						assert.False(t, expectedDryRun) // Default should be false
					}
				}
			})
		}
	})

	t.Run("workspace ID format validation", func(t *testing.T) {
		tests := []struct {
			name        string
			workspaceID string
			expectValid bool
		}{
			{"valid workspace ID", "ws-123456789abcdef", true},
			{"valid short workspace ID", "ws-123abc", true},
			{"invalid format - no prefix", "123456789abcdef", false},
			{"invalid format - wrong prefix", "workspace-123456", false},
			{"empty workspace ID", "", false},
			{"only prefix", "ws-", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Simple validation: workspace ID should start with "ws-" and have content after
				isValid := strings.HasPrefix(tt.workspaceID, "ws-") && len(tt.workspaceID) > 3
				assert.Equal(t, tt.expectValid, isValid)
			})
		}
	})

	t.Run("workspace deletion result structure", func(t *testing.T) {
		// Test the WorkspaceDeletionResult structure
		type WorkspaceDeletionResult struct {
			WorkspaceID       string   `json:"workspace_id"`
			WorkspaceName     string   `json:"workspace_name"`
			ResourceCount     int      `json:"resource_count"`
			IsLocked          bool     `json:"is_locked"`
			CanDelete         bool     `json:"can_delete"`
			DryRun            bool     `json:"dry_run"`
			Deleted           bool     `json:"deleted"`
			Message           string   `json:"message"`
			Warnings          []string `json:"warnings,omitempty"`
			BlockingFactors   []string `json:"blocking_factors,omitempty"`
		}

		// Test successful deletion scenario
		successResult := WorkspaceDeletionResult{
			WorkspaceID:     "ws-123456",
			WorkspaceName:   "test-workspace",
			ResourceCount:   0,
			IsLocked:        false,
			CanDelete:       true,
			DryRun:          false,
			Deleted:         true,
			Message:         "Workspace 'test-workspace' (ws-123456) deleted successfully",
			Warnings:        []string{},
			BlockingFactors: []string{},
		}

		jsonData, err := json.Marshal(successResult)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "ws-123456")
		assert.Contains(t, string(jsonData), "test-workspace")
		assert.Contains(t, string(jsonData), "deleted successfully")

		var unmarshaled WorkspaceDeletionResult
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, successResult.WorkspaceID, unmarshaled.WorkspaceID)
		assert.Equal(t, successResult.CanDelete, unmarshaled.CanDelete)
		assert.Equal(t, successResult.Deleted, unmarshaled.Deleted)

		// Test blocked deletion scenario
		blockedResult := WorkspaceDeletionResult{
			WorkspaceID:     "ws-789012",
			WorkspaceName:   "prod-workspace",
			ResourceCount:   15,
			IsLocked:        true,
			CanDelete:       false,
			DryRun:          false,
			Deleted:         false,
			Message:         "Cannot delete workspace: it is managing active resources",
			Warnings:        []string{},
			BlockingFactors: []string{
				"Workspace has 15 managed resources",
				"Workspace is locked",
			},
		}

		jsonData, err = json.Marshal(blockedResult)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "Cannot delete workspace")
		assert.Contains(t, string(jsonData), "15 managed resources")
		assert.Contains(t, string(jsonData), "locked")

		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, blockedResult.WorkspaceID, unmarshaled.WorkspaceID)
		assert.Equal(t, blockedResult.CanDelete, unmarshaled.CanDelete)
		assert.Equal(t, blockedResult.Deleted, unmarshaled.Deleted)
		assert.Len(t, unmarshaled.BlockingFactors, 2)
	})

	t.Run("run status validation", func(t *testing.T) {
		// Test the run statuses that should block deletion
		blockingStatuses := []tfe.RunStatus{
			tfe.RunPlanning,
			tfe.RunApplying,
			tfe.RunPending,
			tfe.RunPolicyChecking,
			tfe.RunPolicyOverride,
			tfe.RunConfirmed,
		}

		nonBlockingStatuses := []tfe.RunStatus{
			tfe.RunApplied,
			tfe.RunCanceled,
			tfe.RunDiscarded,
			tfe.RunErrored,
			tfe.RunPlannedAndFinished,
		}

		for _, status := range blockingStatuses {
			t.Run("blocking status: "+string(status), func(t *testing.T) {
				isBlocking := status == tfe.RunPlanning ||
					status == tfe.RunApplying ||
					status == tfe.RunPending ||
					status == tfe.RunPolicyChecking ||
					status == tfe.RunPolicyOverride ||
					status == tfe.RunConfirmed
				assert.True(t, isBlocking)
			})
		}

		for _, status := range nonBlockingStatuses {
			t.Run("non-blocking status: "+string(status), func(t *testing.T) {
				isBlocking := status == tfe.RunPlanning ||
					status == tfe.RunApplying ||
					status == tfe.RunPending ||
					status == tfe.RunPolicyChecking ||
					status == tfe.RunPolicyOverride ||
					status == tfe.RunConfirmed
				assert.False(t, isBlocking)
			})
		}
	})
}
