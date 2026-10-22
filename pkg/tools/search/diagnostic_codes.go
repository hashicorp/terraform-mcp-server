// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"errors"
	"fmt"
)

// searchDiagnosticCode identifies a stable Search tool failure category.
type searchDiagnosticCode string

// Query evidence and discovery diagnostics (provider/status codes live below).
const (
	queryBackendUnavailableCode               searchDiagnosticCode = "query_backend_unavailable"
	queryDuplicateOrInvalidIdentityCode       searchDiagnosticCode = "query_duplicate_or_invalid_identity"
	queryEvidenceIncompleteCode               searchDiagnosticCode = "query_evidence_incomplete"
	queryEvidenceInvalidCode                  searchDiagnosticCode = "query_evidence_invalid"
	queryGeneratedBlockConflictCode           searchDiagnosticCode = "query_generated_block_conflict"
	queryGeneratedConfigurationInvalidCode    searchDiagnosticCode = "query_generated_configuration_invalid"
	queryGeneratedConfigurationUnexpectedCode searchDiagnosticCode = "query_generated_configuration_unexpected"
	queryIdentityInvalidCode                  searchDiagnosticCode = "query_identity_invalid"
	queryIdentityVersionInvalidCode           searchDiagnosticCode = "query_identity_version_invalid"
	queryLogSizeLimitCode                     searchDiagnosticCode = "query_log_size_limit"
	queryLogUnavailableCode                   searchDiagnosticCode = "query_log_unavailable"
	queryNotFinishedCode                      searchDiagnosticCode = "query_not_finished"
	queryNotSelectableCode                    searchDiagnosticCode = "query_not_selectable"
	queryOrganizationMismatchCode             searchDiagnosticCode = "query_organization_mismatch"
	queryProvenanceUnavailableCode            searchDiagnosticCode = "query_provenance_unavailable"
	queryProviderAmbiguousCode                searchDiagnosticCode = "query_provider_ambiguous"
	queryReportedErrorCode                    searchDiagnosticCode = "query_reported_error"
	queryRunIDInvalidCode                     searchDiagnosticCode = "query_run_id_invalid"
	querySummaryUnserializableCode            searchDiagnosticCode = "query_summary_unserializable"
)

// Shared tool infrastructure diagnostics.
const (
	clientUnavailableCode   searchDiagnosticCode = "client_unavailable"
	providerUnavailableCode searchDiagnosticCode = "provider_data_unavailable"
)

// Reusable provider-address and exact locked-version validation diagnostics.
const (
	queryProviderSourceUnresolvedCode searchDiagnosticCode = "query_provider_source_unresolved"
	lockVersionsInvalidCode           searchDiagnosticCode = "lock_versions_invalid"
	lockedProviderSourceInvalidCode   searchDiagnosticCode = "locked_provider_source_invalid"
)

// QueryRun status and backend ownership diagnostics.
const (
	queryOwnershipUnverifiedCode searchDiagnosticCode = "query_ownership_unverified"
	queryRunUnavailableCode      searchDiagnosticCode = "query_run_unavailable"
)

// Import preparation diagnostics; Plan verification is introduced separately.
const (
	importArgumentsInvalidCode    searchDiagnosticCode = "import_arguments_invalid"
	importContextInvalidCode      searchDiagnosticCode = "import_context_invalid"
	importResponseInvalidCode     searchDiagnosticCode = "import_response_invalid"
	prepareInputInvalidCode       searchDiagnosticCode = "prepare_input_invalid"
	selectionInvalidCode          searchDiagnosticCode = "selection_invalid"
	targetReadUnavailableCode     searchDiagnosticCode = "target_read_unavailable"
	targetOwnershipUnverifiedCode searchDiagnosticCode = "target_ownership_unverified"
	targetEvidenceInvalidCode     searchDiagnosticCode = "target_evidence_invalid"
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
