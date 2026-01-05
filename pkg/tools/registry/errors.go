// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

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

func ToolErrorf(logger *log.Logger, format string, args ...interface{}) (*mcp.CallToolResult, error) {
	message := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("Tool error: %s", message)
	}
	return mcp.NewToolResultError(message), nil
}
