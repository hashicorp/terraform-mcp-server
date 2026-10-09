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

func TestListRunsTool(t *testing.T) {
	tool := ListRunsTool()
	assert.Equal(t, "list_runs", tool.Name)
	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestListRunsToolInputSchema(t *testing.T) {
	schema := inputSchema(t, ListRunsTool().InputSchema)
	assert.Equal(t, []string{"terraform_org_name", "workspace_name", "vcs_username", "status", "page", "pageSize"}, schema.PropertyOrder)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)
	assert.Equal(t, enumOf(runStatuses...), schema.Properties["status"].Items.Enum)
	assert.Contains(t, schema.Properties["status"].Items.Enum, "force_canceled")
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}

func TestListRunsFuncRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		input   ListRunsArguments
		wantErr string
	}{
		{input: ListRunsArguments{}, wantErr: "terraform_org_name must not be blank"},
		{input: ListRunsArguments{TerraformOrgName: "org", Status: []string{"unknown"}}, wantErr: `invalid status "unknown"`},
	}
	for _, test := range tests {
		_, _, err := ListRunsFunc(t.Context(), nil, test.input)
		require.EqualError(t, err, test.wantErr)
	}
}

func TestRunSummaries(t *testing.T) {
	createdAt := time.Now()
	summaries := runSummaries([]*tfe.Run{
		nil,
		{ID: "run-1", Status: tfe.RunPlanned, CreatedAt: createdAt, Workspace: &tfe.Workspace{Name: "workspace"}},
		{ID: "run-2", Status: tfe.RunErrored, CreatedAt: createdAt},
	})

	require.Len(t, summaries, 2)
	assert.Equal(t, "workspace", summaries[0].WorkspaceName)
	assert.Empty(t, summaries[1].WorkspaceName)
	assert.NotNil(t, runSummaries(nil))
}

func TestRunPaginationDetails(t *testing.T) {
	assert.Equal(t, PaginationDetails{}, runPaginationDetails(nil))
	assert.Equal(t, PaginationDetails{CurrentPage: 2, PreviousPage: 1, NextPage: 3}, runPaginationDetails(&tfe.PaginationNextPrev{
		CurrentPage: 2, PreviousPage: 1, NextPage: 3,
	}))
}

func TestListRunsToolOutputSchema(t *testing.T) {
	schema, ok := ListRunsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	assert.Equal(t, "object", items.Items.Type)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	responses := []RunSummaryList{
		{Items: []RunSummary{}},
		{Items: []RunSummary{{ID: "run-1", Status: "pending", CreatedAt: time.Now()}}},
	}
	for _, response := range responses {
		data, err := json.Marshal(response)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}
