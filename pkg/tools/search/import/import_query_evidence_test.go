// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportDiscoveryAcceptsLiveNoCodeResourceTypeWireName(t *testing.T) {
	f := importBackendFixture(t)
	path := "/api/v2/search/no-code-query/ncqry-fixture"
	f.responses[path] = []byte(strings.ReplaceAll(string(f.responses[path]), `"resource_type"`, `"resource-type"`))
	discovery, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	require.Len(t, discovery.Candidates, 1)
	assert.Equal(t, "registry.terraform.io/hashicorp/aws", discovery.Candidates[0].Provider.Source)
}

func TestImportGeneratedBlocksFromQueryLogWireNames(t *testing.T) {
	for _, tt := range []struct {
		name   string
		fields map[string]string
		want   string
	}{
		{"current", map[string]string{"config": "resource draft", "import_config": "import draft"}, ""},
		{"legacy", map[string]string{"configuration": "resource draft", "import_configuration": "import draft"}, ""},
		{"both matching", map[string]string{"config": "resource draft", "configuration": "resource draft", "import_config": "import draft", "import_configuration": "import draft"}, ""},
		{"none", nil, ""},
		{"conflicting config", map[string]string{"config": "resource draft", "configuration": "other"}, "query_generated_block_conflict"},
		{"conflicting import", map[string]string{"import_config": "import draft", "import_configuration": "other"}, "query_generated_block_conflict"},
		{"conflicting empty", map[string]string{"config": "", "configuration": "other"}, "query_generated_block_conflict"},
		{"oversized", map[string]string{"config": strings.Repeat("x", 32*1024+1)}, "query_generated_block_size_limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := importBackendFixture(t)
			lines := bytes.Split(bytes.TrimSpace(f.queryLog), []byte("\n"))
			var found map[string]any
			require.NoError(t, json.Unmarshal(lines[0], &found))
			resource := found["list_resource_found"].(map[string]any)
			for key, value := range tt.fields {
				resource[key] = value
			}
			first, err := json.Marshal(found)
			require.NoError(t, err)
			f.queryLog = append(append(first, '\n'), lines[1]...)
			discovery, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
			if tt.want != "" {
				assert.Equal(t, tt.want, importDiagnosticCode(err))
				return
			}
			require.NoError(t, err)
			require.Len(t, discovery.Candidates, 1)
			candidate := discovery.Candidates[0]
			if tt.fields == nil {
				assert.Empty(t, candidate.Configuration)
				assert.Empty(t, candidate.ImportConfig)
				return
			}
			assert.Equal(t, "resource draft", candidate.Configuration)
			assert.Equal(t, "import draft", candidate.ImportConfig)
			// Both the candidate-list and prepare path use the same bounded
			// decoder, preserving the stable outward field names.
			result := prepareFromAPIs(context.Background(), f.client, importFixtureInput(t))
			require.Equal(t, "prepared", result.Status, result.Diagnostics)
			assert.Equal(t, candidate.Configuration, result.Selection.Configuration)
			assert.Equal(t, candidate.ImportConfig, result.Selection.ImportConfig)
			encoded, err := json.Marshal(candidate)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"import_configuration"`)
			assert.NotContains(t, string(encoded), `"import_config"`)
		})
	}
}

func TestImportDiscoveryExactSelection(t *testing.T) {
	providers := map[string]workspaceProvider{"aws_iam_role": {Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.62.0"}}
	data := phase0Fixture(t, "query.ndjson")
	items, err := parseImportDiscovery(data, "qry-fixture", providers)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, importFixtureInput(t).Selections[0].CandidateID, items[0].CandidateID)
	for _, raw := range [][]byte{
		[]byte(strings.Split(string(data), "\n")[0]),
		append(append([]byte{}, data...), []byte(strings.Split(string(data), "\n")[0])...),
		append(append([]byte{}, data...), []byte(`{"type":"diagnostic","diagnostic":{"severity":"error"}}`)...),
		append(append([]byte{}, data...), []byte(`{"type":"list_resource_found",`)...),
	} {
		_, err := parseImportDiscovery(raw, "qry-fixture", providers)
		assert.Error(t, err)
	}
	providers["aws_iam_role"] = workspaceProvider{Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.63.0"}
	changed, err := parseImportDiscovery(data, "qry-fixture", providers)
	require.NoError(t, err)
	assert.NotEqual(t, items[0].CandidateID, changed[0].CandidateID)
}

func TestImportSDKBoundedResponse(t *testing.T) {
	f := importBackendFixture(t)
	_, _, err := readImportBackendJSON(context.Background(), f.client, "runs/run-schema/plan/json-schema", 8)
	assert.ErrorContains(t, err, "evidence_size_limit")
}

func TestImportSchemaPresignedDownloadRecovery(t *testing.T) {
	for _, failures := range []int{1, 2} {
		t.Run(strconv.Itoa(failures), func(t *testing.T) {
			f := importBackendFixture(t)
			f.deniedDownloads = failures
			result := prepareFromAPIs(context.Background(), f.client, importFixtureInput(t))
			if failures == 1 {
				assert.Equal(t, "prepared", result.Status, result.Diagnostics)
			} else {
				assert.Equal(t, "blocked", result.Status)
				assert.Contains(t, result.Diagnostics, "schema_source_access_denied")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			assert.Equal(t, 2, f.requests["GET /api/v2/runs/run-schema/plan/json-schema"], "reacquire at most once through the API")
			assert.Equal(t, 2, f.requests["GET /schema-download"])
		})
	}
}
