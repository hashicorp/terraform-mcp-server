// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package util

import (
	"errors"
	"fmt"
	"github.com/hashicorp/go-tfe"
	"github.com/sirupsen/logrus"
)

func LogAndWrapError(logger *logrus.Logger, context string, err error) error {
	var wrappedErr error
	switch {
	case err == nil:
		wrappedErr = fmt.Errorf("%s", context)
		logger.Errorf("Error: %s", context)

	case errors.Is(err, tfe.ErrUnauthorized):
		wrappedErr = fmt.Errorf("%s: %w. Please set HCP_TFE_TOKEN in your MCP Server configuration correctly", context, err)
		logger.Errorf("Unauthorized: %s: %v", context, err)

	default:
		wrappedErr = fmt.Errorf("%s: %w", context, err)
		logger.Errorf("Error: %s: %v", context, err)
	}

	return wrappedErr
}
