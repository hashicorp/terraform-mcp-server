// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
)

// mockCallToolRequest implements mcp.CallToolRequest for internal tool calls
type mockCallToolRequest struct {
	params map[string]interface{}
}

func (m *mockCallToolRequest) GetString(key, defaultValue string) string {
	if value, ok := m.params[key].(string); ok {
		return value
	}
	return defaultValue
}

func (m *mockCallToolRequest) GetInt(key string, defaultValue int) int {
	if value, ok := m.params[key].(int); ok {
		return value
	}
	if value, ok := m.params[key].(float64); ok {
		return int(value)
	}
	return defaultValue
}

func (m *mockCallToolRequest) GetBool(key string, defaultValue bool) bool {
	if value, ok := m.params[key].(bool); ok {
		return value
	}
	return defaultValue
}

func (m *mockCallToolRequest) GetParams() map[string]interface{} {
	return m.params
}

// parseToolResult extracts data from MCP tool result
func parseToolResult(result *mcp.CallToolResult, target interface{}) error {
	if result == nil {
		return nil
	}

	// Get the content from the result
	var content string
	for _, c := range result.Content {
		if textContent, ok := c.(mcp.TextContent); ok {
			content = textContent.Text
			break
		}
	}

	if content == "" {
		return nil
	}

	// Parse JSON content
	return json.Unmarshal([]byte(content), target)
}
