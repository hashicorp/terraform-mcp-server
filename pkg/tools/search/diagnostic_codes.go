// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import "errors"

// searchDiagnosticCode identifies a stable, value-free Search tool failure.
type searchDiagnosticCode string

// Query evidence and discovery diagnostics.
const (
	queryBackendUnavailableCode         searchDiagnosticCode = "query_backend_unavailable"
	queryDuplicateOrInvalidIdentityCode searchDiagnosticCode = "query_duplicate_or_invalid_identity"
	queryEvidenceDepthLimitCode         searchDiagnosticCode = "query_evidence_depth_limit"
	queryEvidenceIncompleteCode         searchDiagnosticCode = "query_evidence_incomplete"
	queryEvidenceInvalidCode            searchDiagnosticCode = "query_evidence_invalid"
	queryEvidenceSizeLimitCode          searchDiagnosticCode = "query_evidence_size_limit"
	queryGeneratedBlockConflictCode     searchDiagnosticCode = "query_generated_block_conflict"
	queryGeneratedBlockSizeLimitCode    searchDiagnosticCode = "query_generated_block_size_limit"
	queryIdentityInvalidCode            searchDiagnosticCode = "query_identity_invalid"
	queryIdentityVersionInvalidCode     searchDiagnosticCode = "query_identity_version_invalid"
	queryLogUnavailableCode             searchDiagnosticCode = "query_log_unavailable"
	queryNotFinishedCode                searchDiagnosticCode = "query_not_finished"
	queryNotSelectableCode              searchDiagnosticCode = "query_not_selectable"
	queryOrganizationMismatchCode       searchDiagnosticCode = "query_organization_mismatch"
	queryOwnershipUnverifiedCode        searchDiagnosticCode = "query_ownership_unverified"
	queryProvenanceChangedCode          searchDiagnosticCode = "query_provenance_changed"
	queryProvenanceUnavailableCode      searchDiagnosticCode = "query_provenance_unavailable"
	queryProviderAmbiguousCode          searchDiagnosticCode = "query_provider_ambiguous"
	queryProviderSourceUnresolvedCode   searchDiagnosticCode = "query_provider_source_unresolved"
	queryReportedErrorCode              searchDiagnosticCode = "query_reported_error"
	queryRunIDInvalidCode               searchDiagnosticCode = "query_run_id_invalid"
	queryRunUnavailableCode             searchDiagnosticCode = "query_run_unavailable"
	querySummaryUnserializableCode      searchDiagnosticCode = "query_summary_unserializable"
)

// Import preparation, arguments, and context diagnostics.
const (
	bindingInvalidCode              searchDiagnosticCode = "binding_invalid"
	importArgumentsInvalidCode      searchDiagnosticCode = "import_arguments_invalid"
	importContextDigestMismatchCode searchDiagnosticCode = "import_context_digest_mismatch"
	importContextInvalidCode        searchDiagnosticCode = "import_context_invalid"
	importContextMarkerInvalidCode  searchDiagnosticCode = "import_context_marker_invalid"
	importResponseInvalidCode       searchDiagnosticCode = "import_response_invalid"
	lockVersionsInvalidCode         searchDiagnosticCode = "lock_versions_invalid"
	prepareInputInvalidCode         searchDiagnosticCode = "prepare_input_invalid"
	selectionInvalidCode            searchDiagnosticCode = "selection_invalid"
	targetEvidenceInvalidCode       searchDiagnosticCode = "target_evidence_invalid"
	targetOwnershipUnverifiedCode   searchDiagnosticCode = "target_ownership_unverified"
	targetReadUnavailableCode       searchDiagnosticCode = "target_read_unavailable"
)

// Run and Plan verification diagnostics.
const (
	planActionsUnsupportedCode       searchDiagnosticCode = "plan_actions_unsupported"
	planDriftEvidenceInvalidCode     searchDiagnosticCode = "plan_drift_evidence_invalid"
	planEvidenceInvalidCode          searchDiagnosticCode = "plan_evidence_invalid"
	planIdentityEvidenceInvalidCode  searchDiagnosticCode = "plan_identity_evidence_invalid"
	planIdentityVersionDuplicateCode searchDiagnosticCode = "plan_identity_version_duplicate"
	planImportEvidenceInvalidCode    searchDiagnosticCode = "plan_import_evidence_invalid"
	planModuleDepthLimitCode         searchDiagnosticCode = "plan_module_depth_limit"
	planReadUnavailableCode          searchDiagnosticCode = "plan_read_unavailable"
	runEvidenceInvalidCode           searchDiagnosticCode = "run_evidence_invalid"
	runReadUnavailableCode           searchDiagnosticCode = "run_read_unavailable"
	verifyCanceledCode               searchDiagnosticCode = "verify_canceled"
	verifyConfigInvalidCode          searchDiagnosticCode = "verify_config_invalid"
	verifyInputInvalidCode           searchDiagnosticCode = "verify_input_invalid"
)

type searchDiagnosticFailure struct {
	code searchDiagnosticCode
}

func (e *searchDiagnosticFailure) Error() string { return string(e.code) }

// searchDiagnosticError creates a value-free error identified by a stable code.
func searchDiagnosticError(code searchDiagnosticCode) error {
	return &searchDiagnosticFailure{code: code}
}

// searchDiagnosticCodeFromError extracts a diagnostic code through wrapped errors.
func searchDiagnosticCodeFromError(err error) (searchDiagnosticCode, bool) {
	var failure *searchDiagnosticFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.code, true
}
