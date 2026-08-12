// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRunSafe(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateRunSafe(logger)

		assert.Equal(t, "create_run", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that destructive hint is false
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that run_type property exists
		runTypeProperty := tool.Tool.InputSchema.Properties["run_type"]
		assert.NotNil(t, runTypeProperty)
	})
}

func TestCreateRun(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateRun(logger)

		assert.Equal(t, "create_run", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new Terraform run")
		assert.NotNil(t, tool.Handler)

		// Check that destructive hint is true
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.True(t, *tool.Tool.Annotations.DestructiveHint)

		// Check required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "workspace_name")

		// Check that run_type property exists
		runTypeProperty := tool.Tool.InputSchema.Properties["run_type"]
		assert.NotNil(t, runTypeProperty)
	})
}

func TestCreateRunLockedWorkspace(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)
	logger.SetOutput(io.Discard)

	terraformServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, err := io.WriteString(w, `{"data":{"id":"test", "type":"workspaces", "attributes":{"name":"test-workspace","locked":true}}}`)
		assert.NoError(t, err)
	}))

	t.Cleanup(terraformServer.Close)
	t.Setenv(client.TerraformAddress, terraformServer.URL)
	t.Setenv(client.TerraformToken, "test-token")

	mcpServer := server.NewMCPServer("test", "1.0.0")
	ctx := mcpServer.WithContext(t.Context(), server.NewInProcessSession("", nil))
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"terraform_org_name": "test-org",
		"workspace_name":     "test-workspace",
	}}}

	tests := []struct {
		name string
		tool server.ServerTool
	}{
		{name: "safe tool", tool: CreateRunSafe(logger)},
		{name: "full tool", tool: CreateRun(logger)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.tool.Handler(ctx, request)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			content, ok := result.Content[0].(mcp.TextContent)
			require.True(t, ok)
			assert.Equal(t, `workspace "test-workspace" is locked and cannot accept new runs. Use the force_unlock_workspace tool to unlock first`, content.Text)
		})
	}
}
