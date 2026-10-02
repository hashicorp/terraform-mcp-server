// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-tfe"
)

const (
	DefaultDiscoveryPageSize = 50
	MaxDiscoveryPageSize     = 100
	maxDiscoveryPageBytes    = 48 * 1024
)

// DiscoveryFilter narrows a page of discovered resources. Tag and attribute
// filtering are intentionally absent (TF-41405).
type DiscoveryFilter struct {
	ResourceType string
	Address      string
	NameContains string
	Limit        int
	After        string
}

type discoveryRow struct {
	CandidateID  string         `json:"candidate_id"`
	Address      string         `json:"address"`
	ResourceType string         `json:"resource_type"`
	DisplayName  string         `json:"display_name,omitempty"`
	Identity     map[string]any `json:"identity"`
}

type discoveryPage struct {
	QueryRunID          string         `json:"query_run_id"`
	LogDigest           string         `json:"log_digest"`
	ResourcesDiscovered int            `json:"resources_discovered"`
	ByType              map[string]int `json:"by_type"`
	TotalMatching       int            `json:"total_matching"`
	Returned            int            `json:"returned"`
	Candidates          []discoveryRow `json:"candidates"`
	NextCursor          string         `json:"next_cursor,omitempty"`
	NextAction          string         `json:"next_action"`
}

type discoveryCursor struct {
	QueryRunID string `json:"q"`
	LogDigest  string `json:"d"`
	LastID     string `json:"a"`
}

func encodeDiscoveryCursor(c discoveryCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeDiscoveryCursor(s string) (discoveryCursor, error) {
	var c discoveryCursor
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || json.Unmarshal(raw, &c) != nil || c.QueryRunID == "" || c.LogDigest == "" || c.LastID == "" {
		return c, importEvidenceFailure("cursor_invalid")
	}
	return c, nil
}

// pageImportDiscovery returns a transport-sized view of a fully checked
// discovery. Pages follow the log order, which is stable for one log digest;
// the cursor is bound to the QueryRun and that digest.
func pageImportDiscovery(d *importDiscovery, f DiscoveryFilter) (*discoveryPage, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultDiscoveryPageSize
	}
	if limit > MaxDiscoveryPageSize {
		limit = MaxDiscoveryPageSize
	}
	start := 0
	if f.After != "" {
		c, err := decodeDiscoveryCursor(f.After)
		if err != nil {
			return nil, err
		}
		if c.QueryRunID != d.QueryRunID {
			return nil, importEvidenceFailure("cursor_query_mismatch")
		}
		if c.LogDigest != d.LogDigest {
			return nil, importEvidenceFailure("snapshot_changed_restart_paging")
		}
		start = -1
		for i, candidate := range d.Candidates {
			if candidate.CandidateID == c.LastID {
				start = i + 1
				break
			}
		}
		if start < 0 {
			return nil, importEvidenceFailure("cursor_invalid")
		}
	}

	page := &discoveryPage{
		QueryRunID:          d.QueryRunID,
		LogDigest:           d.LogDigest,
		ResourcesDiscovered: len(d.Candidates),
		ByType:              map[string]int{},
		Candidates:          []discoveryRow{},
	}
	name := strings.ToLower(f.NameContains)
	matches := func(c importDiscoveryCandidate) bool {
		return (f.ResourceType == "" || c.ResourceType == f.ResourceType) &&
			(f.Address == "" || c.Address == f.Address) &&
			(name == "" || strings.Contains(strings.ToLower(c.DisplayName), name))
	}

	size, lastReturned, more := 0, "", false
	for i, c := range d.Candidates {
		page.ByType[c.ResourceType]++
		if !matches(c) {
			continue
		}
		page.TotalMatching++
		if i < start || more {
			continue
		}
		row := discoveryRow{CandidateID: c.CandidateID, Address: c.Address, ResourceType: c.ResourceType, DisplayName: c.DisplayName, Identity: c.Identity}
		encoded, _ := json.Marshal(row)
		if len(page.Candidates) >= limit || (len(page.Candidates) > 0 && size+len(encoded) > maxDiscoveryPageBytes) {
			more = true
			continue
		}
		size += len(encoded)
		page.Candidates = append(page.Candidates, row)
		lastReturned = c.CandidateID
	}
	page.Returned = len(page.Candidates)
	if more {
		page.NextCursor = encodeDiscoveryCursor(discoveryCursor{QueryRunID: d.QueryRunID, LogDigest: d.LogDigest, LastID: lastReturned})
		page.NextAction = "Pass next_cursor as after to read the next page. Select candidate_id values from any page, then call prepare_import."
	} else {
		page.NextAction = "All matching results are listed. Select up to 100 candidate_id values, then call prepare_import."
	}
	return page, nil
}

// ReadDiscoveryPage supplies get_query_summary with one bounded, filtered page.
func ReadDiscoveryPage(ctx context.Context, c *tfe.Client, queryID string, f DiscoveryFilter) (any, error) {
	d, err := readImportDiscovery(ctx, c, queryID)
	if err != nil {
		return nil, err
	}
	return pageImportDiscovery(d, f)
}
