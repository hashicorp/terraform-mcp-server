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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, fragment := range test.required {
				require.Contains(t, test.description, fragment)
			}
		})
	}
}
