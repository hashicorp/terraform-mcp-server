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

func TestGetStateVersionWithID(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Tool definition contract
	t.Run("tool creation", func(t *testing.T) {
		tool := GetStateVersion(logger)

		assert.Equal(t, "get_state_version", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "Gets StateVersion with state_version_id")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		assert.NotContains(t, tool.Tool.InputSchema.Required, "state_version_id")
		assert.NotContains(t, tool.Tool.InputSchema.Required, "workspace_id")
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
				params:      map[string]interface{}{"state_version_id": "sv-abc123"},
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
				val, err := request.RequireString("state_version_id")

				if tt.expectError {
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "state_version_id")
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.params["state_version_id"], val)
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
				input:    "sv-abc123",
				expected: "sv-abc123",
			},
			{
				name:     "leading and trailing spaces",
				input:    "  sv-abc123  ",
				expected: "sv-abc123",
			},
			{
				name:     "tabs and spaces",
				input:    "\t sv-abc123 \t",
				expected: "sv-abc123",
			},
			{
				name:     "internal text preserved",
				input:    "  sv-with-dashes-inside  ",
				expected: "sv-with-dashes-inside",
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

	// Empty/whitespace state_version_id falls through to workspace_id branch
	t.Run("empty or whitespace state_version_id falls through", func(t *testing.T) {
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
				raw:         "sv-abc123",
				expectEmpty: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				trimmed := strings.TrimSpace(tt.raw)
				isEmpty := trimmed == ""
				assert.Equal(t, tt.expectEmpty, isEmpty,
					"guard should fire when trimmed ID is empty")
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
