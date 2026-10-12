// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStateVersionTool(t *testing.T) {
	tool := GetStateVersionTool()

	assert.Equal(t, "get_state_version", tool.Name)
	assert.Contains(t, tool.Annotations.Title, "Get Terraform state version")

	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetStateVersionArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetStateVersionArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Empty(t, schema.Required)

	stateVersionID := schema.Properties["state_version_id"]
	require.NotNil(t, stateVersionID)
	assert.Equal(t, "string", stateVersionID.Type)

	workspaceID := schema.Properties["workspace_id"]
	require.NotNil(t, workspaceID)
	assert.Equal(t, "string", workspaceID.Type)
}

func TestGetStateVersionSchemaValidation(t *testing.T) {
	schema, err := jsonschema.For[GetStateVersionArguments](nil)
	require.NoError(t, err)
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		params      map[string]any
		expectError bool
	}{
		{name: "state version present", params: map[string]any{"state_version_id": "sv-abc123"}},
		{name: "workspace present", params: map[string]any{"workspace_id": "ws-abc123"}},
		{name: "invalid state version type", params: map[string]any{"state_version_id": 123}, expectError: true},
		{name: "invalid workspace type", params: map[string]any{"workspace_id": 123}, expectError: true},
		// Missing IDs must reach the handler so it can return the existing error message.
		{name: "both IDs missing", params: map[string]any{}},
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

func TestGetStateVersionFunc_ReturnsErrorWhenIDsAreMissingOrBlank(t *testing.T) {
	tests := []struct {
		name  string
		input GetStateVersionArguments
	}{
		{name: "missing IDs"},
		{name: "blank IDs", input: GetStateVersionArguments{StateVersionID: "  ", WorkspaceID: "\t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, output, err := GetStateVersionFunc(t.Context(), nil, tt.input)
			require.EqualError(t, err, "One of state_version_id or workspace_id must be provided")
			assert.Nil(t, result)
			assert.Nil(t, output)
		})
	}
}

// These will fail when go-tfe adds an attribute the details structs don't carry.
func TestStateVersionDetailsCoversStateVersionAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.StateVersion{}), reflect.TypeOf(StateVersionDetails{}), map[string]string{
		"UploadURL":               "upload URLs are for writing state; get_state_version is read only",
		"JSONUploadURL":           "upload URLs are for writing state; get_state_version is read only",
		"SanitizedStateUploadURL": "upload URLs are for writing state; get_state_version is read only",
		"Modules":                 "go-tfe only decodes two hard-coded root module counts; resources has the full breakdown",
		"Providers":               "go-tfe's provider[map]string key never matches the API payload, so it is always zero",
	})
}

func TestStateVersionResourceCoversAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.StateVersionResources{}), reflect.TypeOf(StateVersionResource{}), nil)
}

func TestStateVersionToDetails(t *testing.T) {
	t.Run("nil state version", func(t *testing.T) {
		assert.Nil(t, stateVersionToDetails(nil))
	})

	t.Run("maps top level fields", func(t *testing.T) {
		now := time.Now()
		rumCount := uint32(7)
		details := stateVersionToDetails(&tfe.StateVersion{
			ID:                 "sv-abc123",
			CreatedAt:          now,
			Status:             tfe.StateVersionFinalized,
			Serial:             12,
			StateVersion:       4,
			Size:               2048,
			TerraformVersion:   "1.9.0",
			VCSCommitSHA:       "abc123",
			VCSCommitURL:       "https://example.com/commit/abc123",
			ResourcesProcessed: true,
			BillableRUMCount:   &rumCount,
			DownloadURL:        "https://archivist.example.com/state",
			JSONDownloadURL:    "https://archivist.example.com/json-state",
			UploadURL:          "https://archivist.example.com/upload",
		})

		require.NotNil(t, details)
		assert.Equal(t, "sv-abc123", details.ID)
		assert.Equal(t, now, details.CreatedAt)
		assert.Equal(t, "finalized", details.Status)
		assert.Equal(t, int64(12), details.Serial)
		assert.Equal(t, 4, details.StateVersion)
		assert.Equal(t, int64(2048), details.Size)
		assert.Equal(t, "1.9.0", details.TerraformVersion)
		assert.Equal(t, "abc123", details.VCSCommitSHA)
		assert.Equal(t, "https://example.com/commit/abc123", details.VCSCommitURL)
		assert.True(t, details.ResourcesProcessed)
		assert.Equal(t, &rumCount, details.BillableRUMCount)
		assert.Equal(t, "https://archivist.example.com/state", details.DownloadURL)
		assert.Equal(t, "https://archivist.example.com/json-state", details.JSONDownloadURL)
	})

	t.Run("leaves relations empty when absent", func(t *testing.T) {
		details := stateVersionToDetails(&tfe.StateVersion{ID: "sv-abc123"})

		require.NotNil(t, details)
		assert.Empty(t, details.RunID)
		require.NotNil(t, details.Resources, "resources must marshal as [] not null")
		assert.Empty(t, details.Resources)
		require.NotNil(t, details.OutputIDs, "output_ids must marshal as [] not null")
		assert.Empty(t, details.OutputIDs)
	})

	t.Run("maps resources and relation ids, skipping nil entries", func(t *testing.T) {
		details := stateVersionToDetails(&tfe.StateVersion{
			ID: "sv-abc123",
			Resources: []*tfe.StateVersionResources{
				{Name: "web", Count: 2, Type: "aws_instance", Module: "root", Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]"},
				nil,
			},
			Run:     &tfe.Run{ID: "run-abc123"},
			Outputs: []*tfe.StateVersionOutput{{ID: "wsout-1"}, nil, {ID: "wsout-2"}},
		})

		require.Len(t, details.Resources, 1)
		assert.Equal(t, StateVersionResource{
			Name:     "web",
			Count:    2,
			Type:     "aws_instance",
			Module:   "root",
			Provider: "provider[\"registry.terraform.io/hashicorp/aws\"]",
		}, details.Resources[0])
		assert.Equal(t, "run-abc123", details.RunID)
		assert.Equal(t, []string{"wsout-1", "wsout-2"}, details.OutputIDs)
	})
}

func TestGetStateVersionToolOutputSchema(t *testing.T) {
	schema := inputSchema(t, GetStateVersionTool().OutputSchema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Required, "resources")
	assert.Contains(t, schema.Required, "output_ids")

	resources := schema.Properties["resources"]
	require.NotNil(t, resources)
	assert.Equal(t, "array", resources.Type)
	assert.Equal(t, "object", resources.Items.Type)

	assert.Equal(t, "array", schema.Properties["output_ids"].Type)
	assert.Equal(t, "string", schema.Properties["run_id"].Type)
	assert.NotContains(t, schema.Required, "run_id")
}
