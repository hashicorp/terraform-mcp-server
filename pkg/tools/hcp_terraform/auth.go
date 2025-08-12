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
