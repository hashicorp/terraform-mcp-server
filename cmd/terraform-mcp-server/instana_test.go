// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func instanaTestLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestSetupInstanaDisabledReturnsNil(t *testing.T) {
	t.Setenv("INSTANA_ENABLED", "")

	collector := setupInstana(instanaTestLogger())

	assert.Nil(t, collector)
}

func TestSetupInstanaEnabledInitializesCollector(t *testing.T) {
	t.Setenv("INSTANA_ENABLED", "true")

	collector := setupInstana(instanaTestLogger())

	assert.NotNil(t, collector)
}
