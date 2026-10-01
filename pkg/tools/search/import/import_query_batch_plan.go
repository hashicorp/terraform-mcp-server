// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"

	"github.com/hashicorp/go-tfe"
)

type importCandidatePlanFact struct {
	CandidateID    string                 `json:"candidate_id"`
	TargetAddress  string                 `json:"target_address"`
	PlanBinding    string                 `json:"plan_binding"`
	ObjectIdentity string                 `json:"object_identity"`
	IdentityReason string                 `json:"identity_reason,omitempty"`
	Planned        *importPlannedResource `json:"planned,omitempty"`
}

type importBatchPlanFacts struct {
	FormatVersion           string                    `json:"format_version"`
	Candidates              []importCandidatePlanFact `json:"candidates"`
	ResourceChangeCount     int                       `json:"resource_change_count"`
	OtherImports            []importPlannedResource   `json:"other_imports"`
	OtherImportCount        int                       `json:"other_import_count"`
	OtherManagedActions     []importPlannedResource   `json:"other_managed_actions"`
	OtherManagedActionCount int                       `json:"other_managed_action_count"`
	OutputChangeCount       int                       `json:"output_change_count"`
	DriftCount              int                       `json:"drift_count"`
	DriftEntries            []importResourceDrift     `json:"drift_entries"`
	DeferredCount           int                       `json:"deferred_count"`
	Truncated               bool                      `json:"truncated"`
}

func readImportBatchPlanFacts(ctx context.Context, c *tfe.Client, planID string, selections []importSelection, candidates []importDiscoveryCandidate) (*importBatchPlanFacts, error) {
	raw, status, err := readImportBackendJSON(ctx, c, "plans/"+url.PathEscape(planID)+"/json-output", maxImportSchemaBytes)
	if err != nil || status != 200 {
		return nil, importReadError(err, status)
	}
	var plan struct {
		FormatVersion   string `json:"format_version"`
		Complete        *bool  `json:"complete"`
		Errored         bool   `json:"errored"`
		ResourceChanges []struct {
			Address      string `json:"address"`
			Mode         string `json:"mode"`
			Type         string `json:"type"`
			ProviderName string `json:"provider_name"`
			Change       struct {
				Actions       []string        `json:"actions"`
				Importing     json.RawMessage `json:"importing"`
				AfterIdentity json.RawMessage `json:"after_identity"`
			} `json:"change"`
		} `json:"resource_changes"`
		OutputChanges   map[string]json.RawMessage `json:"output_changes"`
		DeferredChanges []json.RawMessage          `json:"deferred_changes"`
		ResourceDrift   []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_drift"`
	}
	if err := decodeImportEvidenceJSONLimit(raw, &plan, maxImportSchemaBytes); err != nil {
		return nil, err
	}
	if !importCompatibleFormat(plan.FormatVersion) || plan.Complete == nil || !*plan.Complete || plan.Errored {
		return nil, importEvidenceFailure("plan_evidence_incomplete")
	}
	if len(plan.ResourceChanges) > 10000 || len(plan.ResourceDrift) > 100 {
		return nil, importEvidenceFailure("plan_resource_limit")
	}
	f := &importBatchPlanFacts{FormatVersion: plan.FormatVersion, ResourceChangeCount: len(plan.ResourceChanges), OutputChangeCount: len(plan.OutputChanges), DeferredCount: len(plan.DeferredChanges), DriftCount: len(plan.ResourceDrift), Candidates: make([]importCandidatePlanFact, len(selections)), OtherImports: []importPlannedResource{}, OtherManagedActions: []importPlannedResource{}, DriftEntries: []importResourceDrift{}}
	selected := make(map[string]int, len(selections))
	for i, s := range selections {
		selected[s.TargetAddress] = i
		f.Candidates[i] = importCandidatePlanFact{CandidateID: s.CandidateID, TargetAddress: s.TargetAddress, PlanBinding: "missing", ObjectIdentity: "unverified", IdentityReason: "no_matching_plan_import"}
	}
	for _, drift := range plan.ResourceDrift {
		if drift.Address == "" || len(drift.Change.Actions) == 0 {
			return nil, importEvidenceFailure("plan_drift_evidence_invalid")
		}
		f.DriftEntries = append(f.DriftEntries, importResourceDrift{Address: drift.Address, Actions: drift.Change.Actions})
	}
	seen := make(map[string]int, len(selections))
	for _, change := range plan.ResourceChanges {
		imported := len(change.Change.Importing) > 0 && !bytes.Equal(change.Change.Importing, []byte("null"))
		var importing struct {
			ID       json.RawMessage `json:"id"`
			Identity json.RawMessage `json:"identity"`
			Unknown  bool            `json:"unknown"`
		}
		if imported {
			if err := decodeImportEvidenceJSONLimit(change.Change.Importing, &importing, maxImportSchemaBytes); err != nil {
				return nil, importEvidenceFailure("plan_import_evidence_invalid")
			}
		}
		idPresent := len(importing.ID) > 0 && !bytes.Equal(importing.ID, []byte("null"))
		identityPresent := len(importing.Identity) > 0 && !bytes.Equal(importing.Identity, []byte("null"))
		item := importPlannedResource{Address: change.Address, Mode: change.Mode, Type: change.Type, ProviderSource: change.ProviderName, Actions: change.Change.Actions, ImportPresent: imported, ImportIDPresent: len(importing.ID) > 0 && !bytes.Equal(importing.ID, []byte("null"))}
		if index, ok := selected[change.Address]; ok {
			seen[change.Address]++
			fact := &f.Candidates[index]
			if seen[change.Address] != 1 {
				fact.PlanBinding, fact.ObjectIdentity, fact.IdentityReason = "conflicting", "unverified", "duplicate_plan_address"
				continue
			}
			fact.Planned = &item
			if !imported || importing.Unknown || idPresent == identityPresent || change.Mode != "managed" || change.Type != selections[index].ManagedType || change.ProviderName != candidates[index].Provider.Source || len(change.Change.Actions) != 1 || change.Change.Actions[0] != "no-op" {
				fact.PlanBinding, fact.IdentityReason = "conflicting", "missing_or_unexpected_import_action_type_or_provider"
				continue
			}
			fact.PlanBinding = "matched"
			fact.IdentityReason = "no_comparable_provider_returned_identity; inspect_id_and_provider_scope"
			if status, reason := importCompareProviderIdentity(candidates[index].Identity, change.Change.AfterIdentity); status != "unverified" {
				fact.ObjectIdentity, fact.IdentityReason = status, reason
			}
			continue
		}
		if imported {
			f.OtherImportCount++
			if len(f.OtherImports) < 100 {
				f.OtherImports = append(f.OtherImports, item)
			} else {
				f.Truncated = true
			}
		}
		if change.Mode == "managed" && (len(change.Change.Actions) != 1 || change.Change.Actions[0] != "no-op") {
			f.OtherManagedActionCount++
			if len(f.OtherManagedActions) < 100 {
				f.OtherManagedActions = append(f.OtherManagedActions, item)
			} else {
				f.Truncated = true
			}
		}
	}
	return f, nil
}

// Only exact, complete primitive identities can be compared without inventing
// cross-provider conversion rules. Importing.identity is deliberately ignored:
// it can echo the agent's HCL, whereas after_identity comes from the provider.
func importCompareProviderIdentity(source map[string]any, after json.RawMessage) (string, string) {
	if len(source) == 0 || len(after) == 0 || bytes.Equal(after, []byte("null")) {
		return "unverified", "provider_returned_identity_unavailable"
	}
	var destination map[string]any
	if err := decodeImportEvidenceJSONLimit(after, &destination, maxImportSchemaBytes); err != nil || len(destination) != len(source) {
		return "unverified", "identity_shape_not_comparable"
	}
	// Check the whole shape before declaring any differing value a mismatch;
	// provider versions can define superficially similar but incompatible keys.
	for key, value := range source {
		actual, ok := destination[key]
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
		if value != destination[key] {
			return "mismatched", "provider_returned_identity_differs"
		}
	}
	return "matched", "complete_provider_returned_identity_matches_query_values; verify_provider_scope_separately"
}
