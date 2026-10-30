// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlanChangesTool(t *testing.T) {
	tool := GetPlanChangesTool()
	assert.Equal(t, "get_plan_changes", tool.Name)
	assert.Contains(t, tool.Description, "Never returns attribute values")
	require.NotNil(t, tool.Annotations)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
}

func TestGetPlanChangesArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetPlanChangesArguments](nil)
	require.NoError(t, err)
	assert.Empty(t, schema.Required)
	assert.Equal(t, "string", schema.Properties["run_id"].Type)
	assert.Equal(t, "string", schema.Properties["plan_id"].Type)
}
