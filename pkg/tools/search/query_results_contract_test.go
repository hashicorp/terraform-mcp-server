// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryLogFusePreservesSourceCeiling(t *testing.T) {
	// Boundary behavior is exercised with small exact/one-over readers in the
	// source tests; pin the production ceiling independently of those fixtures.
	require.EqualValues(t, 256<<20, maxQueryLogBytes)
}
