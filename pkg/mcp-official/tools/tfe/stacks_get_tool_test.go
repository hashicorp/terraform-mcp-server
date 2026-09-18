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

func TestGetStackDetailsTool(t *testing.T) {
	tool := GetStackDetailsTool()

	assert.Equal(t, "get_stack_details", tool.Name)
	assert.Contains(t, tool.Description, "Fetches detailed information about a Terraform Stack")
	assert.Nil(t, tool.InputSchema, "get_stack_details relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get a Terraform Stack by ID", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetStackDetailsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetStackDetailsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"stack_id"}, schema.Required)

	stackID := schema.Properties["stack_id"]
	require.NotNil(t, stackID)
	assert.Equal(t, "string", stackID.Type)
}

func TestGetStackDetailsFunc_RequiresStackID(t *testing.T) {
	tests := []struct {
		name    string
		stackID string
	}{
		{name: "empty", stackID: ""},
		{name: "whitespace only", stackID: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetStackDetailsFunc(t.Context(), nil, GetStackDetailsArguments{StackID: tt.stackID})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "stack_id must not be blank")
		})
	}
}

// These will fail when go-tfe adds an attribute the details structs don't carry.
func TestStackDetailsCoversStackAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.Stack{}), reflect.TypeOf(StackDetails{}), nil)
}

func TestStackVCSRepoCoversAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.StackVCSRepo{}), reflect.TypeOf(StackVCSRepo{}), nil)
}

func TestStackToDetails(t *testing.T) {
	t.Run("nil stack", func(t *testing.T) {
		assert.Nil(t, stackToDetails(nil))
	})

	t.Run("maps top level fields", func(t *testing.T) {
		now := time.Now()
		details := stackToDetails(&tfe.Stack{
			ID:                 "st-abc123",
			Name:               "networking",
			Description:        "shared networking stack",
			SpeculativeEnabled: true,
			CreatedAt:          now,
			UpdatedAt:          now,
			UpstreamCount:      1,
			DownstreamCount:    2,
			InputsCount:        3,
			OutputsCount:       4,
			CreationSource:     "tfe-ui",
			WorkingDirectory:   "stacks/networking",
			TriggerPatterns:    []string{"stacks/**"},
		})

		require.NotNil(t, details)
		assert.Equal(t, "st-abc123", details.ID)
		assert.Equal(t, "networking", details.Name)
		assert.Equal(t, "shared networking stack", details.Description)
		assert.True(t, details.SpeculativeEnabled)
		assert.Equal(t, now, details.CreatedAt)
		assert.Equal(t, 1, details.UpstreamCount)
		assert.Equal(t, 2, details.DownstreamCount)
		assert.Equal(t, 3, details.InputsCount)
		assert.Equal(t, 4, details.OutputsCount)
		assert.Equal(t, "tfe-ui", details.CreationSource)
		assert.Equal(t, "stacks/networking", details.WorkingDirectory)
		assert.Equal(t, []string{"stacks/**"}, details.TriggerPatterns)
	})

	t.Run("leaves relations empty when absent", func(t *testing.T) {
		details := stackToDetails(&tfe.Stack{ID: "st-abc123"})

		require.NotNil(t, details)
		assert.Nil(t, details.VCSRepo)
		assert.Empty(t, details.ProjectID)
		assert.Empty(t, details.AgentPoolID)
		assert.Empty(t, details.LatestStackConfigurationID)
	})

	t.Run("maps vcs repo and relation ids", func(t *testing.T) {
		details := stackToDetails(&tfe.Stack{
			ID: "st-abc123",
			VCSRepo: &tfe.StackVCSRepo{
				Identifier:   "hashicorp/stacks-demo",
				Branch:       "main",
				OAuthTokenID: "ot-abc123",
			},
			Project:                  &tfe.Project{ID: "prj-abc123"},
			AgentPool:                &tfe.AgentPool{ID: "apool-abc123"},
			LatestStackConfiguration: &tfe.StackConfiguration{ID: "sc-abc123"},
		})

		require.NotNil(t, details.VCSRepo)
		assert.Equal(t, "hashicorp/stacks-demo", details.VCSRepo.Identifier)
		assert.Equal(t, "main", details.VCSRepo.Branch)
		assert.Equal(t, "ot-abc123", details.VCSRepo.OAuthTokenID)
		assert.Equal(t, "prj-abc123", details.ProjectID)
		assert.Equal(t, "apool-abc123", details.AgentPoolID)
		assert.Equal(t, "sc-abc123", details.LatestStackConfigurationID)
	})
}
