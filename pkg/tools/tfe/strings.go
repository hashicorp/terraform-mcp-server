// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// GetTrimmedString wrapper: function around request.GetString() that also calls TrimSpace()
func GetTrimmedString(request mcp.CallToolRequest, name, defaultValue string) string {
	value := request.GetString(name, defaultValue)
	return strings.TrimSpace(value)
}
