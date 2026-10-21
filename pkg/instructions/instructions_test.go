// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package instructions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedInstructionsPreserveDiscoveryAndGuidanceBoundaries(t *testing.T) {
	searchStart := strings.Index(Text, "**Terraform Search (")
	codeGenerationStart := strings.Index(Text, "**Code Generation**")
	require.GreaterOrEqual(t, searchStart, 0)
	require.Greater(t, codeGenerationStart, searchStart)
	guidance := Text[searchStart:codeGenerationStart]
	for _, required := range []string{
		"`provider_list_schema_list`",
		"valid Terraform credentials",
		"`TF_CLOUD_ORGANIZATION` and `TF_MCP_QUERY_WORKSPACE`",
		"`TF_WORKSPACE`",
		"proposed defaults—not authorization",
		"reuse the accepted `terraform_org_name` and `workspace_name` explicitly",
		"`provider_namespace` and `provider_name` together",
		"Do not replace them with `get_latest_provider_version`",
		"`generate_query_configuration` transforms a supplied schema without Terraform credentials or backend calls",
		"exact `provider_namespace`, `provider_name`, and `provider_version`",
		"bare schema map or a wrapper",
		"comma-separated exact schema keys",
		"nesting modes/cardinality, required values",
		"Never invent unsupported filters or omit required values",
		"Guidance is not backend validation",
		"Stop at reviewed configuration guidance",
		"Do not request an import target or local path",
		"Do not create queries or modify infrastructure",
	} {
		require.Contains(t, guidance, required)
	}
	for _, unavailable := range []string{
		"execute_query", "get_query_status", "get_query_summary",
		"prepare_import", "verify_import_plan", "TF_MCP_IMPORT_WORKSPACE",
	} {
		require.NotContains(t, Text, unavailable)
	}
	require.NotContains(t, guidance, "`organization_name`")
}
