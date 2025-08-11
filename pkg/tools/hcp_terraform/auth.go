// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import (
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// resolveToken implements secure token resolution with proper precedence
func resolveToken(request mcp.CallToolRequest) (string, error) {
	// 1. Check environment variable first (highest precedence)
	if token := os.Getenv("HCP_TERRAFORM_TOKEN"); token != "" {
		return token, nil
	}

	// 2. Check tool request parameters
	if authStr := request.GetString("authorization", ""); authStr != "" {
		// Handle both "Bearer <token>" and raw token formats
		if strings.HasPrefix(authStr, "Bearer ") {
			return strings.TrimPrefix(authStr, "Bearer "), nil
		}
		return authStr, nil
	}

	// 3. No token found
	return "", fmt.Errorf("HCP Terraform token required: provide via HCP_TERRAFORM_TOKEN environment variable or authorization parameter")
}

// validateTokenFormat performs basic token format validation
func validateTokenFormat(token string) error {
	if len(token) == 0 {
		return fmt.Errorf("token cannot be empty")
	}

	// HCP Terraform tokens can have various formats:
	// - User tokens: start with "user-"
	// - Team tokens: start with "team-" or have format like "xxxxx.atlasv1.xxxxx"
	// - Organization tokens: start with "org-"
	validPrefixes := []string{"user-", "team-", "org-"}
	hasValidPrefix := false

	// Check for standard prefixes
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(token, prefix) {
			hasValidPrefix = true
			break
		}
	}

	// Also check for atlasv1 format tokens (team tokens can have this format)
	if !hasValidPrefix && strings.Contains(token, ".atlasv1.") {
		hasValidPrefix = true
	}

	if !hasValidPrefix {
		return fmt.Errorf("token format appears invalid: expected token to start with user-, team-, org- or contain .atlasv1")
	}

	return nil
}
