// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStateVersionsTool(t *testing.T) {
	tool := ListStateVersionsTool()

	assert.Equal(t, "list_state_versions", tool.Name)
	assert.Contains(t, tool.Annotations.Title, "List Terraform state versions")

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Contains(t, schema.Required, "terraform_org_name")
	assert.Contains(t, schema.Required, "workspace_name")
	assert.NotContains(t, schema.Required, "page")
	assert.NotContains(t, schema.Required, "pageSize")
}

func TestListStateVersionsParameterValidation(t *testing.T) {
	tool := ListStateVersionsTool()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "both params present", params: map[string]any{"terraform_org_name": "my-org", "workspace_name": "my-workspace"}},
		{name: "missing organization", params: map[string]any{"workspace_name": "my-workspace"}, expectError: true},
		{name: "missing workspace", params: map[string]any{"terraform_org_name": "my-org"}, expectError: true},
		{name: "both params missing", params: map[string]any{}, expectError: true},
		{name: "valid pagination", params: map[string]any{"terraform_org_name": "my-org", "workspace_name": "my-workspace", "page": 2, "pageSize": 100}},
		{name: "page below minimum", params: map[string]any{"terraform_org_name": "my-org", "workspace_name": "my-workspace", "page": 0}, expectError: true},
		{name: "pageSize below minimum", params: map[string]any{"terraform_org_name": "my-org", "workspace_name": "my-workspace", "pageSize": 0}, expectError: true},
		{name: "pageSize above maximum", params: map[string]any{"terraform_org_name": "my-org", "workspace_name": "my-workspace", "pageSize": 101}, expectError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolved.Validate(tt.params)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListStateVersionsOutputSchema(t *testing.T) {
	tool := ListStateVersionsTool()
	schema, ok := tool.OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)

	list := schema.Properties["items"]
	assert.Equal(t, "array", list.Type)
	assert.Empty(t, list.Types)
	assert.Equal(t, "object", list.Items.Type)
	assert.Empty(t, list.Items.Types)
	assert.ElementsMatch(t, []string{"items", "current-page", "prev-page", "next-page", "total-count", "total-pages"}, schema.Required)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	summary := StateVersionSummary{
		ID:               "sv-123",
		CreatedAt:        time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		Serial:           1,
		TerraformVersion: "1.9.0",
		VCSCommitSHA:     "abc123",
		VCSCommitURL:     "https://example.com/commit/abc123",
		StateVersion:     4,
	}

	tests := []struct {
		name        string
		output      any
		expectError bool
	}{
		{
			name: "items with pagination",
			output: &StateVersionSummaryList{
				Items:      []StateVersionSummary{summary},
				Pagination: &tfe.Pagination{CurrentPage: 1, TotalCount: 1, TotalPages: 1},
			},
		},
		{
			name: "empty items with pagination",
			output: &StateVersionSummaryList{
				Items:      []StateVersionSummary{},
				Pagination: &tfe.Pagination{CurrentPage: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, isRaw := tt.output.(string)
			b := []byte(raw)
			if !isRaw {
				var err error
				b, err = json.Marshal(tt.output)
				require.NoError(t, err)
			}
			var m map[string]any
			require.NoError(t, json.Unmarshal(b, &m))

			err := resolved.Validate(m)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
