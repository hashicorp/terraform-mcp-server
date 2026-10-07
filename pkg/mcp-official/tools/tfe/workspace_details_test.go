// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceToDetails(t *testing.T) {
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
		TagNames:            []string{"production"},
		ResourceCount:       2,
		Locked:              true,
	})

	assert.Equal(t, "ws-123", details.ID)
	assert.Equal(t, "workspace", details.Name)
	assert.Equal(t, []string{"infra/"}, details.TriggerPrefixes)
	assert.Equal(t, []string{"production"}, details.TagNames)
	assert.True(t, details.AutoApply)
	assert.True(t, details.Locked)
}

func TestWorkspaceToDetailsUsesNonNilCollections(t *testing.T) {
	details := workspaceToDetails(&tfe.Workspace{})
	assert.NotNil(t, details.TriggerPrefixes)
	assert.NotNil(t, details.TagNames)

	nilDetails := workspaceToDetails(nil)
	assert.NotNil(t, nilDetails.TriggerPrefixes)
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
	assert.Equal(t, "array", schema.Properties["tag_names"].Type)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
}
