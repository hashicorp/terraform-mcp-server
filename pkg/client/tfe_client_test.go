// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestBuildTFEConfig_SharedSecret(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("sets shared secret header when env is set", func(t *testing.T) {
		t.Setenv(SharedSecretEnv, "super-secret-value")
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "", logger)
		assert.Equal(t, "super-secret-value", cfg.Headers.Get(SharedSecretHeader))
	})

	t.Run("omits shared secret header when env is unset", func(t *testing.T) {
		t.Setenv(SharedSecretEnv, "")
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "", logger)
		assert.Empty(t, cfg.Headers.Get(SharedSecretHeader))
	})
}

func TestBuildTFEConfig_ForwardedFor(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	t.Run("sets X-Forwarded-For when clientIP is provided", func(t *testing.T) {
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "203.0.113.5", logger)
		assert.Equal(t, "203.0.113.5", cfg.Headers.Get("X-Forwarded-For"))
	})

	t.Run("omits X-Forwarded-For when clientIP is empty", func(t *testing.T) {
		cfg := buildTFEConfig("https://app.terraform.io", false, "token", "", logger)
		assert.Empty(t, cfg.Headers.Get("X-Forwarded-For"))
	})
}
