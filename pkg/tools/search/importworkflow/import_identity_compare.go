// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"encoding/json"
)

// Only exact, complete primitive identities can be compared without inventing
// cross-provider conversion rules. Importing.identity is deliberately ignored:
// it can echo the agent's HCL, whereas after_identity comes from the provider.
func importCompareProviderIdentity(source map[string]any, after json.RawMessage) (string, string) {
	if len(source) == 0 || len(after) == 0 || bytes.Equal(after, []byte("null")) {
		return "unverified", "provider_returned_identity_unavailable"
	}
	var providerIdentity map[string]any
	if err := decodeImportEvidenceJSONLimit(after, &providerIdentity, maxImportSchemaBytes); err != nil || len(providerIdentity) != len(source) {
		return "unverified", "identity_shape_not_comparable"
	}
	// Check the whole shape before declaring any differing value a mismatch;
	// provider versions can define superficially similar but incompatible keys.
	for key, value := range source {
		actual, ok := providerIdentity[key]
		if !ok || value == nil || actual == nil {
			return "unverified", "identity_shape_not_comparable"
		}
		switch value.(type) {
		case string:
			if _, ok := actual.(string); !ok {
				return "unverified", "identity_shape_not_comparable"
			}
		case bool:
			if _, ok := actual.(bool); !ok {
				return "unverified", "identity_shape_not_comparable"
			}
		default:
			// Composite or numeric values need a type-aware, versioned contract.
			return "unverified", "identity_type_not_comparable"
		}
	}
	for key, value := range source {
		if value != providerIdentity[key] {
			return "mismatched", "provider_returned_identity_differs"
		}
	}
	return "matched", "complete_provider_returned_identity_matches_query_values; verify_provider_scope_separately"
}
