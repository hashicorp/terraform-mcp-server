// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package prompts provides MCP prompt resources for the Terraform MCP server.
// It includes embedded prompt templates and functions to register them with the MCP server.
package prompts

import _ "embed"

// WorkspaceAnalysisPrompt contains the embedded workspace analysis prompt template.
// This prompt is used for comprehensive workspace analysis and details retrieval.
//go:embed workspace_analysis.md
var WorkspaceAnalysisPrompt string

// GetWorkspaceAnalysisPrompt returns the workspace analysis prompt content.
// This function provides access to the embedded workspace analysis prompt template
// that can be used by MCP clients for workspace analysis operations.
func GetWorkspaceAnalysisPrompt() string {
	return WorkspaceAnalysisPrompt
}
