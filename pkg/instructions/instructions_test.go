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

	generalGuidance := Text[:searchStart]
	searchGuidance := Text[searchStart:importStart]
	importGuidance := Text[importStart:codeGenerationStart]

	for _, required := range []string{
		"supersede the generic Tool Usage Guidelines and Code Generation guidance",
		"use the provider versions and schemas returned by the Search tools",
		"do not replace them with a Registry latest-version lookup",
		"Search catalog version is source-query evidence, not a target-version recommendation",
		"Search-only intent, report the summary and stop",
		"Do not request an import target or local path",
	} {
		require.Contains(t, searchGuidance, required)
	}

	for _, required := range []string{
		"exact locked provider releases",
		"provider source and exact version relevant to the selected imports",
		"public namespace/type shorthand is accepted",
		"custom/private sources retain their hostname",
		"Use the target's locked releases, not the Search catalog versions",
		"alert the user that exact target provider releases cannot be determined",
		"do not run `terraform init`",
		"or call `prepare_import`, and wait for further instructions",
		"documentation applicable to the locked release",
		"do not use a latest-version lookup to replace or upgrade that selection",
		"Format and, when appropriate, validate the authored HCL",
		"diagnostics and do not authorize a Plan",
		"Obtain separate approval for one plain HCP-connected speculative `terraform plan`",
		"Poll the same Run ID",
		"never guess the latest Run",
		"does not continue to `terraform apply`, `terraform import` or `apply_run`",
	} {
		require.Contains(t, importGuidance, required)
	}

	require.NotContains(t, generalGuidance, "Search catalog version")
	require.NotContains(t, generalGuidance, "Search-to-import speculative")
}

func TestEmbeddedInstructionsPreserveGeneralWorkflowGuidance(t *testing.T) {
	searchStart := strings.Index(Text, "**Terraform Search (")
	require.GreaterOrEqual(t, searchStart, 0)
	generalGuidance := Text[:searchStart]

	for _, required := range []string{
		"**BEFORE generating any Terraform code**",
		"Query registries for latest provider/module versions and styling guidelines",
		"search private registries first, then fall back to public",
		"All modules in a project must use compatible provider versions",
		"Verify with get_provider_details before generating code",
		"Run terraform validate immediately after generation",
		"then terraform plan only if validation passes",
		"Use terraform fmt to format code as needed",
		"ALWAYS get explicit yes/no confirmation before",
		"ensure generated code uses current versions and follows best practices",
	} {
		require.Contains(t, generalGuidance, required)
	}
}

func TestEmbeddedInstructionsContainNoV0SearchWorkflow(t *testing.T) {
	for _, forbidden := range []string{
		"get_import_configuration_download", "create_import_cv", "create_import_run",
		"candidate_id", "next_calls", "empty authoring directory",
	} {
		require.NotContains(t, Text, forbidden)
	}
}
