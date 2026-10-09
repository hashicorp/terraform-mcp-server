// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"encoding/json"
)

// selectImportCandidates resolves selected IDs against an already parsed
// discovery, failing closed on an unknown or overlapping identity.
func selectImportCandidates(discovery *importDiscovery, selections []importSelection, workspaceID string) ([]importDiscoveryCandidate, error) {
	if discovery.WorkspaceID != workspaceID {
		return nil, importEvidenceFailure("query_workspace_mismatch")
	}
	byID := make(map[string]importDiscoveryCandidate, len(discovery.Candidates))
	for _, candidate := range discovery.Candidates {
		byID[candidate.CandidateID] = candidate
	}
	selected := make([]importDiscoveryCandidate, 0, len(selections))
	seenIdentity := make(map[string]bool, len(selections))
	for _, selection := range selections {
		candidate, ok := byID[selection.CandidateID]
		if !ok {
			return nil, importEvidenceFailure("selected_candidate_not_found")
		}
		// Defense in depth: parsing already rejects a QueryRun with an invalid
		// version, but a candidate without one must never reach a carry block.
		if candidate.IdentityVersion == nil || *candidate.IdentityVersion < 0 {
			return nil, importEvidenceFailure("query_identity_version_invalid")
		}
		identity, err := json.Marshal(candidate.Identity)
		if err != nil {
			return nil, importEvidenceFailure("query_identity_invalid")
		}
		identityKey := candidate.Provider.Source + "/" + candidate.Provider.Version + "/" + string(identity)
		if seenIdentity[identityKey] {
			return nil, importEvidenceFailure("selected_identity_overlap")
		}
		seenIdentity[identityKey] = true
		selected = append(selected, candidate)
	}
	return selected, nil
}
