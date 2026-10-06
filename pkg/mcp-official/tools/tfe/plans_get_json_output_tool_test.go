// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlanJSONOutputTool(t *testing.T) {
	tool := GetPlanJSONOutputTool()

	assert.Equal(t, "get_plan_json_output", tool.Name)
	assert.Contains(t, tool.Description, "Retrieves the JSON output of a specific Terraform plan")
	assert.Nil(t, tool.InputSchema, "get_plan_json_output relies on the SDK deriving its input schema")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get JSON output for a Terraform plan", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetPlanJSONOutputArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetPlanJSONOutputArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"plan_id"}, schema.Required)

	planID := schema.Properties["plan_id"]
	require.NotNil(t, planID)
	assert.Equal(t, "string", planID.Type)
	assert.Contains(t, planID.Description, "The ID of the plan to get JSON output for")
}

func TestGetPlanJSONOutputFunc_RequiresPlanID(t *testing.T) {
	tests := []struct {
		name   string
		planID string
	}{
		{name: "empty", planID: ""},
		{name: "whitespace only", planID: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetPlanJSONOutputFunc(t.Context(), nil, GetPlanJSONOutputArguments{PlanID: tt.planID})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing required input: plan_id")
		})
	}
}
