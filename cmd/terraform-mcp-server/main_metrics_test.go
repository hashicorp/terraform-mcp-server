// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"io"
	"reflect"
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

func metricsTestLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestSetupMetricsDisabledReturnsNoopShutdown(t *testing.T) {
	t.Setenv("OTEL_METRICS_ENABLED", "")

	metricsConfig, shutdown := setupMetrics(metricsTestLogger())

	assert.False(t, metricsConfig.Enabled)
	assert.Nil(t, metricsConfig.MeterProvider)
	assert.NotPanics(t, shutdown)
}

func TestInitMetricsInvalidEndpointReturnsError(t *testing.T) {
	config := client.DefaultMetricsConfig()
	config.Endpoint = "://bad-endpoint"

	shutdown, err := initMetrics(context.Background(), &config, metricsTestLogger())

	require.Error(t, err)
	assert.Nil(t, shutdown)
	assert.Nil(t, config.MeterProvider)
	assert.Contains(t, err.Error(), "failed to create metrics exporter")
}

func TestInitMetricsSuccessInitializesInstrumentsAndShutdown(t *testing.T) {
	config := client.DefaultMetricsConfig()
	config.Endpoint = "localhost:4318"
	config.ExportInterval = 50 * time.Millisecond
	config.ServiceName = "terraform-mcp-server-test"
	config.ServiceVersion = "test"

	shutdown, err := initMetrics(context.Background(), &config, metricsTestLogger())
	require.NoError(t, err)
	require.NotNil(t, shutdown)

	assert.NotNil(t, config.MeterProvider)
	assert.NotNil(t, config.ToolCounter)
	assert.NotNil(t, config.ErrorCounter)
	assert.NotNil(t, config.ToolCallLatencyBucket)

	assert.NotPanics(t, shutdown)
}

func TestSetupMetricsEnabledInitializesAndReturnsShutdown(t *testing.T) {
	t.Setenv("OTEL_METRICS_ENABLED", "true")
	t.Setenv("OTEL_METRICS_ENDPOINT", "localhost:4318")
	t.Setenv("OTEL_METRICS_EXPORT_INTERVAL", "50ms")
	t.Setenv("OTEL_METRICS_SERVICE_NAME", "terraform-mcp-server-test")
	t.Setenv("OTEL_METRICS_SERVICE_VERSION", "test")

	metricsConfig, shutdown := setupMetrics(metricsTestLogger())

	assert.True(t, metricsConfig.Enabled)
	assert.NotNil(t, metricsConfig.MeterProvider)
	assert.NotNil(t, metricsConfig.ToolCounter)
	assert.NotNil(t, metricsConfig.ErrorCounter)
	assert.NotNil(t, metricsConfig.ToolCallLatencyBucket)
	assert.NotPanics(t, shutdown)
}

func newMetricsConfigForHooks(t *testing.T) (client.MetricsConfig, *sdkmetric.ManualReader) {
	t.Helper()

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

	return client.MetricsConfig{
		Enabled:               true,
		ServiceName:           "terraform-mcp-server",
		ServiceVersion:        "test-version",
		MeterProvider:         provider,
		ToolCounter:           toolCounter,
		ErrorCounter:          errorCounter,
		ToolCallLatencyBucket: latencyHistogram,
	}, reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &resourceMetrics))
	return resourceMetrics
}

func findInt64SumMetric(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) metricdata.Sum[int64] {
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

func findFloat64HistogramMetric(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
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

func getServerHooksForTest(t *testing.T, srv *mcpserver.MCPServer) *mcpserver.Hooks {
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

func TestAttachMetricsHooksNoopWhenDisabled(t *testing.T) {
	hooks := &mcpserver.Hooks{}

	attachMetricsHooks(hooks, client.MetricsConfig{Enabled: false}, metricsTestLogger())

	assert.Empty(t, hooks.OnBeforeCallTool)
	assert.Empty(t, hooks.OnAfterCallTool)
}

func TestAttachMetricsHooksRecordsToolCallAndLatency(t *testing.T) {
	metricsConfig, reader := newMetricsConfigForHooks(t)
	hooks := &mcpserver.Hooks{}
	attachMetricsHooks(hooks, metricsConfig, metricsTestLogger())
	require.Len(t, hooks.OnBeforeCallTool, 1)
	require.Len(t, hooks.OnAfterCallTool, 1)

	request := &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "test_tool"}}
	hooks.OnBeforeCallTool[0](context.Background(), "req-1", request)
	hooks.OnAfterCallTool[0](context.Background(), "req-1", request, mcp.NewToolResultText("ok"))

	resourceMetrics := collectMetrics(t, reader)
	toolCalls := findInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("tool.name", "test_tool"))

	latency := findFloat64HistogramMetric(t, resourceMetrics, "mcp_tool_duration_seconds")
	require.Len(t, latency.DataPoints, 1)
	assert.EqualValues(t, 1, latency.DataPoints[0].Count)
	assert.Greater(t, latency.DataPoints[0].Sum, 0.0)
}

func TestAttachMetricsHooksRecordsErrorResult(t *testing.T) {
	metricsConfig, reader := newMetricsConfigForHooks(t)
	hooks := &mcpserver.Hooks{}
	attachMetricsHooks(hooks, metricsConfig, metricsTestLogger())
	require.Len(t, hooks.OnBeforeCallTool, 1)
	require.Len(t, hooks.OnAfterCallTool, 1)

	request := &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "failing_tool"}}
	hooks.OnBeforeCallTool[0](context.Background(), "req-2", request)
	result := mcp.NewToolResultText("failed")
	result.IsError = true
	hooks.OnAfterCallTool[0](context.Background(), "req-2", request, result)

	resourceMetrics := collectMetrics(t, reader)
	errorCalls := findInt64SumMetric(t, resourceMetrics, "mcp_tool_errors_total")
	require.Len(t, errorCalls.DataPoints, 1)
	assert.EqualValues(t, 1, errorCalls.DataPoints[0].Value)
	assert.Contains(t, errorCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("tool.name", "failing_tool"))
}

func TestAttachMetricsHooksAfterCallWithoutStartTimeStillRecords(t *testing.T) {
	metricsConfig, reader := newMetricsConfigForHooks(t)
	hooks := &mcpserver.Hooks{}
	attachMetricsHooks(hooks, metricsConfig, metricsTestLogger())
	require.Len(t, hooks.OnAfterCallTool, 1)

	request := &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "fallback_tool"}}
	hooks.OnAfterCallTool[0](context.Background(), "req-3", request, mcp.NewToolResultText("ok"))

	resourceMetrics := collectMetrics(t, reader)
	toolCalls := findInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)
}

func TestAttachMetricsHooksMalformedIDDoesNotPanicAndRecordsMetrics(t *testing.T) {
	metricsConfig, reader := newMetricsConfigForHooks(t)
	hooks := &mcpserver.Hooks{}
	attachMetricsHooks(hooks, metricsConfig, metricsTestLogger())
	require.Len(t, hooks.OnBeforeCallTool, 1)
	require.Len(t, hooks.OnAfterCallTool, 1)

	request := &mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "malformed_id_tool"}}
	malformedID := []any{"bad", 1}

	assert.NotPanics(t, func() {
		hooks.OnBeforeCallTool[0](context.Background(), malformedID, request)
	})
	assert.NotPanics(t, func() {
		hooks.OnAfterCallTool[0](context.Background(), malformedID, request, mcp.NewToolResultText("ok"))
	})

	resourceMetrics := collectMetrics(t, reader)
	toolCalls := findInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("tool.name", "malformed_id_tool"))
}

func TestNewServerWithHooksOptionPassesThrough(t *testing.T) {
	customHooks := &mcpserver.Hooks{}
	customHooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {})

	srv := NewServer("test-version", metricsTestLogger(), nil, mcpserver.WithHooks(customHooks))
	hooks := getServerHooksForTest(t, srv)

	require.GreaterOrEqual(t, len(hooks.OnBeforeCallTool), 1)
}
