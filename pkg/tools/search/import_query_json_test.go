// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportEvidenceJSONBounds(t *testing.T) {
	for _, tc := range []struct{ raw, code string }{
		{strings.Repeat(" ", maxImportEvidenceBytes+1), "evidence_size_limit"},
		{strings.Repeat("[", maxImportEvidenceDepth+2) + "0" + strings.Repeat("]", maxImportEvidenceDepth+2), "evidence_depth_limit"},
		{`{"nested":{"version":1,"version":2}}`, "evidence_json_invalid"},
		{`{} {}`, "evidence_json_invalid"},
	} {
		var value any
		assert.ErrorContains(t, decodeImportEvidenceJSON([]byte(tc.raw), &value), tc.code)
	}
}

func TestImportSchemaJSONDepthBound(t *testing.T) {
	// A managed provider artifact may exceed the query-evidence nesting limit,
	// but an unbounded or malicious artifact must still fail closed.
	for _, depth := range []int{50, maxImportSchemaDepth + 1} {
		raw := []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
		var value any
		err := decodeImportEvidenceJSONLimit(raw, &value, maxImportSchemaBytes)
		if depth <= maxImportSchemaDepth {
			require.NoError(t, err)
		} else {
			assert.ErrorContains(t, err, "evidence_depth_limit")
		}
	}
}

func FuzzImportEvidenceJSON(f *testing.F) {
	for _, seed := range []string{`null`, `{"id":9007199254740993}`, `{"id":"00123"}`, `{"id":1,"id":2}`, `[[{}]]`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var value any
		if decodeImportEvidenceJSON([]byte(raw), &value) == nil {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			var roundtrip any
			require.NoError(t, decodeImportEvidenceJSON(encoded, &roundtrip))
			assert.Equal(t, value, roundtrip)
		}
	})
}
