// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package otelmetrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

	assert.NotNil(t, config.ClientTypeCounter)
	// A nil OrgToolCallCounter would silently turn every org-metric call into a no-op
	assert.NotNil(t, config.OrgToolCallCounter)

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

func TestNewResourceHasServiceInstanceID(t *testing.T) {
	hostname, err := os.Hostname()
	require.NoError(t, err)
	require.NotEmpty(t, hostname, "this test needs a hostname to check the fallback")

	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "env set", env: "3f1c2a9e-7b1d-4c55-9a53-0d6f3a8b1e11", want: "3f1c2a9e-7b1d-4c55-9a53-0d6f3a8b1e11"},
		{name: "env set with surrounding whitespace is trimmed", env: "  pod-a \n", want: "pod-a"},
		{name: "env empty falls back to hostname", env: "", want: hostname},
		{name: "env whitespace only falls back to hostname", env: "   \t\n", want: hostname},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ServiceInstanceIDEnv, tt.env)
			config := client.DefaultMetricsConfig()

			// Look the attribute up on the resource itself: this is what the meter provider exports
			value, ok := newResource(&config).Set().Value("service.instance.id")

			require.True(t, ok, "newResource must set service.instance.id")
			assert.Equal(t, tt.want, value.AsString())
		})
	}
}

// newResource is only useful if Init passes it to the meter provider, so check
// what a real exporter sends to a collector.
func TestInitExportsServiceInstanceID(t *testing.T) {
	var mu sync.Mutex
	var payload []byte
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		payload = append(payload, body...)
		mu.Unlock()
	}))
	defer collector.Close()

	t.Setenv(ServiceInstanceIDEnv, "pod-from-env")
	config := client.DefaultMetricsConfig()
	config.Enabled = true
	config.Endpoint = strings.TrimPrefix(collector.URL, "http://")
	config.ExportInterval = 50 * time.Millisecond

	shutdown, err := Init(context.Background(), &config, testLogger())
	require.NoError(t, err)
	// Record one data point so there is something to export
	client.RecordOrgToolCall(context.Background(), "acme", config, testLogger())
	shutdown() // flushes pending metrics to the collector

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, string(payload), "service.instance.id")
	assert.Contains(t, string(payload), "pod-from-env")
}
