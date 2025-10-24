// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
