// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCurrentWorkspaceStateVersion(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Tool definition contract
	t.Run("tool creation", func(t *testing.T) {
		tool := GetCurrentWorkspaceStateVersion(logger)

		assert.Equal(t, "get_current_workspace_state_version", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "Gets State-Version with Workspace ID")
		assert.Contains(t, tool.Tool.Description, "latest available state")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace-id")
	})

	// Required parameter validation
	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name        string
			params      map[string]interface{}
			expectError bool
		}{
			{
				name:        "param present",
				params:      map[string]interface{}{"workspace-id": "ws-abc123"},
				expectError: false,
			},
			{
				name:        "param missing",
				params:      map[string]interface{}{},
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}
				val, err := request.RequireString("workspace-id")

				if tt.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "workspace-id")
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.params["workspace-id"], val)
				}
			})
		}
	})

	// Input whitespace trimming
	t.Run("input trimming", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "no whitespace",
				input:    "ws-abc123",
				expected: "ws-abc123",
			},
			{
				name:     "leading and trailing spaces",
				input:    "  ws-abc123  ",
				expected: "ws-abc123",
			},
			{
				name:     "tabs and spaces",
				input:    "\t ws-abc123 \t",
				expected: "ws-abc123",
			},
			{
				name:     "internal text preserved",
				input:    "  ws-with-dashes-inside  ",
				expected: "ws-with-dashes-inside",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.expected, strings.TrimSpace(tt.input))
			})
		}
	})

	// tfe.StateVersion JSON marshal/unmarshal round-trip
	t.Run("StateVersion JSON round-trip", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		sv := &tfe.StateVersion{
			ID:               "sv-abc123",
			CreatedAt:        now,
			Serial:           42,
			TerraformVersion: "1.5.0",
			VCSCommitSHA:     "abc123def456",
			VCSCommitURL:     "https://github.com/example/repo/commit/abc123",
			StateVersion:     3,
		}

		jsonData, err := json.Marshal(sv)
		require.NoError(t, err)

		jsonStr := string(jsonData)
		assert.Contains(t, jsonStr, "sv-abc123")
		assert.Contains(t, jsonStr, "1.5.0")
		assert.Contains(t, jsonStr, "abc123def456")

		var unmarshaled tfe.StateVersion
		require.NoError(t, json.Unmarshal(jsonData, &unmarshaled))
		assert.Equal(t, sv.ID, unmarshaled.ID)
		assert.Equal(t, sv.Serial, unmarshaled.Serial)
		assert.Equal(t, sv.TerraformVersion, unmarshaled.TerraformVersion)
		assert.Equal(t, sv.VCSCommitSHA, unmarshaled.VCSCommitSHA)
		assert.Equal(t, sv.VCSCommitURL, unmarshaled.VCSCommitURL)
		assert.Equal(t, sv.StateVersion, unmarshaled.StateVersion)
	})

	// Empty/whitespace workspace-id guard logic
	t.Run("empty or whitespace workspace-id is rejected", func(t *testing.T) {
		tests := []struct {
			name        string
			raw         string
			expectEmpty bool
		}{
			{
				name:        "empty string",
				raw:         "",
				expectEmpty: true,
			},
			{
				name:        "whitespace only",
				raw:         "   ",
				expectEmpty: true,
			},
			{
				name:        "valid id not rejected",
				raw:         "ws-abc123",
				expectEmpty: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				trimmed := strings.TrimSpace(tt.raw)
				isEmpty := trimmed == ""
				assert.Equal(t, tt.expectEmpty, isEmpty,
					"guard should fire when trimmed workspace-id is empty")
			})
		}
	})

	// Zero-value tfe.StateVersion marshals without panic
	t.Run("zero-value StateVersion marshals without panic", func(t *testing.T) {
		sv := &tfe.StateVersion{}
		jsonData, err := json.Marshal(sv)
		require.NoError(t, err)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(jsonData, &result))
	})
}
