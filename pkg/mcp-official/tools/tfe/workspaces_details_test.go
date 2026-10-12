// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceToDetails(t *testing.T) {
	createdAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	details := workspaceToDetails(&tfe.Workspace{
		ID:                  "ws-123",
		Name:                "workspace",
		Description:         "description",
		AutoApply:           true,
		ExecutionMode:       "remote",
		TerraformVersion:    "1.14.0",
		WorkingDirectory:    "infra",
		QueueAllRuns:        true,
		SpeculativeEnabled:  true,
		FileTriggersEnabled: true,
		TriggerPrefixes:     []string{"infra/"},
		TriggerPatterns:     []string{"modules/**"},
		TagNames:            []string{"production"},
		ResourceCount:       2,
		Locked:              true,
		AssessmentsEnabled:  true,
		GlobalRemoteState:   true,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	})

	assert.Equal(t, "ws-123", details.ID)
	assert.Equal(t, "workspace", details.Name)
	assert.Equal(t, []string{"infra/"}, details.TriggerPrefixes)
	assert.Equal(t, []string{"modules/**"}, details.TriggerPatterns)
	assert.Equal(t, []string{"production"}, details.TagNames)
	assert.True(t, details.AutoApply)
	assert.True(t, details.Locked)
	assert.True(t, details.AssessmentsEnabled)
	assert.True(t, details.GlobalRemoteState)
	assert.Equal(t, createdAt, details.CreatedAt)
	assert.Equal(t, updatedAt, details.UpdatedAt)
}

func TestWorkspaceToDetailsUsesNonNilCollections(t *testing.T) {
	details := workspaceToDetails(&tfe.Workspace{})
	assert.NotNil(t, details.TriggerPrefixes)
	assert.NotNil(t, details.TriggerPatterns)
	assert.NotNil(t, details.TagNames)

	nilDetails := workspaceToDetails(nil)
	assert.NotNil(t, nilDetails.TriggerPrefixes)
	assert.NotNil(t, nilDetails.TriggerPatterns)
	assert.NotNil(t, nilDetails.TagNames)
}

func TestWorkspaceDetailsSchema(t *testing.T) {
	schema := workspaceDetailsSchema()
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	details := workspaceToDetails(&tfe.Workspace{ID: "ws-123", Name: "workspace"})
	data, err := json.Marshal(details)
	require.NoError(t, err)
	var value any
	require.NoError(t, json.Unmarshal(data, &value))
	assert.NoError(t, resolved.Validate(&value))

	assert.Equal(t, "array", schema.Properties["trigger_prefixes"].Type)
	assert.Equal(t, "array", schema.Properties["trigger_patterns"].Type)
	assert.Equal(t, "array", schema.Properties["tag_names"].Type)
	assert.Equal(t, "date-time", schema.Properties["created_at"].Format)
	assert.Equal(t, "date-time", schema.Properties["updated_at"].Format)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}
