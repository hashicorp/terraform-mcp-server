package client

import (
	"context"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type MetricsConfig struct {
	Enabled               bool
	Endpoint              string                   // URL of your OTel Collector or backend
	ExportInterval        time.Duration            // Controls the frequency of metric flushes
	ServiceName           string                   // ServiceName identifies the source of the metrics (e.g., "terraform-mcp-server")
	ServiceVersion        string                   // ServiceVersion helps track metrics across different deployments
	MeterProvider         *sdkmetric.MeterProvider // MeterProvider is the OTel provider used to create instruments
	Attributes            []attribute.KeyValue     // Attributes are global labels applied to every metric emitted
	EnableRuntimeMetrics  bool                     // EnableRuntimeMetrics toggles the collection of Go runtime stats (GC, Memory)
	ToolCounter           metric.Int64Counter      // ToolCounter tracks the total number of tool calls initiated
	ErrorCounter          metric.Int64Counter      // Error count
	ToolCallLatencyBucket metric.Float64Histogram  // Latency distribution
}

func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled:              false,
		Endpoint:             "localhost:4318",
		ExportInterval:       2 * time.Second,
		ServiceName:          "terraform-mcp-server",
		ServiceVersion:       "latest",
		MeterProvider:        nil,
		Attributes:           []attribute.KeyValue{},
		EnableRuntimeMetrics: true,
	}
}

func LoadMetricsConfigFromEnv() MetricsConfig {
	config := DefaultMetricsConfig()
	if endpoint := os.Getenv("MCP_METRICS_ENDPOINT"); endpoint != "" {
		config.Endpoint = endpoint
		log.Infof("Using env value for MCP_METRICS_ENDPOINT: %s", endpoint)
	} else {
		log.Infof("MCP_METRICS_ENDPOINT not set in env, using default: %s", config.Endpoint)
	}
	if interval := os.Getenv("MCP_METRICS_EXPORT_INTERVAL"); interval != "" {
		if dur, err := time.ParseDuration(interval); err == nil {
			config.ExportInterval = dur
			log.Infof("Using env value for MCP_METRICS_EXPORT_INTERVAL: %s", interval)
		} else {
			log.Warnf("Error parsing MCP_METRICS_EXPORT_INTERVAL: %v", err)
			log.Infof("Using default export interval: %s", config.ExportInterval)
		}
	} else {
		log.Infof("MCP_METRICS_EXPORT_INTERVAL not set in env, using default: %s", config.ExportInterval)
	}
	if serviceName := os.Getenv("MCP_METRICS_SERVICE_NAME"); serviceName != "" {
		config.ServiceName = serviceName
		log.Infof("Using env value for MCP_METRICS_SERVICE_NAME: %s", serviceName)
	} else {
		log.Infof("MCP_METRICS_SERVICE_NAME not set in env, using default: %s", config.ServiceName)
	}
	if serviceVersion := os.Getenv("MCP_METRICS_SERVICE_VERSION"); serviceVersion != "" {
		config.ServiceVersion = serviceVersion
		log.Infof("Using env value for MCP_METRICS_SERVICE_VERSION: %s", serviceVersion)
	} else {
		log.Infof("MCP_METRICS_SERVICE_VERSION not set in env, using default: %s", config.ServiceVersion)
	}
	if enabled := os.Getenv("MCP_METRICS_ENABLED"); enabled == "true" {
		config.Enabled = true
		log.Infof("MCP_METRICS_ENABLED set to true in env, enabling metrics")
	} else {
		log.Infof("MCP_METRICS_ENABLED not set in env, using default: %t", config.Enabled)
	}
	return config
}

func RecordToolCall(ctx context.Context, startTime time.Time, toolErr error, id any, message *mcp.CallToolRequest, config MetricsConfig, logger *log.Logger) {
	logger.Infof("Recording tool call for tool: %s id: %v", message.Params.Name, id)
	if !config.Enabled || config.ToolCounter == nil {
		logger.Errorf("DEBUG: Either metrics are not enabled or ToolCounter is NIL! Initialization failed.")
		return
	}
	// Calculate latency
	elapsed := time.Since(startTime).Seconds()

	attrs := metric.WithAttributes(
		attribute.String("tool.name", message.Params.Name),
		attribute.String("service.name", config.ServiceName),
		attribute.String("service.version", config.ServiceVersion),
	)
	// Record tool call count
	config.ToolCounter.Add(ctx, 1, attrs)
	// Record Latency (Histogram)
	config.ToolCallLatencyBucket.Record(ctx, elapsed, attrs)
	// Record errors if any
	if toolErr != nil {
		config.ErrorCounter.Add(ctx, 1, attrs)
		logger.Errorf("Recorded error for tool %s: %v", message.Params.Name, toolErr)
	}
}
