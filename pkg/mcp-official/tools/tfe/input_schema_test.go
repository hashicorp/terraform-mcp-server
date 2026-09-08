// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputSchemasMatchArgumentTypes(t *testing.T) {
	t.Run("list organizations", func(t *testing.T) {
		assertInputSchemaMatches[ListOrganizationsArguments](t, ListTerraformOrganizationsTool())
	})
	t.Run("list projects", func(t *testing.T) {
		assertInputSchemaMatches[ListProjectsArguments](t, ListProjectsTool())
	})
	t.Run("list workspaces", func(t *testing.T) {
		assertInputSchemaMatches[ListWorkspacesArguments](t, ListWorkspacesTool())
	})
	t.Run("create project", func(t *testing.T) {
		assertInputSchemaMatches[CreateProjectArguments](t, CreateProjectTool())
	})
	t.Run("get project", func(t *testing.T) {
		assertInputSchemaMatches[GetProjectArguments](t, GetProjectTool())
	})
	t.Run("delete project", func(t *testing.T) {
		assertInputSchemaMatches[DeleteProjectArguments](t, DeleteProjectTool())
	})
}

func TestInputSchemasRejectUnknownProperties(t *testing.T) {
	tests := []struct {
		tool  *mcp.Tool
		input map[string]any
	}{
		{tool: ListTerraformOrganizationsTool(), input: map[string]any{}},
		{tool: ListProjectsTool(), input: map[string]any{"terraform_org_name": "org"}},
		{tool: ListWorkspacesTool(), input: map[string]any{"terraform_org_name": "org"}},
		{tool: CreateProjectTool(), input: map[string]any{
			"terraform_org_name": "org",
			"project_name":       "project",
		}},
		{tool: GetProjectTool(), input: map[string]any{"project_id": "prj-123"}},
		{tool: DeleteProjectTool(), input: map[string]any{"project_id": "prj-123"}},
	}

	for _, tt := range tests {
		t.Run(tt.tool.Name, func(t *testing.T) {
			schema := requireInputSchema(t, tt.tool)
			resolved, err := schema.Resolve(nil)
			require.NoError(t, err)

			tt.input["unknown"] = true
			input := any(tt.input)
			err = resolved.Validate(&input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unexpected additional properties")
		})
	}
}

func TestPaginationInputSchema(t *testing.T) {
	tools := []*mcp.Tool{
		ListTerraformOrganizationsTool(),
		ListProjectsTool(),
		ListWorkspacesTool(),
	}

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			properties := requireInputSchema(t, tool).Properties

			page := properties["page"]
			require.NotNil(t, page)
			assert.Equal(t, "integer", page.Type)
			assert.Equal(t, float64(defaultPage), requireFloat(t, page.Minimum))

			pageSize := properties["pageSize"]
			require.NotNil(t, pageSize)
			assert.Equal(t, "integer", pageSize.Type)
			assert.Equal(t, float64(minPageSize), requireFloat(t, pageSize.Minimum))
			assert.Equal(t, float64(maxPageSize), requireFloat(t, pageSize.Maximum))
		})
	}
}

func TestCreateProjectInputConstraints(t *testing.T) {
	properties := requireInputSchema(t, CreateProjectTool()).Properties

	projectName := properties["project_name"]
	require.NotNil(t, projectName)
	assert.Equal(t, 3, requireInt(t, projectName.MinLength))
	assert.Equal(t, 40, requireInt(t, projectName.MaxLength))
	assert.Equal(t, `^[A-Za-z0-9_-][A-Za-z0-9 _-]*[A-Za-z0-9_-]$`, projectName.Pattern)

	description := properties["description"]
	require.NotNil(t, description)
	assert.Equal(t, 256, requireInt(t, description.MaxLength))

	executionMode := properties["default_execution_mode"]
	require.NotNil(t, executionMode)
	assert.Equal(t, []any{executionModeLocal, executionModeAgent, executionModeRemote}, executionMode.Enum)
}

func TestListOutputSchemasRemainExplicit(t *testing.T) {
	tools := []*mcp.Tool{
		ListTerraformOrganizationsTool(),
		ListProjectsTool(),
		ListWorkspacesTool(),
	}

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			schema, ok := tool.OutputSchema.(*jsonschema.Schema)
			require.True(t, ok)
			require.NotNil(t, schema)

			items := schema.Properties["items"]
			require.NotNil(t, items)
			assert.Equal(t, "array", items.Type)
			require.NotNil(t, items.Items)
			assert.Equal(t, "object", items.Items.Type)
		})
	}
}

func assertInputSchemaMatches[T any](t *testing.T, tool *mcp.Tool) {
	t.Helper()

	schema := requireInputSchema(t, tool)
	assert.Equal(t, "object", schema.Type)
	require.NotNil(t, schema.AdditionalProperties)
	require.NotNil(t, schema.AdditionalProperties.Not)

	wantProperties, wantRequired := argumentJSONFields(reflect.TypeFor[T]())
	assert.ElementsMatch(t, wantProperties, mapKeys(schema.Properties), "schema properties do not match argument JSON fields")
	assert.ElementsMatch(t, wantRequired, schema.Required, "schema required fields do not match argument JSON tags")
	assert.Equal(t, wantProperties, schema.PropertyOrder, "schema property order does not match argument field order")
}

func argumentJSONFields(argumentType reflect.Type) ([]string, []string) {
	var properties []string
	var required []string
	for _, field := range reflect.VisibleFields(argumentType) {
		if field.Anonymous || !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("json")
		parts := strings.Split(tag, ",")
		if len(parts) == 0 || parts[0] == "-" {
			continue
		}

		name := parts[0]
		if name == "" {
			name = field.Name
		}
		properties = append(properties, name)

		optional := false
		for _, option := range parts[1:] {
			if option == "omitempty" || option == "omitzero" {
				optional = true
				break
			}
		}
		if !optional {
			required = append(required, name)
		}
	}
	return properties, required
}

func requireInputSchema(t *testing.T, tool *mcp.Tool) *jsonschema.Schema {
	t.Helper()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.NotNil(t, schema)
	return schema
}

func mapKeys(values map[string]*jsonschema.Schema) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func requireFloat(t *testing.T, value *float64) float64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireInt(t *testing.T, value *int) int {
	t.Helper()
	require.NotNil(t, value)
	return *value
}
