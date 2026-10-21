// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRunDetailsTool(t *testing.T) {
	tool := GetRunDetailsTool()
	assert.Equal(t, "get_run_details", tool.Name)
	assert.Nil(t, tool.InputSchema)
	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetRunDetailsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetRunDetailsArguments](nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"run_id"}, schema.Required)
	assert.Equal(t, "string", schema.Properties["run_id"].Type)
}

func TestGetRunDetailsFuncRequiresRunID(t *testing.T) {
	for _, runID := range []string{"", "  "} {
		_, _, err := GetRunDetailsFunc(t.Context(), nil, GetRunDetailsArguments{RunID: runID})
		require.EqualError(t, err, "run_id must not be blank")
	}
}

func TestRunDetailsCoversRunAttributes(t *testing.T) {
	assertMapsAllAttributes(t, reflect.TypeOf(tfe.Run{}), reflect.TypeOf(RunDetails{}), map[string]string{
		"Variables": "run variables may contain secrets and provide no sensitivity metadata",
	})
}

func TestRunToDetails(t *testing.T) {
	createdAt := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	run := &tfe.Run{
		ID:                "run-1",
		CreatedAt:         createdAt,
		Status:            tfe.RunPlanning,
		Message:           "message",
		InvokeActionAddrs: []string{"action.one"},
		PolicyPaths:       []string{"policies"},
		ReplaceAddrs:      []string{"resource.one"},
		TargetAddrs:       []string{"resource.two"},
		Actions:           &tfe.RunActions{IsCancelable: true},
		Permissions:       &tfe.RunPermissions{CanCancel: true},
		Workspace:         &tfe.Workspace{ID: "ws-1"},
		Plan:              &tfe.Plan{ID: "plan-1"},
	}

	details := runToDetails(run)
	assert.Equal(t, "run-1", details.ID)
	assert.Equal(t, "planning", details.Status)
	assert.Equal(t, createdAt.Format(time.RFC3339Nano), details.CreatedAt)
	assert.Equal(t, []string{"action.one"}, details.InvokeActionAddrs)
	assert.Equal(t, "ws-1", details.WorkspaceID)
	assert.Equal(t, "plan-1", details.PlanID)
	require.NotNil(t, details.Actions)
	assert.True(t, details.Actions.IsCancelable)
	require.NotNil(t, details.Permissions)
	assert.True(t, details.Permissions.CanCancel)

	nilDetails := runToDetails(nil)
	assert.NotNil(t, nilDetails.InvokeActionAddrs)
	assert.NotNil(t, nilDetails.PolicyPaths)
	assert.NotNil(t, nilDetails.ReplaceAddrs)
	assert.NotNil(t, nilDetails.TargetAddrs)
}

func TestGetRunDetailsToolOutputSchema(t *testing.T) {
	schema, ok := GetRunDetailsTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.NotContains(t, schema.Properties, "variables")
	assert.Equal(t, "array", schema.Properties["target_addrs"].Type)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	details := runToDetails(&tfe.Run{ID: "run-1", CreatedAt: time.Now(), Status: tfe.RunPending})
	data, err := json.Marshal(details)
	require.NoError(t, err)
	var value any
	require.NoError(t, json.Unmarshal(data, &value))
	assert.NoError(t, resolved.Validate(&value))
}
