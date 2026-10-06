// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"io"
	"log/slog"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSentinelMockTool(t *testing.T) {
	tool := GetSentinelMockTool()

	assert.Equal(t, "get_sentinel_mock", tool.Name)
	assert.Contains(t, tool.Description, "Exports and downloads Sentinel mock bundle data for a Terraform plan")
	assert.Nil(t, tool.InputSchema, "get_sentinel_mock relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get the Sentinel mock for a Terraform plan", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetSentinelMockArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetSentinelMockArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"plan_id"}, schema.Required)

	planID := schema.Properties["plan_id"]
	require.NotNil(t, planID)
	assert.Equal(t, "string", planID.Type)
	assert.Contains(t, planID.Description, "The ID of the plan to export Sentinel mock data for")
}

func TestGetSentinelMockFunc_RequiresPlanID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := GetSentinelMockFunc(logger)

	tests := []struct {
		name   string
		planID string
	}{
		{name: "empty", planID: ""},
		{name: "whitespace only", planID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := handler(t.Context(), nil, GetSentinelMockArguments{PlanID: tt.planID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required input: plan_id")
		})
	}
}
