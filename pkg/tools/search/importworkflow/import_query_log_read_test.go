// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryLogIsReadInOneRequest(t *testing.T) {
	f := importBackendFixture(t)
	f.queryLog = largeDiscoveryLog(3000)
	require.Greater(t, len(f.queryLog), 400*1024, "the fixture log should be several hundred KB")

	d, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	assert.Len(t, d.Candidates, 3000)
	assert.Equal(t, importEvidenceDigest(f.queryLog), d.LogDigest, "markers are removed and the digest covers the log itself")
	assert.Equal(t, 1, logRequests(f), "one ranged request reads the whole log")
	for _, auth := range f.logAuth {
		assert.Empty(t, auth, "the API token is not sent to the log location")
	}
}

func TestQueryLogReadFollowsAServerThatCapsResponses(t *testing.T) {
	f := importBackendFixture(t)
	f.queryLog = largeDiscoveryLog(300)
	f.logChunkMax = 16 * 1024

	d, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	assert.Len(t, d.Candidates, 300)
	assert.Equal(t, importEvidenceDigest(f.queryLog), d.LogDigest)
	reads := logRequests(f)
	assert.Greater(t, reads, 1)
	assert.LessOrEqual(t, reads, importLogReadMaxRequests)
}

func TestQueryLogReadFallsBackWhenTheLogCannotBeReadInFewRequests(t *testing.T) {
	f := importBackendFixture(t)
	f.queryLog = largeDiscoveryLog(40)
	f.logChunkMax = 64

	d, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	assert.Len(t, d.Candidates, 40)
	assert.Equal(t, importEvidenceDigest(f.queryLog), d.LogDigest, "the fallback reads the same log")
	assert.Greater(t, logRequests(f), importLogReadMaxRequests)
}

func TestQueryLogReadStillRejectsALogOverTheBound(t *testing.T) {
	f := importBackendFixture(t)
	f.queryLog = make([]byte, maxImportEvidenceBytes+1)
	for i := range f.queryLog {
		f.queryLog[i] = ' '
	}
	_, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.Error(t, err)
	assert.Equal(t, "query_evidence_size_limit", importDiagnosticCode(err))
}

func TestQueryLogReadWithoutALocationFallsBack(t *testing.T) {
	data, complete, err := readImportQueryLog(context.Background(), nil, "")
	require.NoError(t, err)
	assert.False(t, complete)
	assert.Nil(t, data)
}
