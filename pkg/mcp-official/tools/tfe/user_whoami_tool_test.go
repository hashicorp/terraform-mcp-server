// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhoAmITool(t *testing.T) {
	tool := WhoAmITool()

	assert.Equal(t, "whoami", tool.Name)
	assert.NotEmpty(t, tool.Description)

	// WhoAmIFunc takes an "any" input, so mcp.AddTool substitutes {"type": "object"}
	// at registration time. Setting a schema here would advertise arguments the tool
	// does not read.
	assert.Nil(t, tool.InputSchema)

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get current Terraform identity", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
}
