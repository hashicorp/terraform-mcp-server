// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package instana

import (
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func testLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestSetupDisabledReturnsNil(t *testing.T) {
	t.Setenv("INSTANA_ENABLED", "")

	collector := Setup(testLogger())

	assert.Nil(t, collector)
}

func TestSetupEnabledInitializesCollector(t *testing.T) {
	t.Setenv("INSTANA_ENABLED", "true")

	collector := Setup(testLogger())

	assert.NotNil(t, collector)
}
