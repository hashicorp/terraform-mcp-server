package client

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestLoadMetricsConfigFromEnv(t *testing.T) {
	t.Run("uses environment overrides", func(t *testing.T) {
		t.Setenv("OTEL_METRICS_ENDPOINT", "collector.internal:4318")
		t.Setenv("OTEL_METRICS_EXPORT_INTERVAL", "5s")
		t.Setenv("OTEL_METRICS_SERVICE_NAME", "custom-mcp")
		t.Setenv("OTEL_METRICS_SERVICE_VERSION", "1.2.3")
		t.Setenv("OTEL_METRICS_ENABLED", "true")

		config := LoadMetricsConfigFromEnv()

		assert.True(t, config.Enabled)
		assert.Equal(t, "collector.internal:4318", config.Endpoint)
		assert.Equal(t, 5*time.Second, config.ExportInterval)
		assert.Equal(t, "custom-mcp", config.ServiceName)
		assert.Equal(t, "1.2.3", config.ServiceVersion)
	})

	t.Run("keeps default export interval when invalid", func(t *testing.T) {
		t.Setenv("OTEL_METRICS_ENDPOINT", "")
		t.Setenv("OTEL_METRICS_EXPORT_INTERVAL", "not-a-duration")
		t.Setenv("OTEL_METRICS_SERVICE_NAME", "")
		t.Setenv("OTEL_METRICS_SERVICE_VERSION", "")
		t.Setenv("OTEL_METRICS_ENABLED", "")

		config := LoadMetricsConfigFromEnv()
		defaults := DefaultMetricsConfig()

		assert.Equal(t, defaults.ExportInterval, config.ExportInterval)
		assert.Equal(t, defaults.Endpoint, config.Endpoint)
		assert.Equal(t, defaults.ServiceName, config.ServiceName)
		assert.Equal(t, defaults.ServiceVersion, config.ServiceVersion)
		assert.False(t, config.Enabled)
	})
}

func TestRecordToolCallRecordsMetrics(t *testing.T) {
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

	config := MetricsConfig{
		Enabled:               true,
		ServiceName:           "terraform-mcp-server",
		ServiceVersion:        "1.2.3",
		MeterProvider:         provider,
		ToolCounter:           toolCounter,
		ErrorCounter:          errorCounter,
		ToolCallLatencyBucket: latencyHistogram,
	}

	logger := log.New()
	logger.SetOutput(io.Discard)

	request := &mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "list_runs"},
	}

	RecordToolCall(
		context.Background(),
		time.Now().Add(-150*time.Millisecond),
		true,
		"request-1",
		request,
		config,
		logger,
	)

	resourceMetrics := collectResourceMetrics(t, reader)

	toolCalls := findInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("tool.name", "list_runs"))
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("service.name", "terraform-mcp-server"))
	assert.Contains(t, toolCalls.DataPoints[0].Attributes.ToSlice(), attribute.String("service.version", "1.2.3"))

	errorCalls := findInt64SumMetric(t, resourceMetrics, "mcp_tool_errors_total")
	require.Len(t, errorCalls.DataPoints, 1)
	assert.EqualValues(t, 1, errorCalls.DataPoints[0].Value)

	latency := findFloat64HistogramMetric(t, resourceMetrics, "mcp_tool_duration_seconds")
	require.Len(t, latency.DataPoints, 1)
	assert.EqualValues(t, 1, latency.DataPoints[0].Count)
	assert.Greater(t, latency.DataPoints[0].Sum, 0.0)
}

func TestRecordToolCallSkipsWhenMetricsDisabled(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)

	assert.NotPanics(t, func() {
		RecordToolCall(
			context.Background(),
			time.Now(),
			false,
			"request-2",
			&mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "list_runs"}},
			MetricsConfig{Enabled: false},
			logger,
		)
	})
}

func collectResourceMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &resourceMetrics))
	return resourceMetrics
}

func findInt64SumMetric(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) metricdata.Sum[int64] {
	t.Helper()

	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				data, ok := metric.Data.(metricdata.Sum[int64])
				require.Truef(t, ok, "metric %s was not an int64 sum", name)
				return data
			}
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
