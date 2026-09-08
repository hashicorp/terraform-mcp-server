// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
)

// toolError logs a tool failure and returns it for the official SDK to convert
// into a CallToolResult with IsError set to true.
func toolError(logger *log.Logger, message string, err error) error {
	toolErr := errors.New(message)
	if err != nil {
		toolErr = fmt.Errorf("%s: %w", message, err)
	}
	if logger != nil {
		logger.Errorf("Tool error: %s", toolErr)
	}
	return toolErr
}
