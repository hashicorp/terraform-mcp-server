// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"errors"
	"fmt"
)

// searchDiagnosticCode identifies a stable Search tool failure category.
type searchDiagnosticCode string

// Shared tool infrastructure diagnostics.
const (
	clientUnavailableCode   searchDiagnosticCode = "client_unavailable"
	providerUnavailableCode searchDiagnosticCode = "provider_data_unavailable"
)

// Query evidence and discovery diagnostics.
const (
	queryBackendUnavailableCode         searchDiagnosticCode = "query_backend_unavailable"
	queryDuplicateOrInvalidIdentityCode searchDiagnosticCode = "query_duplicate_or_invalid_identity"
	queryEvidenceIncompleteCode         searchDiagnosticCode = "query_evidence_incomplete"
	queryEvidenceInvalidCode            searchDiagnosticCode = "query_evidence_invalid"
	queryGeneratedBlockConflictCode     searchDiagnosticCode = "query_generated_block_conflict"
	queryIdentityInvalidCode            searchDiagnosticCode = "query_identity_invalid"
	queryIdentityVersionInvalidCode     searchDiagnosticCode = "query_identity_version_invalid"
	queryLogUnavailableCode             searchDiagnosticCode = "query_log_unavailable"
	queryNotFinishedCode                searchDiagnosticCode = "query_not_finished"
	queryNotSelectableCode              searchDiagnosticCode = "query_not_selectable"
	queryOrganizationMismatchCode       searchDiagnosticCode = "query_organization_mismatch"
	queryOwnershipUnverifiedCode        searchDiagnosticCode = "query_ownership_unverified"
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
	lockedProviderSourceInvalidCode searchDiagnosticCode = "locked_provider_source_invalid"
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
	planJSONInvalidCode              searchDiagnosticCode = "plan_json_invalid"
	planModuleDepthLimitCode         searchDiagnosticCode = "plan_module_depth_limit"
	planReadUnavailableCode          searchDiagnosticCode = "plan_read_unavailable"
	runEvidenceInvalidCode           searchDiagnosticCode = "run_evidence_invalid"
	runReadUnavailableCode           searchDiagnosticCode = "run_read_unavailable"
	verifyCanceledCode               searchDiagnosticCode = "verify_canceled"
	verifyConfigInvalidCode          searchDiagnosticCode = "verify_config_invalid"
	verifyInputInvalidCode           searchDiagnosticCode = "verify_input_invalid"
)

type searchDiagnosticFailure struct {
	code   searchDiagnosticCode
	detail string
	cause  error
}

func (e *searchDiagnosticFailure) Error() string {
	if e.detail != "" {
		return string(e.code) + ": " + e.detail
	}
	return string(e.code)
}

func (e *searchDiagnosticFailure) Unwrap() error { return e.cause }

// searchDiagnosticError creates a value-free error identified by a stable code.
func searchDiagnosticError(code searchDiagnosticCode) error {
	return &searchDiagnosticFailure{code: code}
}

// searchDiagnosticErrorWithDetail adds reviewed, non-sensitive recovery text
// while preserving the stable diagnostic code for both MCP adapters.
func searchDiagnosticErrorWithDetail(code searchDiagnosticCode, detail string) error {
	return &searchDiagnosticFailure{code: code, detail: detail}
}

// searchDiagnosticErrorWithCause retains an operational cause for Go error
// inspection without adding backend details to the value-free diagnostic.
func searchDiagnosticErrorWithCause(code searchDiagnosticCode, cause error) error {
	return &searchDiagnosticFailure{code: code, cause: cause}
}

// TerraformClientUnavailableError gives both MCP SDK adapters the same
// contextual, classifiable error for client acquisition failures.
func TerraformClientUnavailableError(err error) error {
	return fmt.Errorf("getting Terraform client: %w", searchDiagnosticErrorWithCause(clientUnavailableCode, err))
}

// ProviderDataUnavailableError prevents provider catalog and schema transport
// details from crossing either MCP adapter boundary.
func ProviderDataUnavailableError(err error) error {
	return sanitizedSearchToolError(err, providerUnavailableCode)
}

// QueryStatusUnavailableError prevents QueryRun transport details from
// crossing either MCP adapter boundary.
func QueryStatusUnavailableError(err error) error {
	return sanitizedSearchToolError(err, queryRunUnavailableCode)
}

func sanitizedSearchToolError(err error, fallback searchDiagnosticCode) error {
	if err == nil {
		return nil
	}
	if _, ok := searchDiagnosticCodeFromError(err); ok {
		return err
	}
	return searchDiagnosticErrorWithCause(fallback, err)
}

// searchDiagnosticCodeFromError extracts a diagnostic code through wrapped errors.
func searchDiagnosticCodeFromError(err error) (searchDiagnosticCode, bool) {
	var failure *searchDiagnosticFailure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.code, true
}
