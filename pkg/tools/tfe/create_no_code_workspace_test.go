// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNoCodeWorkspace(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create a mock MCP server for testing
	mcpServer := &server.MCPServer{}

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateNoCodeWorkspace(logger, mcpServer)

		// Check that the tool is properly configured
		assert.Equal(t, "create_no_code_workspace", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform No Code module workspace")

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "no_code_module_id")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that it accepts open world parameters (for dynamic variables)
		// The tool should be configured to accept additional parameters beyond those defined
		assert.NotNil(t, tool.Tool.InputSchema.Properties)
		assert.Contains(t, tool.Tool.InputSchema.Properties, "no_code_module_id")
		assert.Contains(t, tool.Tool.InputSchema.Properties, "workspace_name")
		assert.Contains(t, tool.Tool.InputSchema.Properties, "auto_apply")

		// Verify the tool has elicitation capabilities through its configuration
		// The WithOpenWorldHintAnnotation(true) allows for dynamic parameter acceptance
		annotations := tool.Tool.Annotations
		assert.NotNil(t, annotations)
		assert.NotNil(t, annotations.OpenWorldHint)
		assert.True(t, *annotations.OpenWorldHint)

		// Handler should not be nil
		assert.NotNil(t, tool.Handler)
	})
}

func TestBuildElicitationSchemaOnlyRequiresRequiredVariables(t *testing.T) {
	moduleMetadata := testModuleMetadata(t)

	properties, requestedVars, requiredVars := buildElicitationSchema(moduleMetadata, &tfe.RegistryNoCodeModule{})

	assert.Equal(t, []string{"name", "description", "enabled", "count"}, requestedVars)
	assert.Equal(t, []string{"name"}, requiredVars)
	assert.Len(t, properties, 4)
}

func TestExtractVariablesFromResponseHonorsOptionalVariables(t *testing.T) {
	moduleMetadata := testModuleMetadata(t)
	properties, requestedVars, requiredVars := buildElicitationSchema(moduleMetadata, &tfe.RegistryNoCodeModule{})

	t.Run("omitted optional variables use module defaults", func(t *testing.T) {
		variables, err := extractVariablesFromResponse(
			map[string]any{"name": "example"},
			requestedVars,
			requiredVars,
			properties,
		)

		require.NoError(t, err)
		require.Len(t, variables, 1)
		assert.Equal(t, "name", variables[0].Key)
		assert.Equal(t, "example", variables[0].Value)
	})

	t.Run("empty optional string uses module default", func(t *testing.T) {
		variables, err := extractVariablesFromResponse(
			map[string]any{"name": "example", "description": ""},
			requestedVars,
			requiredVars,
			properties,
		)

		require.NoError(t, err)
		require.Len(t, variables, 1)
		assert.Equal(t, "name", variables[0].Key)
	})

	t.Run("false and zero optional values are preserved", func(t *testing.T) {
		variables, err := extractVariablesFromResponse(
			map[string]any{"name": "example", "enabled": false, "count": float64(0)},
			requestedVars,
			requiredVars,
			properties,
		)

		require.NoError(t, err)
		require.Len(t, variables, 3)
		assert.Equal(t, "false", variables[1].Value)
		assert.Equal(t, "0", variables[2].Value)
	})

	t.Run("missing required variable remains invalid", func(t *testing.T) {
		_, err := extractVariablesFromResponse(map[string]any{}, requestedVars, requiredVars, properties)

		require.EqualError(t, err, "required variable 'name' is missing from response")
	})

	t.Run("empty required string remains invalid", func(t *testing.T) {
		_, err := extractVariablesFromResponse(
			map[string]any{"name": ""},
			requestedVars,
			requiredVars,
			properties,
		)

		require.EqualError(t, err, "variable 'name' cannot be empty")
	})
}

func testModuleMetadata(t *testing.T) *client.ModuleMetadata {
	t.Helper()

	metadataJSON := []byte(`{
		"data": {
			"attributes": {
				"input-variables": [
					{"name": "name", "type": "string", "required": true},
					{"name": "description", "type": "string", "required": false},
					{"name": "enabled", "type": "bool", "required": false},
					{"name": "count", "type": "number", "required": false}
				]
			}
		}
	}`)

	var moduleMetadata client.ModuleMetadata
	require.NoError(t, json.Unmarshal(metadataJSON, &moduleMetadata))
	return &moduleMetadata
}
