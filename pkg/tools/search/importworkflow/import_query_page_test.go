// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"encoding/json"
	"fmt"
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
		for _, g := range page.Lists {
			for _, row := range g.Candidates {
				assert.False(t, seen[row.CandidateID], "row repeated across pages")
				seen[row.CandidateID] = true
			}
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
	assert.Equal(t, 100, DefaultDiscoveryPageSize)
	assert.Equal(t, 200, MaxDiscoveryPageSize)
	assert.Equal(t, DefaultDiscoveryPageSize, page.Returned)
	page, err = pageImportDiscovery(d, DiscoveryFilter{Limit: 5000})
	require.NoError(t, err)
	assert.Equal(t, MaxDiscoveryPageSize, page.Returned)
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
	assert.Empty(t, page.Lists)
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

func TestDiscoveryPagesAreGroupedWithSharedIdentity(t *testing.T) {
	d := pagedFixture(t, 5)
	page, err := pageImportDiscovery(d, DiscoveryFilter{})
	require.NoError(t, err)
	require.Len(t, page.Lists, 1)
	g := page.Lists[0]
	assert.Equal(t, "list.aws_iam_role.roles", g.Address)
	assert.Equal(t, "aws_iam_role", g.ResourceType)
	assert.Equal(t, map[string]any{"account_id": "123456789012"}, g.SharedIdentity)
	require.Len(t, g.Candidates, 5)
	for i, row := range g.Candidates {
		assert.Equal(t, map[string]any{"name": fmt.Sprintf("role-%d", i)}, row.Identity, "only differing keys remain")
		assert.NotEmpty(t, row.CandidateID)
	}

	// A single-row group keeps its full identity: nothing is shared.
	one, err := pageImportDiscovery(d, DiscoveryFilter{NameContains: "role-3"})
	require.NoError(t, err)
	require.Len(t, one.Lists[0].Candidates, 1)
	assert.Empty(t, one.Lists[0].SharedIdentity)
	assert.Equal(t, "123456789012", one.Lists[0].Candidates[0].Identity["account_id"])
}

func TestDiscoveryGroupingKeepsListsSeparate(t *testing.T) {
	mk := func(addr, name string) importDiscoveryCandidate {
		return importDiscoveryCandidate{CandidateID: "candidate-" + name, Address: addr, ResourceType: "aws_iam_role", Identity: map[string]any{"account_id": "1", "name": name}}
	}
	groups := groupDiscoveryRows([]importDiscoveryCandidate{mk("list.a", "x"), mk("list.b", "y"), mk("list.a", "z")}, func(c importDiscoveryCandidate) discoveryRow {
		return discoveryRow{CandidateID: c.CandidateID}
	})
	require.Len(t, groups, 2)
	assert.Equal(t, "list.a", groups[0].Address)
	assert.Len(t, groups[0].Candidates, 2)
	assert.Equal(t, map[string]any{"account_id": "1"}, groups[0].SharedIdentity)
	assert.Len(t, groups[1].Candidates, 1)
	assert.Empty(t, groups[1].SharedIdentity)
}

func TestDiscoveryPageByteCapStopsWideRows(t *testing.T) {
	d := pagedFixture(t, 300)
	for i := range d.Candidates {
		d.Candidates[i].DisplayName = strings.Repeat("w", 1024)
	}
	page, err := pageImportDiscovery(d, DiscoveryFilter{Limit: MaxDiscoveryPageSize})
	require.NoError(t, err)
	assert.Less(t, page.Returned, MaxDiscoveryPageSize)
	assert.NotEmpty(t, page.NextCursor, "stopped by bytes, not truncated silently")
	encoded, _ := json.Marshal(page)
	assert.Less(t, len(encoded), maxDiscoveryPageBytes+4*1024)
}

func TestDiscoveryPageResponseSizes(t *testing.T) {
	for _, n := range []int{10, 100, 1000} {
		d := pagedFixture(t, n)
		page, err := pageImportDiscovery(d, DiscoveryFilter{Limit: MaxDiscoveryPageSize})
		require.NoError(t, err)
		encoded, _ := json.Marshal(page)
		t.Logf("get_query_summary page, %d results, %d rows: %d bytes", n, page.Returned, len(encoded))
		assert.Less(t, len(encoded), maxDiscoveryPageBytes)
		assert.False(t, strings.Contains(string(encoded), "resource_object"))
	}
}

func TestDiscoveryPagePartialMarkers(t *testing.T) {
	d := pagedFixture(t, 250)
	first, err := pageImportDiscovery(d, DiscoveryFilter{Limit: 100})
	require.NoError(t, err)
	assert.True(t, first.HasMore)
	assert.Equal(t, 150, first.Remaining)
	assert.Contains(t, first.NextAction, "partial page")
	assert.Contains(t, first.NextAction, "do not restart without after")
	assert.Contains(t, first.NextAction, "candidate_id and display_name")

	second, err := pageImportDiscovery(d, DiscoveryFilter{Limit: 100, After: first.NextCursor})
	require.NoError(t, err)
	assert.True(t, second.HasMore)
	assert.Equal(t, 50, second.Remaining)

	last, err := pageImportDiscovery(d, DiscoveryFilter{Limit: 100, After: second.NextCursor})
	require.NoError(t, err)
	assert.False(t, last.HasMore)
	assert.Equal(t, 0, last.Remaining)
	assert.Contains(t, last.NextAction, "candidate_id and display_name")
}

func jsonKeyOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	_, err := dec.Token()
	require.NoError(t, err)
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		require.NoError(t, dec.Decode(&skip))
	}
	return keys
}

func indexOf(keys []string, k string) int {
	for i, v := range keys {
		if v == k {
			return i
		}
	}
	return -1
}

// Guidance must precede the large arrays so a client that truncates a long
// response still sees it.
func TestDiscoveryPageGuidancePrecedesLists(t *testing.T) {
	page, err := pageImportDiscovery(pagedFixture(t, 250), DiscoveryFilter{Limit: 100})
	require.NoError(t, err)
	raw, err := json.Marshal(page)
	require.NoError(t, err)
	keys := jsonKeyOrder(t, raw)
	lists := indexOf(keys, "lists")
	require.NotEqual(t, -1, lists)
	for _, k := range []string{"has_more", "remaining", "next_cursor", "next_action"} {
		i := indexOf(keys, k)
		require.NotEqual(t, -1, i, k)
		assert.Less(t, i, lists, k)
	}
}

func TestPreparedGuidancePrecedesLargeArrays(t *testing.T) {
	raw, err := json.Marshal(importPrepared{
		Carry:      &importCarryBlock{},
		Candidates: []importDiscoveryCandidate{{}},
		Types:      []importPreparedType{{}},
		NextAction: "x",
	})
	require.NoError(t, err)
	keys := jsonKeyOrder(t, raw)
	for _, big := range []string{"types", "candidates"} {
		b := indexOf(keys, big)
		require.NotEqual(t, -1, b, big)
		for _, k := range []string{"carry", "diagnostics", "next_action"} {
			assert.Less(t, indexOf(keys, k), b, k+" before "+big)
		}
	}
}

func TestDiscoveryGuidanceKeepsRowsAndAttributes(t *testing.T) {
	page, err := pageImportDiscovery(pagedFixture(t, 250), DiscoveryFilter{Limit: 100})
	require.NoError(t, err)
	for _, want := range []string{"candidate_id and display_name", "attributes the user's search was about", "only what you return is kept", "returns the same fields"} {
		assert.Contains(t, page.NextAction, want)
	}
}
