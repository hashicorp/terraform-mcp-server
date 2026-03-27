// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetSentinelMock(t *testing.T) {
	logger := log.New()

	t.Run("tool creation", func(t *testing.T) {
		tool := GetSentinelMock(logger)

		assert.Equal(t, "get_sentinel_mock", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Sentinel mock")
		assert.NotNil(t, tool.Handler)
	})
}
