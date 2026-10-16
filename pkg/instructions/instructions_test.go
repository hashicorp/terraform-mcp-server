// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package instructions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedInstructionsPreserveSearchImportBoundaries(t *testing.T) {
	for _, required := range []string{
		"Search-only intent, report the summary and stop",
		"Do not request an import target or local path",
		"exact `.terraform.lock.hcl` selections",
		"separately approved plain HCP-connected speculative Plan",
		"Poll the same Run ID",
		"never guess the latest Run",
		"does not continue to `terraform apply`, `terraform import` or `apply_run`",
	} {
		require.Contains(t, Text, required)
	}
	require.NotContains(t, Text, "Run terraform validate immediately after generation")
	require.NotContains(t, Text, "ensure generated code uses current versions")
	require.Less(t, strings.Index(Text, "**Terraform Search ("), strings.Index(Text, "**Search-to-import ("))
}

func TestEmbeddedInstructionsContainNoV0SearchWorkflow(t *testing.T) {
	for _, forbidden := range []string{
		"get_import_configuration_download", "create_import_cv", "create_import_run",
		"candidate_id", "next_calls", "empty authoring directory",
	} {
		require.NotContains(t, Text, forbidden)
	}
}
