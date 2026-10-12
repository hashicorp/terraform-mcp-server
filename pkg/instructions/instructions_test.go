// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package instructions

import (
	"strings"
	"testing"
)

func TestSearchWorkflowInstructions(t *testing.T) {
	for _, required := range []string{
		"Terraform Search (when Search tools are available)",
		"provider_list_schema_list", "generate_query_configuration", "execute_query",
		"get_query_status", "get_query_summary", "query_run_id",
		"Search-only requests end here", "existing local configuration path",
		"query_result_marker", "neither import IDs nor proof",
		"only when `prepare_import` and `verify_import_plan` are available",
		"speculative `terraform plan`", "full HCP Plan separately",
		"no `-out` or apply", "CLI Run URL", "Follow `prepare_import`'s description",
		"Approval for a plan is not approval to apply",
	} {
		if !strings.Contains(Text, required) {
			t.Errorf("missing workflow guidance %q", required)
		}
	}
	for _, obsolete := range []string{
		"get_import_configuration_download", "create_import_cv", "create_import_run",
		"latest-query-run", "data.relationships", "download archive",
	} {
		if strings.Contains(Text, obsolete) {
			t.Errorf("shared instructions include obsolete Search handoff %q", obsolete)
		}
	}
}
