// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package instructions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedInstructionsPreserveSearchImportBoundaries(t *testing.T) {
	searchStart := strings.Index(Text, "**Terraform Search (")
	importStart := strings.Index(Text, "**Search-to-import (")
	codeGenerationStart := strings.Index(Text, "**Code Generation**")
	require.GreaterOrEqual(t, searchStart, 0)
	require.Greater(t, importStart, searchStart)
	require.Greater(t, codeGenerationStart, importStart)

	searchGuidance := Text[searchStart:importStart]
	importGuidance := Text[importStart:codeGenerationStart]

	for _, required := range []string{
		"`TF_CLOUD_ORGANIZATION` and `TF_MCP_QUERY_WORKSPACE`",
		"proposed defaults—not authorization",
		"reuse the accepted `terraform_org_name` and `workspace_name` explicitly",
		"Source confirmation does not approve query execution",
		"Do not replace them with `get_latest_provider_version`",
		"Stop for Search-only intent",
		"Do not request an import target or local path",
	} {
		require.Contains(t, searchGuidance, required)
	}

	for _, required := range []string{
		"Confirm selection and target independently",
		"`TF_CLOUD_ORGANIZATION` and `TF_MCP_IMPORT_WORKSPACE`",
		"never use `TF_MCP_QUERY_WORKSPACE` as the import target",
		"Query-source confirmation never confirms the import target",
		"Use the target's locked releases, not Search catalog versions",
		"Do not run `terraform init`",
		"Obtain separate approval for one plain HCP-connected speculative `terraform plan`",
		"Do not continue automatically to `terraform apply`, `terraform import` or `apply_run`",
	} {
		require.Contains(t, importGuidance, required)
	}

	require.NotContains(t, searchGuidance, "TF_MCP_IMPORT_WORKSPACE")
	for _, guidance := range []string{searchGuidance, importGuidance} {
		require.NotContains(t, guidance, "input_required")
		require.NotContains(t, guidance, "`organization_name`")
	}
}
