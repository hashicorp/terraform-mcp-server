// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pagedFixture(t *testing.T, n int) *importDiscovery {
	t.Helper()
	providers := map[string]workspaceProvider{"aws_iam_role": {Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.62.0"}}
	data := largeDiscoveryLog(n)
	items, err := parseImportDiscovery(data, "qry-large", providers)
	require.NoError(t, err)
	return &importDiscovery{QueryRunID: "qry-large", LogDigest: importEvidenceDigest(data), Candidates: items}
}

func TestDiscoveryPagingIsStableAndNonOverlapping(t *testing.T) {
	d := pagedFixture(t, 1000)
	seen := map[string]bool{}
	cursor, pages := "", 0
	for {
		page, err := pageImportDiscovery(d, DiscoveryFilter{Limit: 100, After: cursor})
		require.NoError(t, err)
		assert.Equal(t, 1000, page.TotalMatching)
		assert.Equal(t, 1000, page.ResourcesDiscovered)
		assert.Equal(t, 1000, page.ByType["aws_iam_role"])
		for _, row := range page.Candidates {
			assert.False(t, seen[row.CandidateID], "row repeated across pages")
			seen[row.CandidateID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	assert.Len(t, seen, 1000)
	assert.Equal(t, 10, pages)
}

func TestDiscoveryPageDefaultsAndMaximum(t *testing.T) {
	d := pagedFixture(t, 300)
	page, err := pageImportDiscovery(d, DiscoveryFilter{})
	require.NoError(t, err)
	assert.Len(t, page.Candidates, DefaultDiscoveryPageSize)
	page, err = pageImportDiscovery(d, DiscoveryFilter{Limit: 5000})
	require.NoError(t, err)
	assert.Len(t, page.Candidates, MaxDiscoveryPageSize)
}

func TestDiscoveryFilters(t *testing.T) {
	d := pagedFixture(t, 250)
	page, err := pageImportDiscovery(d, DiscoveryFilter{NameContains: "ROLE-24", Limit: 100})
	require.NoError(t, err)
	// role-24 and role-240..role-249
	assert.Equal(t, 11, page.TotalMatching)
	page, err = pageImportDiscovery(d, DiscoveryFilter{ResourceType: "aws_s3_bucket"})
	require.NoError(t, err)
	assert.Zero(t, page.TotalMatching)
	assert.Empty(t, page.Candidates)
	page, err = pageImportDiscovery(d, DiscoveryFilter{Address: "list.aws_iam_role.roles", Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 250, page.TotalMatching)
	assert.NotEmpty(t, page.NextCursor)
}

func TestDiscoveryCursorIsBoundToSnapshotAndQuery(t *testing.T) {
	d := pagedFixture(t, 120)
	page, err := pageImportDiscovery(d, DiscoveryFilter{Limit: 10})
	require.NoError(t, err)

	changed := *d
	changed.LogDigest = "sha256:other"
	_, err = pageImportDiscovery(&changed, DiscoveryFilter{After: page.NextCursor})
	assert.ErrorContains(t, err, "snapshot_changed_restart_paging")

	other := *d
	other.QueryRunID = "qry-other"
	_, err = pageImportDiscovery(&other, DiscoveryFilter{After: page.NextCursor})
	assert.ErrorContains(t, err, "cursor_query_mismatch")

	for _, bad := range []string{"not-a-cursor", encodeDiscoveryCursor(discoveryCursor{QueryRunID: d.QueryRunID, LogDigest: d.LogDigest, LastID: "candidate-missing"})} {
		_, err = pageImportDiscovery(d, DiscoveryFilter{After: bad})
		assert.ErrorContains(t, err, "cursor_invalid")
	}
}

func TestDiscoveryPageResponseSizes(t *testing.T) {
	for _, n := range []int{10, 100, 1000} {
		d := pagedFixture(t, n)
		page, err := pageImportDiscovery(d, DiscoveryFilter{Limit: 100})
		require.NoError(t, err)
		encoded, _ := json.Marshal(page)
		t.Logf("get_query_summary page, %d results, %d rows: %d bytes", n, page.Returned, len(encoded))
		assert.Less(t, len(encoded), 64*1024)
		assert.False(t, strings.Contains(string(encoded), "resource_object"))
	}
}
