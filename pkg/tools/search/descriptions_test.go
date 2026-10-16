// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchDescriptionsOwnToolLocalContracts(t *testing.T) {
	tests := []struct {
		name        string
		description string
		required    []string
	}{
		{"provider_list_schema_list", ProviderListSchemaListDescription, []string{"organization_name", "workspace_name", "catalog", "generate_query_configuration", "target managed-resource"}},
		{"generate_query_configuration", GenerateQueryConfigurationDescription, []string{"does not execute the query", "provider_list_schema_list", "exact schema key", "execute_query", "cannot be used as a query-time filter", "generate_config_out=true", "does not guarantee capture"}},
		{"execute_query", ExecuteQueryDescription, []string{"QueryRun", "exact list_resource_schemas key", "query_run_id", "get_query_status", "do not blindly execute", "absent from the list-resource schema"}},
		{"get_query_status", GetQueryStatusDescription, []string{"bounded call", "same ID", "get_query_summary", "Terminal status does not imply selectable"}},
		{"get_query_summary", GetQuerySummaryDescription, []string{"presentation filters", "query_result_marker", "Search-only intent", "target workspace or local path", "never select a first-N subset", "generate_config_out", "does not guarantee capture", "new markers", "does not expose arbitrary resource attributes"}},
		{"prepare_import", PrepareImportDescription, []string{"does not read", ".terraform.lock.hcl", "lock file is missing", "ambiguous, alert the user", "do not run terraform init", "substitute Search catalog versions", "wait for further instructions", "namespace/type shorthand is accepted", "custom/private sources require", "canonicalizes sources", "unchanged", "import_context", "Search list types do not prove managed types", "separate approval"}},
		{"verify_import_plan", VerifyImportPlanDescription, []string{"one exact ordinary", "search for or substitute", "change and object_identity are independent", "get_plan_json_output", "continue to terraform apply"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, fragment := range test.required {
				require.Contains(t, test.description, fragment)
			}
			for _, forbidden := range []string{"candidate_id", "next_calls", "create_import_cv", "create_import_run", "get_import_configuration_download"} {
				require.NotContains(t, test.description, forbidden)
			}
		})
	}
}
