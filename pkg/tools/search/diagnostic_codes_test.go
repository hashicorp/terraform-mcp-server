// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFoundationTerraformClientUnavailableErrorPreservesCodeAndCause(t *testing.T) {
	cause := errors.New("session is not configured")
	err := TerraformClientUnavailableError(cause)
	require.EqualError(t, err, "getting Terraform client: client_unavailable")
	require.ErrorIs(t, err, cause)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, clientUnavailableCode, code)
}

func TestProviderDataUnavailableErrorPreservesCodeAndCause(t *testing.T) {
	require.NoError(t, ProviderDataUnavailableError(nil))
	cause := errors.New("backend-sensitive-sentinel")
	err := ProviderDataUnavailableError(cause)
	require.EqualError(t, err, "provider_data_unavailable")
	require.ErrorIs(t, err, cause)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, providerUnavailableCode, code)

	classified := fmt.Errorf("provider validation: %w", searchDiagnosticErrorWithDetail(lockedProviderSourceInvalidCode, "reviewed recovery text"))
	require.Same(t, classified, ProviderDataUnavailableError(classified))
	code, ok = searchDiagnosticCodeFromError(classified)
	require.True(t, ok)
	require.Equal(t, lockedProviderSourceInvalidCode, code)
	require.EqualError(t, classified, "provider validation: locked_provider_source_invalid: reviewed recovery text")
}

func TestQueryStatusUnavailableErrorPreservesCodeAndCause(t *testing.T) {
	require.NoError(t, QueryStatusUnavailableError(nil))
	cause := errors.New("backend-sensitive-sentinel")
	err := QueryStatusUnavailableError(cause)
	require.EqualError(t, err, "query_run_unavailable")
	require.ErrorIs(t, err, cause)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, queryRunUnavailableCode, code)
	classified := searchDiagnosticError(queryOwnershipUnverifiedCode)
	require.Same(t, classified, QueryStatusUnavailableError(classified))
}

func TestSearchDiagnosticCodeFromUnclassifiedError(t *testing.T) {
	for _, err := range []error{nil, errors.New("unclassified")} {
		code, ok := searchDiagnosticCodeFromError(err)
		require.False(t, ok)
		require.Empty(t, code)
	}
}
