// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package otelmetrics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// ServiceInstanceIDEnv overrides the service.instance.id resource attribute
// The Helm chart sets it from the pod UID
const ServiceInstanceIDEnv = "OTEL_INSTANCE_ID"

// Setup reads metrics config from the environment and, if metrics are
// enabled, initializes the OTel exporter and meter provider. It always
// returns a shutdown func; the func does nothing when metrics are disabled.
func Setup(logger *log.Logger) (client.MetricsConfig, func()) {
	metricsConfig := client.LoadMetricsConfigFromEnv(logger)
	logger.Infof("Metrics enabled: %t endpoint: %s exportInterval: %s", metricsConfig.Enabled, metricsConfig.Endpoint, metricsConfig.ExportInterval)
	if !metricsConfig.Enabled {
		return metricsConfig, func() {}
	}

	// Context for metrics is for tracking the lifecycle of the metrics setup and shutdown, not tied to individual tool calls.
	ctxMetrics := context.Background()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Errorf("OTel Internal Error: %v", err)
	}))

	shutdown, err := Init(ctxMetrics, &metricsConfig, logger)
	if err != nil {
		logger.Errorf("Failed to initialize metrics: %v", err)
		return metricsConfig, func() {}
	}

	return metricsConfig, shutdown
}

// Init builds the OTel exporter, meter provider, and the tool/error/latency/
// client-type counters used to record metrics, storing them on config.
func Init(ctx context.Context, config *client.MetricsConfig, logger *log.Logger) (func(), error) {
	logger.Infof("Initializing exporter and meter provider for OTel metrics...")
	// Create the Exporter (Sends data to the Collector)
	// exporter, err := stdoutmetric.New() // For stdio debugging
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(config.Endpoint), otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
	}
	// Create the MeterProvider with a PeriodicReader
	// The reader flushes metrics to the exporter periodically
	resourceAttrs := newResource(config)

	config.MeterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(config.ExportInterval))),
		sdkmetric.WithResource(resourceAttrs),
	)

	// Set it as the global provider
	otel.SetMeterProvider(config.MeterProvider)

	meter := config.MeterProvider.Meter(config.ServiceName)

	config.ToolCounter, err = meter.Int64Counter("mcp_tool_calls_total")
	if err != nil {
		return nil, fmt.Errorf("failed to create tool counter: %w", err)
	}

	config.ErrorCounter, err = meter.Int64Counter("mcp_tool_errors_total",
		metric.WithDescription("Total number of failed tool calls"))
	if err != nil {
		return nil, fmt.Errorf("failed to create error counter: %w", err)
	}

	config.ToolCallLatencyBucket, err = meter.Float64Histogram("mcp_tool_duration_seconds",
		metric.WithDescription("Duration of tool calls in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		return nil, fmt.Errorf("failed to create latency histogram: %w", err)
	}

	config.ClientTypeCounter, err = meter.Int64Counter("mcp_client_type_total",
		metric.WithDescription("Total number of connections by client type"))
	if err != nil {
		return nil, fmt.Errorf("failed to create client type counter: %w", err)
	}

	config.OrgToolCallCounter, err = meter.Int64Counter("mcp_org_tool_calls_total",
		metric.WithDescription("Successful tool calls per Terraform organization, counted only for tools that take a terraform_org_name argument. "+
			"Sum by terraform.org.name for calls per organization; count distinct terraform.org.name for the number of organizations"))
	if err != nil {
		return nil, fmt.Errorf("failed to create org tool call counter: %w", err)
	}

	return func() {
		logger.Infof("Shutting down metrics exporter..")
		if err := config.MeterProvider.Shutdown(ctx); err != nil {
			logger.Errorf("Error shutting down meter provider: %v", err)
		}
	}, nil
}

func newResource(config *client.MetricsConfig) *resource.Resource {
	return resource.NewSchemaless(
		attribute.String("service.name", config.ServiceName),
		attribute.String("service.version", config.ServiceVersion),
		attribute.String("service.instance.id", serviceInstanceID()),
	)
}

// serviceInstanceID returns OTEL_INSTANCE_ID if set (the Helm chart sets it to
// the pod UID), otherwise the hostname (the pod name in Kubernetes), otherwise
// a random ID, so two replicas never share one.
func serviceInstanceID() string {
	if id := strings.TrimSpace(os.Getenv(ServiceInstanceIDEnv)); id != "" {
		return id
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
