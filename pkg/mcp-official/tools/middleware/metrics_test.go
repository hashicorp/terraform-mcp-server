// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// newTestMetricsConfig builds a MetricsConfig backed by a ManualReader so
// tests can synchronously collect whatever the Metrics middleware recorded.
func newTestMetricsConfig(t *testing.T) (tfeclient.MetricsConfig, *sdkmetric.ManualReader) {
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
	clientTypeCounter, err := meter.Int64Counter("mcp_client_type_total")
	require.NoError(t, err)
	orgToolCallCounter, err := meter.Int64Counter("mcp_org_tool_calls_total")
	require.NoError(t, err)

	return tfeclient.MetricsConfig{
		Enabled:               true,
		ServiceName:           "terraform-mcp-server",
		ServiceVersion:        "test-version",
		MeterProvider:         provider,
		ToolCounter:           toolCounter,
		ErrorCounter:          errorCounter,
		ToolCallLatencyBucket: latencyHistogram,
		ClientTypeCounter:     clientTypeCounter,
		OrgToolCallCounter:    orgToolCallCounter,
	}, reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &resourceMetrics))
	return resourceMetrics
}

func findInt64SumMetric(t *testing.T, resourceMetrics metricdata.ResourceMetrics, name string) (metricdata.Sum[int64], bool) {
	t.Helper()

	for _, scope := range resourceMetrics.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[int64])
			require.Truef(t, ok, "metric %s was not an int64 sum", name)
			return data, true
		}
	}
	return metricdata.Sum[int64]{}, false
}

func newCallToolRequest(t *testing.T, toolName string, session *mcp.ServerSession) *mcp.CallToolRequest {
	t.Helper()

	argsJSON, err := json.Marshal(map[string]any{})
	require.NoError(t, err)

	return &mcp.CallToolRequest{
		Session: session,
		Params: &mcp.CallToolParamsRaw{
			Name:      toolName,
			Arguments: argsJSON,
		},
	}
}

// newTestServerSession spins up a real *mcp.ServerSession, pre-seeded with
// InitializeParams, without doing a full client/server handshake. Metrics
// only reads session.InitializeParams(), so this is enough to exercise the
// client-info recording path.
func newTestServerSession(t *testing.T, clientInfo *mcp.Implementation) *mcp.ServerSession {
	t.Helper()

	transport, _ := mcp.NewInMemoryTransports()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.0"}, nil)

	session, err := srv.Connect(context.Background(), transport, &mcp.ServerSessionOptions{
		State: &mcp.ServerSessionState{
			InitializeParams:  &mcp.InitializeParams{ClientInfo: clientInfo},
			InitializedParams: &mcp.InitializedParams{},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return session
}

func TestMetricsMiddleware_NoopWhenDisabled(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.CallToolResult{}, nil
	}

	handler := Metrics(tfeclient.MetricsConfig{Enabled: false}, logger)(next)
	request := newCallToolRequest(t, "test_tool", nil)

	_, err := handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	assert.True(t, nextCalled, "next should still run even when metrics are disabled")
}

func TestMetricsMiddleware_IgnoresNonToolCalls(t *testing.T) {
	metricsConfig, reader := newTestMetricsConfig(t)
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	nextCalled := false
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		nextCalled = true
		return &mcp.ListToolsResult{}, nil
	}

	handler := Metrics(metricsConfig, logger)(next)
	_, err := handler(context.Background(), "tools/list", &mcp.ListToolsRequest{})
	require.NoError(t, err)
	assert.True(t, nextCalled)

	resourceMetrics := collectMetrics(t, reader)
	_, found := findInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	assert.False(t, found, "non-tools/call methods should not record tool-call metrics")
}

func TestMetricsMiddleware_RecordsToolCallAndLatency(t *testing.T) {
	metricsConfig, reader := newTestMetricsConfig(t)
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		time.Sleep(time.Millisecond) // ensure measurable, non-zero latency
		return &mcp.CallToolResult{}, nil
	}

	handler := Metrics(metricsConfig, logger)(next)
	request := newCallToolRequest(t, "list_workspaces", nil)

	result, err := handler(context.Background(), "tools/call", request)
	require.NoError(t, err)
	callResult, ok := result.(*mcp.CallToolResult)
	require.True(t, ok)
	assert.False(t, callResult.IsError)

	resourceMetrics := collectMetrics(t, reader)

	toolCalls, found := findInt64SumMetric(t, resourceMetrics, "mcp_tool_calls_total")
	require.True(t, found)
	require.Len(t, toolCalls.DataPoints, 1)
	assert.EqualValues(t, 1, toolCalls.DataPoints[0].Value)

	errorCalls, found := findInt64SumMetric(t, resourceMetrics, "mcp_tool_errors_total")
	if found {
		assert.Empty(t, errorCalls.DataPoints, "no errors should be recorded for a successful call")
	}
}

func TestMetricsMiddleware_RecordsErrorResult(t *testing.T) {
	metricsConfig, reader := newTestMetricsConfig(t)
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{IsError: true}, nil
	}

	handler := Metrics(metricsConfig, logger)(next)
	request := newCallToolRequest(t, "failing_tool", nil)

	_, err := handler(context.Background(), "tools/call", request)
	require.NoError(t, err)

	resourceMetrics := collectMetrics(t, reader)
	errorCalls, found := findInt64SumMetric(t, resourceMetrics, "mcp_tool_errors_total")
	require.True(t, found)
	require.Len(t, errorCalls.DataPoints, 1)
	assert.EqualValues(t, 1, errorCalls.DataPoints[0].Value)
}

func TestMetricsMiddleware_RecordsErrorWhenNextReturnsErr(t *testing.T) {
	metricsConfig, reader := newTestMetricsConfig(t)
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return nil, assert.AnError
	}

	handler := Metrics(metricsConfig, logger)(next)
	request := newCallToolRequest(t, "erroring_tool", nil)

	_, err := handler(context.Background(), "tools/call", request)
	require.Error(t, err)

	resourceMetrics := collectMetrics(t, reader)
	errorCalls, found := findInt64SumMetric(t, resourceMetrics, "mcp_tool_errors_total")
	require.True(t, found)
	require.Len(t, errorCalls.DataPoints, 1)
}

func TestMetricsMiddleware_RecordsClientTypeFromSession(t *testing.T) {
	metricsConfig, reader := newTestMetricsConfig(t)
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	session := newTestServerSession(t, &mcp.Implementation{
		Name:        "vscode",
		Version:     "1.2.3",
		Title:       "VS Code",
		Description: "VS Code MCP client",
	})

	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	}

	handler := Metrics(metricsConfig, logger)(next)
	request := newCallToolRequest(t, "list_workspaces", session)

	_, err := handler(context.Background(), "tools/call", request)
	require.NoError(t, err)

	resourceMetrics := collectMetrics(t, reader)
	clientTypes, found := findInt64SumMetric(t, resourceMetrics, "mcp_client_type_total")
	require.True(t, found)
	require.Len(t, clientTypes.DataPoints, 1)

	attrs := clientTypes.DataPoints[0].Attributes.ToSlice()
	assert.Contains(t, attrs, attribute.String("client.name", "vscode"))
	assert.Contains(t, attrs, attribute.String("client.version", "1.2.3"))
	assert.Contains(t, attrs, attribute.String("client.title", "VS Code"))
	assert.Contains(t, attrs, attribute.String("client.description", "VS Code MCP client"))
}

// orgCallCounts returns the mcp_org_tool_calls_total value per terraform.org.name.
// The second result reports whether the instrument exported anything at all.
func orgCallCounts(t *testing.T, reader *sdkmetric.ManualReader) (map[string]int64, bool) {
	t.Helper()

	sum, found := findInt64SumMetric(t, collectMetrics(t, reader), "mcp_org_tool_calls_total")
	if !found {
		return nil, false
	}
	counts := map[string]int64{}
	for _, dp := range sum.DataPoints {
		org, ok := dp.Attributes.Value(attribute.Key(tfeclient.OrgAttributeKey))
		require.True(t, ok, "every data point must carry %s", tfeclient.OrgAttributeKey)
		counts[org.AsString()] += dp.Value
	}
	return counts, true
}

func callToolWithArgs(t *testing.T, handler mcp.MethodHandler, args map[string]any) (mcp.Result, error) {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)
	return handler(context.Background(), "tools/call", &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: "list_workspaces", Arguments: argsJSON},
	})
}

// An org is counted when the call succeeds, and not when the result is an error
// (IsError result, or a protocol-level error). Each case first makes a successful
// control call on another org, so a missing series for "acme" can only mean
// "not counted" and never "the counter is nil".
func TestMetricsMiddleware_CountsOrgOnSuccessNotOnError(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	tests := []struct {
		name   string
		result mcp.Result
		err    error
		want   map[string]int64
	}{
		{name: "success is counted", result: &mcp.CallToolResult{}, want: map[string]int64{"control-org": 1, "acme": 1}},
		{name: "IsError result is not counted", result: &mcp.CallToolResult{IsError: true}, want: map[string]int64{"control-org": 1}},
		{name: "protocol error is not counted", err: assert.AnError, want: map[string]int64{"control-org": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metricsConfig, reader := newTestMetricsConfig(t)

			ok := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return &mcp.CallToolResult{}, nil
			}
			_, err := callToolWithArgs(t, Metrics(metricsConfig, logger)(ok), map[string]any{"terraform_org_name": "control-org"})
			require.NoError(t, err)

			underTest := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				return tt.result, tt.err
			}
			_, _ = callToolWithArgs(t, Metrics(metricsConfig, logger)(underTest), map[string]any{"terraform_org_name": "acme"})

			counts, found := orgCallCounts(t, reader)
			require.True(t, found)
			assert.Equal(t, tt.want, counts)
		})
	}
}

func TestOrgNameFromArguments(t *testing.T) {
	tests := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{name: "org present", raw: json.RawMessage(`{"terraform_org_name":"acme","workspace_name":"w"}`), want: "acme"},
		{name: "nil arguments", raw: nil, want: ""},
		{name: "empty arguments", raw: json.RawMessage(``), want: ""},
		{name: "empty object", raw: json.RawMessage(`{}`), want: ""},
		{name: "org absent", raw: json.RawMessage(`{"run_id":"run-1"}`), want: ""},
		{name: "invalid JSON", raw: json.RawMessage(`{not json`), want: ""},
		{name: "truncated JSON", raw: json.RawMessage(`{"terraform_org_name":"acme"`), want: ""},
		{name: "JSON that is not an object", raw: json.RawMessage(`["acme"]`), want: ""},
		{name: "org of the wrong type", raw: json.RawMessage(`{"terraform_org_name":5}`), want: ""},
		// Go decodes "acme" first and only then fails on the second key, so only the
		// err check keeps malformed input from yielding an org
		{name: "duplicate key with a wrong type", raw: json.RawMessage(`{"terraform_org_name":"acme","terraform_org_name":5}`), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, orgNameFromArguments(tt.raw))
		})
	}
}
