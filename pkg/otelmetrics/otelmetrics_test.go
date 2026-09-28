// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package otelmetrics

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestSetupDisabledReturnsNoopShutdown(t *testing.T) {
	t.Setenv("OTEL_METRICS_ENABLED", "")

	metricsConfig, shutdown := Setup(testLogger())

	assert.False(t, metricsConfig.Enabled)
	assert.Nil(t, metricsConfig.MeterProvider)
	assert.NotPanics(t, shutdown)
}

func TestInitInvalidEndpointReturnsError(t *testing.T) {
	config := client.DefaultMetricsConfig()
	config.Endpoint = "://bad-endpoint"

	shutdown, err := Init(context.Background(), &config, testLogger())

	require.Error(t, err)
	assert.Nil(t, shutdown)
	assert.Nil(t, config.MeterProvider)
	assert.Contains(t, err.Error(), "failed to create metrics exporter")
}

func TestInitSuccessInitializesInstrumentsAndShutdown(t *testing.T) {
	config := client.DefaultMetricsConfig()
	config.Endpoint = "localhost:4318"
	config.ExportInterval = 50 * time.Millisecond
	config.ServiceName = "terraform-mcp-server-test"
	config.ServiceVersion = "test"

	shutdown, err := Init(context.Background(), &config, testLogger())
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	assert.NotNil(t, config.MeterProvider)
	assert.NotNil(t, config.ToolCounter)
	assert.NotNil(t, config.ErrorCounter)
	assert.NotNil(t, config.ToolCallLatencyBucket)

	assert.NotPanics(t, shutdown)
}

func TestSetupEnabledInitializesAndReturnsShutdown(t *testing.T) {
	t.Setenv("OTEL_METRICS_ENABLED", "true")
	t.Setenv("OTEL_METRICS_ENDPOINT", "localhost:4318")
	t.Setenv("OTEL_METRICS_EXPORT_INTERVAL", "50ms")
	t.Setenv("OTEL_METRICS_SERVICE_NAME", "terraform-mcp-server-test")
	t.Setenv("OTEL_METRICS_SERVICE_VERSION", "test")

	metricsConfig, shutdown := Setup(testLogger())

	assert.True(t, metricsConfig.Enabled)
	assert.NotNil(t, metricsConfig.MeterProvider)
	assert.NotNil(t, metricsConfig.ToolCounter)
	assert.NotNil(t, metricsConfig.ErrorCounter)
	assert.NotNil(t, metricsConfig.ToolCallLatencyBucket)
	assert.NotPanics(t, shutdown)
}
