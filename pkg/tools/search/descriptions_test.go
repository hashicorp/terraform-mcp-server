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
		{"provider_list_schema_list", ProviderListSchemaListDescription, []string{"terraform_org_name", "workspace_name", "catalog", "generate_query_configuration", "target managed-resource"}},
		{"generate_query_configuration", GenerateQueryConfigurationDescription, []string{"schema-backed guidance", "does not execute", "exact schema key", "nesting/cardinality", "Required schema attributes remain required"}},
		{"execute_query", ExecuteQueryDescription, []string{"EXECUTION APPROVAL", "wait for their approval", "server does not enforce a separate approval parameter", "query_run_id", "reconcile", "does not create or modify cloud"}},
		{"get_query_status", GetQueryStatusDescription, []string{"bounded call", "retry_after_seconds", "same ID", "do not execute another query", "Terminal status does not imply selectable results"}},
		{"prepare_import", PrepareImportDescription, []string{"explicitly selected", "existing local configuration directory", ".terraform.lock.hcl", "do not run terraform init", "does not read", "unchanged", "not import IDs or authorization", "separate approval"}},
		{"generate_query_configuration_source", GenerateQueryConfigurationDescription, []string{"does not execute the query", "provider_list_schema_list", "exact schema key", "execute_query", "cannot be used as a query-time filter", "generate_config_out=true", "does not guarantee capture"}},
		{"execute_query_source", ExecuteQueryDescription, []string{"QueryRun", "exact list_resource_schemas key", "query_run_id", "get_query_status", "do not blindly execute", "absent from the list-resource schema"}},
		{"get_query_status_source", GetQueryStatusDescription, []string{"bounded call", "same ID", "get_query_summary", "Terminal status does not imply selectable"}},
		{"get_query_summary", GetQuerySummaryDescription, []string{"presentation filters", "query_result_marker", "Search-only intent", "target workspace or local path", "never select a first-N subset", "generate_config_out", "does not guarantee capture", "new markers", "does not expose arbitrary resource attributes"}},
		{"prepare_import_source", PrepareImportDescription, []string{"does not read", ".terraform.lock.hcl", "lock file is missing", "ambiguous, alert the user", "do not run terraform init", "substitute Search catalog versions", "wait for further instructions", "namespace/type shorthand is accepted", "custom/private sources require", "canonicalizes sources", "unchanged", "import_context", "Search list types do not prove managed types", "separate approval"}},
		{"verify_import_plan", VerifyImportPlanDescription, []string{"one exact ordinary", "search for or substitute", "change and object_identity are independent", "get_plan_json_output", "continue to terraform apply"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, fragment := range test.required {
				require.Contains(t, test.description, fragment)
			}
		})
	}
}
