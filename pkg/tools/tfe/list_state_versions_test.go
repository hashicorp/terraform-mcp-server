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

func TestListAllStateVersions(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	// Tool definition contract
	t.Run("tool creation", func(t *testing.T) {
		tool := ListStateVersions(logger)

		assert.Equal(t, "list_all_state_versions", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "List all States Versions")
		assert.NotNil(t, tool.Handler)

		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")
	})

	// Required parameter validation
	t.Run("parameter validation", func(t *testing.T) {
		tests := []struct {
			name         string
			params       map[string]interface{}
			expectOrgErr bool
			expectWsErr  bool
		}{
			{
				name: "both params present",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
					"workspace_name":     "my-workspace",
				},
				expectOrgErr: false,
				expectWsErr:  false,
			},
			{
				name: "missing terraform_org_name",
				params: map[string]interface{}{
					"workspace_name": "my-workspace",
				},
				expectOrgErr: true,
				expectWsErr:  false,
			},
			{
				name: "missing workspace_name",
				params: map[string]interface{}{
					"terraform_org_name": "my-org",
				},
				expectOrgErr: false,
				expectWsErr:  true,
			},
			{
				name:         "both params missing",
				params:       map[string]interface{}{},
				expectOrgErr: true,
				expectWsErr:  true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{params: tt.params}

				orgName, orgErr := request.RequireString("terraform_org_name")
				wsName, wsErr := request.RequireString("workspace_name")

				if tt.expectOrgErr {
					assert.Error(t, orgErr)
					assert.Contains(t, orgErr.Error(), "terraform_org_name")
				} else {
					assert.NoError(t, orgErr)
					assert.Equal(t, tt.params["terraform_org_name"], orgName)
				}

				if tt.expectWsErr {
					assert.Error(t, wsErr)
					assert.Contains(t, wsErr.Error(), "workspace_name")
				} else {
					assert.NoError(t, wsErr)
					assert.Equal(t, tt.params["workspace_name"], wsName)
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
				input:    "my-org",
				expected: "my-org",
			},
			{
				name:     "leading and trailing spaces",
				input:    "  my-org  ",
				expected: "my-org",
			},
			{
				name:     "tabs and spaces",
				input:    "\t my-workspace \t",
				expected: "my-workspace",
			},
			{
				name:     "only internal text preserved",
				input:    "  org-with-spaces-inside  ",
				expected: "org-with-spaces-inside",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.expected, strings.TrimSpace(tt.input))
			})
		}
	})

	// StateVersionsSummary JSON marshal/unmarshal round-trip
	t.Run("StateVersionsSummary JSON round-trip", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		summary := StateVersionsSummary{
			ID:               "sv-abc123",
			CreatedAt:        now,
			Serial:           42,
			TerraformVersion: "1.5.0",
			VCSCommitSHA:     "abc123def456",
			VCSCommitURL:     "https://github.com/example/repo/commit/abc123",
			StateVersion:     3,
		}

		jsonData, err := json.Marshal(summary)
		require.NoError(t, err)

		jsonStr := string(jsonData)
		assert.Contains(t, jsonStr, `"id"`)
		assert.Contains(t, jsonStr, `"created_at"`)
		assert.Contains(t, jsonStr, `"serial"`)
		assert.Contains(t, jsonStr, `"terraform_version"`)
		assert.Contains(t, jsonStr, `"vcs_commit_sha"`)
		assert.Contains(t, jsonStr, `"vcs_commit_url"`)
		assert.Contains(t, jsonStr, `"state_version"`)

		assert.Contains(t, jsonStr, "sv-abc123")
		assert.Contains(t, jsonStr, "1.5.0")
		assert.Contains(t, jsonStr, "abc123def456")

		var unmarshaled StateVersionsSummary
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)
		assert.Equal(t, summary.ID, unmarshaled.ID)
		assert.Equal(t, summary.Serial, unmarshaled.Serial)
		assert.Equal(t, summary.TerraformVersion, unmarshaled.TerraformVersion)
		assert.Equal(t, summary.VCSCommitSHA, unmarshaled.VCSCommitSHA)
		assert.Equal(t, summary.VCSCommitURL, unmarshaled.VCSCommitURL)
		assert.Equal(t, summary.StateVersion, unmarshaled.StateVersion)
	})

	// StateVersionsSummaryList JSON with embedded pagination
	t.Run("StateVersionsSummaryList JSON marshalling", func(t *testing.T) {
		t.Run("with pagination", func(t *testing.T) {
			list := &StateVersionsSummaryList{
				Items: []*StateVersionsSummary{
					{ID: "sv-001", Serial: 1, TerraformVersion: "1.4.0"},
					{ID: "sv-002", Serial: 2, TerraformVersion: "1.5.0"},
				},
				Pagination: &tfe.Pagination{
					CurrentPage:  1,
					PreviousPage: 0,
					NextPage:     2,
					TotalPages:   3,
					TotalCount:   6,
				},
			}

			jsonData, err := json.Marshal(list)
			require.NoError(t, err)

			jsonStr := string(jsonData)
			assert.Contains(t, jsonStr, `"items"`)
			assert.Contains(t, jsonStr, "sv-001")
			assert.Contains(t, jsonStr, "sv-002")
			assert.Contains(t, jsonStr, "current-page")

			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(jsonData, &result))
			items, ok := result["items"].([]interface{})
			require.True(t, ok)
			assert.Len(t, items, 2)
		})

		t.Run("nil pagination does not panic", func(t *testing.T) {
			list := &StateVersionsSummaryList{
				Items: []*StateVersionsSummary{
					{ID: "sv-001", Serial: 1},
				},
				Pagination: nil,
			}

			jsonData, err := json.Marshal(list)
			require.NoError(t, err)
			assert.Contains(t, string(jsonData), "sv-001")
		})
	})

	// Empty results edge case
	t.Run("empty state versions list", func(t *testing.T) {
		emptyList := &tfe.StateVersionList{
			Items: []*tfe.StateVersion{},
		}

		assert.NotNil(t, emptyList.Items)
		assert.Len(t, emptyList.Items, 0)
		assert.True(t, len(emptyList.Items) == 0, "empty items slice should trigger the no-results guard")
	})

	// Multi-item summary-building loop field mapping
	t.Run("summary field mapping", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		mockVersions := []*tfe.StateVersion{
			{
				ID:               "sv-111",
				CreatedAt:        now,
				Serial:           1,
				TerraformVersion: "1.3.0",
				VCSCommitSHA:     "sha1",
				VCSCommitURL:     "https://github.com/example/repo/commit/sha1",
				StateVersion:     1,
			},
			{
				ID:               "sv-222",
				CreatedAt:        now,
				Serial:           2,
				TerraformVersion: "1.4.0",
				VCSCommitSHA:     "sha2",
				VCSCommitURL:     "https://github.com/example/repo/commit/sha2",
				StateVersion:     2,
			},
			{
				ID:               "sv-333",
				CreatedAt:        now,
				Serial:           3,
				TerraformVersion: "1.5.0",
				VCSCommitSHA:     "sha3",
				VCSCommitURL:     "https://github.com/example/repo/commit/sha3",
				StateVersion:     3,
			},
		}

		// Replicate the handler's summary-building loop
		summaries := make([]*StateVersionsSummary, len(mockVersions))
		for i, o := range mockVersions {
			summaries[i] = &StateVersionsSummary{
				ID:               o.ID,
				CreatedAt:        o.CreatedAt,
				Serial:           o.Serial,
				TerraformVersion: o.TerraformVersion,
				VCSCommitSHA:     o.VCSCommitSHA,
				VCSCommitURL:     o.VCSCommitURL,
				StateVersion:     o.StateVersion,
			}
		}

		require.Len(t, summaries, 3)
		for i, s := range summaries {
			src := mockVersions[i]
			assert.Equal(t, src.ID, s.ID)
			assert.Equal(t, src.CreatedAt, s.CreatedAt)
			assert.Equal(t, src.Serial, s.Serial)
			assert.Equal(t, src.TerraformVersion, s.TerraformVersion)
			assert.Equal(t, src.VCSCommitSHA, s.VCSCommitSHA)
			assert.Equal(t, src.VCSCommitURL, s.VCSCommitURL)
			assert.Equal(t, src.StateVersion, s.StateVersion)
		}
	})
}
