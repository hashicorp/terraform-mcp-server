// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package orchestrator

import (
	"fmt"
	"os"
	"time"
)

// resolveToken extracts authentication token from request or environment
func resolveToken(request *WorkspaceAnalysisRequest) (string, error) {
	if request.Authorization != "" {
		return request.Authorization, nil
	}

	if token := os.Getenv("HCP_TERRAFORM_TOKEN"); token != "" {
		return token, nil
	}

	return "", fmt.Errorf("no authentication token provided")
}

// Helper functions for safe type conversion
func getStringValue(data map[string]interface{}, key string) string {
	if value, ok := data[key].(string); ok {
		return value
	}
	return ""
}

func getBoolValue(data map[string]interface{}, key string) bool {
	if value, ok := data[key].(bool); ok {
		return value
	}
	return false
}

func getIntValue(data map[string]interface{}, key string) int {
	if value, ok := data[key].(float64); ok {
		return int(value)
	}
	if value, ok := data[key].(int); ok {
		return value
	}
	return 0
}

func parseTimeValue(timeStr string) time.Time {
	if timeStr == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t
	}
	return time.Time{}
}
