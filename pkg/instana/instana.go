// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package instana

import (
	"os"

	instanasdk "github.com/instana/go-sensor"
	log "github.com/sirupsen/logrus"
)

// Setup initializes the Instana collector when INSTANA_ENABLED is set. Once
// initialized, application metrics (CPU, memory, goroutines) are collected
// automatically.
func Setup(logger *log.Logger) instanasdk.TracerLogger {
	if os.Getenv("INSTANA_ENABLED") != "true" {
		return nil
	}
	serviceName := "terraform-mcp-server"
	if n := os.Getenv("INSTANA_SERVICE_NAME"); n != "" {
		serviceName = n
	}
	logger.Info("Instana instrumentation enabled")
	return instanasdk.InitCollector(&instanasdk.Options{
		Service: serviceName,
		Tracer:  instanasdk.DefaultTracerOptions(),
	})
}
