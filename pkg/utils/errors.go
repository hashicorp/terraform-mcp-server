// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

// ToolError returns a Tool Execution Error that the model can see and learn from.
// Unlike Protocol Errors, Tool Execution Errors are returned to the LLM's context window

func ToolError(logger *log.Logger, message string, err error) (*mcp.CallToolResult, error) {
	fullMessage := message
	if err != nil {
		fullMessage = fmt.Sprintf("%s: %v", message, err)
	}
	if logger != nil {
		logger.Errorf("Tool error: %s", fullMessage)
	}
	return mcp.NewToolResultError(fullMessage), nil
}

// ToolErrorf returns a formatted Tool Execution Error that the model can see.
func ToolErrorf(logger *log.Logger, format string, args ...interface{}) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("Tool error: %s", message)
	}
	return mcp.NewToolResultError(message), nil
}
