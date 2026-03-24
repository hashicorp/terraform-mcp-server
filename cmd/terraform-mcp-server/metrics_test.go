// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestSetupMetricsReturnsNoopWhenDisabled(t *testing.T) {
	t.Setenv("MCP_METRICS_ENABLED", "false")
	t.Setenv("MCP_METRICS_ENDPOINT", "")
	t.Setenv("MCP_METRICS_EXPORT_INTERVAL", "")
	t.Setenv("MCP_METRICS_SERVICE_NAME", "")
	t.Setenv("MCP_METRICS_SERVICE_VERSION", "")

	logger := testLogger()

	config, shutdown := setupMetrics(logger)

	assert.False(t, config.Enabled)
	assert.Nil(t, config.MeterProvider)
	assert.NotNil(t, shutdown)
	assert.NotPanics(t, shutdown)
}

func TestSetupMetricsReturnsNoopWhenInitFails(t *testing.T) {
	t.Setenv("MCP_METRICS_ENABLED", "true")
	t.Setenv("MCP_METRICS_ENDPOINT", "http://invalid-endpoint")
	t.Setenv("MCP_METRICS_EXPORT_INTERVAL", "2s")
	t.Setenv("MCP_METRICS_SERVICE_NAME", "terraform-mcp-server")
	t.Setenv("MCP_METRICS_SERVICE_VERSION", "test")

	logger := testLogger()

	config, shutdown := setupMetrics(logger)

	assert.True(t, config.Enabled)
	assert.Nil(t, config.MeterProvider)
	assert.NotNil(t, shutdown)
	assert.NotPanics(t, shutdown)
}

func TestInitMetricsInitializesAndFlushesOnShutdown(t *testing.T) {
	var exportRequests atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exportRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	collectorURL, err := url.Parse(collector.URL)
	require.NoError(t, err)

	config := client.DefaultMetricsConfig()
	config.Enabled = true
	config.Endpoint = collectorURL.Host
	config.ExportInterval = time.Hour
	config.ServiceName = "terraform-mcp-server"
	config.ServiceVersion = "9.9.9"

	logger := testLogger()

	shutdown, err := initMetrics(context.Background(), &config, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	require.NotNil(t, config.MeterProvider)
	require.NotNil(t, config.ToolCounter)
	require.NotNil(t, config.ErrorCounter)
	require.NotNil(t, config.ToolCallLatencyBucket)

	config.ToolCounter.Add(context.Background(), 1)
	shutdown()

	assert.GreaterOrEqual(t, exportRequests.Load(), int32(1))
	assert.Equal(t, "terraform-mcp-server", config.ServiceName)
	assert.Equal(t, "9.9.9", config.ServiceVersion)
	assert.NotNil(t, config.MeterProvider)
	assert.NotNil(t, config.ToolCounter)
	assert.NotNil(t, config.ErrorCounter)
	assert.NotNil(t, config.ToolCallLatencyBucket)
}

func TestNewServerRecordsToolErrorMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	meter := provider.Meter("test-service")
	toolCounter, err := meter.Int64Counter("mcp_tool_calls_total")
	require.NoError(t, err)
	errorCounter, err := meter.Int64Counter("mcp_tool_errors_total")
	require.NoError(t, err)
	latencyHistogram, err := meter.Float64Histogram("mcp_tool_duration_seconds")
	require.NoError(t, err)

	metricsConfig := client.MetricsConfig{
		Enabled:               true,
		ServiceName:           "terraform-mcp-server",
		ServiceVersion:        "test-version",
		MeterProvider:         provider,
		ToolCounter:           toolCounter,
		ErrorCounter:          errorCounter,
		ToolCallLatencyBucket: latencyHistogram,
	}

	server := NewServer("test-version", testLogger(), nil, metricsConfig)
	server.AddTool(mcp.NewTool("failing_tool"), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := mcp.NewToolResultText("tool failed")
		result.IsError = true
		return result, nil
	})

	requestBody, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "req-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "failing_tool",
		},
	})
	require.NoError(t, err)

	response := server.HandleMessage(context.Background(), requestBody)
	assert.NotNil(t, response)

	resourceMetrics := collectServerResourceMetrics(t, reader)

	toolCalls := findServerInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("tool.name", "failing_tool"))

	errorCalls := findServerInt64SumMetric(t, resourceMetrics, "mcp_tool_errors_total")
	require.Len(t, errorCalls.DataPoints, 1)
	assert.EqualValues(t, 1, errorCalls.DataPoints[0].Value)

	latency := findServerFloat64HistogramMetric(t, resourceMetrics, "mcp_tool_duration_seconds")
	require.Len(t, latency.DataPoints, 1)
	assert.EqualValues(t, 1, latency.DataPoints[0].Count)
	assert.Greater(t, latency.DataPoints[0].Sum, 0.0)
}

func TestNewServerAfterHookHandlesMissingStartTime(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	meter := provider.Meter("test-service")
	toolCounter, err := meter.Int64Counter("mcp_tool_calls_total")
	require.NoError(t, err)
	errorCounter, err := meter.Int64Counter("mcp_tool_errors_total")
	require.NoError(t, err)
	latencyHistogram, err := meter.Float64Histogram("mcp_tool_duration_seconds")
	require.NoError(t, err)

	metricsConfig := client.MetricsConfig{
		Enabled:               true,
		ServiceName:           "terraform-mcp-server",
		ServiceVersion:        "test-version",
		MeterProvider:         provider,
		ToolCounter:           toolCounter,
		ErrorCounter:          errorCounter,
		ToolCallLatencyBucket: latencyHistogram,
	}

	srv := NewServer("test-version", testLogger(), nil, metricsConfig)
	hooks := extractServerHooks(t, srv)
	require.NotEmpty(t, hooks.OnAfterCallTool)

	// Call after-hook directly without a corresponding before-hook store to force fallback startTime path.
	request := &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "fallback_tool"}}
	result := mcp.NewToolResultText("ok")

	assert.NotPanics(t, func() {
		hooks.OnAfterCallTool[0](context.Background(), "missing-start", request, result)
	})

	resourceMetrics := collectServerResourceMetrics(t, reader)

	toolCalls := findServerInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("tool.name", "fallback_tool"))

	latency := findServerFloat64HistogramMetric(t, resourceMetrics, "mcp_tool_duration_seconds")
	require.Len(t, latency.DataPoints, 1)
	assert.EqualValues(t, 1, latency.DataPoints[0].Count)
	assert.Greater(t, latency.DataPoints[0].Sum, 0.0)
}

func extractServerHooks(t *testing.T, srv *mcpserver.MCPServer) *mcpserver.Hooks {
	t.Helper()

	value := reflect.ValueOf(srv).Elem().FieldByName("hooks")
	require.True(t, value.IsValid())
	require.Equal(t, reflect.Pointer, value.Kind())
	require.False(t, value.IsNil())

	hooksPtr := reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).Elem()
	hooks, ok := hooksPtr.Interface().(*mcpserver.Hooks)
	require.True(t, ok)
	require.NotNil(t, hooks)

	return hooks
}

func testLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func collectServerResourceMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &resourceMetrics))
	return resourceMetrics
}

func findServerInt64SumMetric(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) metricdata.Sum[int64] {
	t.Helper()

	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[int64])
			require.Truef(t, ok, "metric %s was not an int64 sum", name)
			return data
		}
	}

	t.Fatalf("metric %s not found", name)
	return metricdata.Sum[int64]{}
}

func findServerFloat64HistogramMetric(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()

	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			data, ok := metric.Data.(metricdata.Histogram[float64])
			require.Truef(t, ok, "metric %s was not a float64 histogram", name)
			return data
		}
	}

	t.Fatalf("metric %s not found", name)
	return metricdata.Histogram[float64]{}
}
