// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlanLogsTool(t *testing.T) {
	tool := GetPlanLogsTool()

	assert.Equal(t, "get_plan_logs", tool.Name)
	assert.Contains(t, tool.Description, "Retrieves the logs of a specific Terraform plan")
	assert.Nil(t, tool.InputSchema, "get_plan_logs relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get logs for a Terraform plan", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetPlanLogsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetPlanLogsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"plan_id"}, schema.Required)

	planID := schema.Properties["plan_id"]
	require.NotNil(t, planID)
	assert.Equal(t, "string", planID.Type)
	assert.Contains(t, planID.Description, "The ID of the plan to get logs for")
}

func TestGetPlanLogsFunc_RequiresPlanID(t *testing.T) {
	tests := []struct {
		name   string
		planID string
	}{
		{name: "empty", planID: ""},
		{name: "whitespace only", planID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetPlanLogsFunc(t.Context(), nil, GetPlanLogsArguments{PlanID: tt.planID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required input: plan_id")
		})
	}
}
