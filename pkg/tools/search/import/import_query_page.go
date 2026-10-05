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
	// Agent-context limits, tuned from measured queries (ADR 0006).
	DefaultDiscoveryPageSize = 100
	MaxDiscoveryPageSize     = 200
	maxDiscoveryPageBytes    = 64 * 1024
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

// discoveryRow is one selectable result. Identity holds only the keys that
// differ from the group's shared_identity; merge the two to get the identity.
type discoveryRow struct {
	CandidateID string         `json:"candidate_id"`
	DisplayName string         `json:"display_name,omitempty"`
	Identity    map[string]any `json:"identity,omitempty"`
	// Tags come from the generated resource object; absent when the result has none.
	Tags map[string]any `json:"tags,omitempty"`
}

// discoveryGroup states the list block address and type once for its rows.
type discoveryGroup struct {
	Address        string         `json:"address"`
	ResourceType   string         `json:"resource_type"`
	SharedIdentity map[string]any `json:"shared_identity,omitempty"`
	Candidates     []discoveryRow `json:"candidates"`
}

type discoveryPage struct {
	QueryRunID          string           `json:"query_run_id"`
	LogDigest           string           `json:"log_digest"`
	ResourcesDiscovered int              `json:"resources_discovered"`
	ByType              map[string]int   `json:"by_type"`
	TotalMatching       int              `json:"total_matching"`
	Returned            int              `json:"returned"`
	Lists               []discoveryGroup `json:"lists"`
	Notes               []string         `json:"notes,omitempty"`
	NextCursor          string           `json:"next_cursor,omitempty"`
	NextAction          string           `json:"next_action"`
}

// groupDiscoveryRows groups rows by list address in first-seen order and hoists
// identity keys that have the same value in every row of a multi-row group.
func groupDiscoveryRows(rows []importDiscoveryCandidate, display func(importDiscoveryCandidate) discoveryRow) []discoveryGroup {
	groups := []discoveryGroup{}
	index := map[string]int{}
	members := map[string][]importDiscoveryCandidate{}
	for _, c := range rows {
		if _, ok := index[c.Address]; !ok {
			index[c.Address] = len(groups)
			groups = append(groups, discoveryGroup{Address: c.Address, ResourceType: c.ResourceType})
		}
		members[c.Address] = append(members[c.Address], c)
	}
	for i := range groups {
		list := members[groups[i].Address]
		shared := map[string]any{}
		if len(list) > 1 {
			for k, v := range list[0].Identity {
				encoded, _ := json.Marshal(v)
				same := true
				for _, other := range list[1:] {
					ov, ok := other.Identity[k]
					oe, _ := json.Marshal(ov)
					if !ok || string(oe) != string(encoded) {
						same = false
						break
					}
				}
				if same {
					shared[k] = v
				}
			}
		}
		if len(shared) > 0 {
			groups[i].SharedIdentity = shared
		}
		groups[i].Candidates = make([]discoveryRow, 0, len(list))
		for _, c := range list {
			row := display(c)
			row.Identity = map[string]any{}
			for k, v := range c.Identity {
				if _, ok := shared[k]; !ok {
					row.Identity[k] = v
				}
			}
			if len(row.Identity) == 0 {
				row.Identity = nil
			}
			groups[i].Candidates = append(groups[i].Candidates, row)
		}
	}
	return groups
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
		Lists:               []discoveryGroup{},
	}
	name := strings.ToLower(f.NameContains)
	matches := func(c importDiscoveryCandidate) bool {
		return (f.ResourceType == "" || c.ResourceType == f.ResourceType) &&
			(f.Address == "" || c.Address == f.Address) &&
			(name == "" || strings.Contains(strings.ToLower(c.DisplayName), name))
	}

	size, lastReturned, more := 0, "", false
	var selected []importDiscoveryCandidate
	for i, c := range d.Candidates {
		page.ByType[c.ResourceType]++
		if !matches(c) {
			continue
		}
		page.TotalMatching++
		if i < start || more {
			continue
		}
		// Size is measured on the flat row, which overstates the grouped size.
		encoded, _ := json.Marshal(discoveryRow{CandidateID: c.CandidateID, DisplayName: c.DisplayName, Identity: c.Identity, Tags: discoveryTags(c)})
		if len(selected) >= limit || (len(selected) > 0 && size+len(encoded) > maxDiscoveryPageBytes) {
			more = true
			continue
		}
		size += len(encoded)
		selected = append(selected, c)
		lastReturned = c.CandidateID
	}
	page.Returned = len(selected)
	page.Lists = groupDiscoveryRows(selected, func(c importDiscoveryCandidate) discoveryRow {
		return discoveryRow{CandidateID: c.CandidateID, DisplayName: c.DisplayName, Tags: discoveryTags(c)}
	})
	if d.GenerateConfigOut != nil && !*d.GenerateConfigOut {
		page.Notes = append(page.Notes, "query_run_without_generated_config")
	}
	if more {
		page.NextCursor = encodeDiscoveryCursor(discoveryCursor{QueryRunID: d.QueryRunID, LogDigest: d.LogDigest, LastID: lastReturned})
		page.NextAction = "Pass next_cursor as after to read the next page. Select candidate_id values from any page, then call prepare_import." + discoveryRerunHint(d)
	} else {
		page.NextAction = "All matching results are listed. Select up to 100 candidate_id values, then call prepare_import." + discoveryRerunHint(d)
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

// discoveryTags returns the result's tags, or nil when it has none. Only simple
// scalar values are returned so an unusual shape cannot bloat a row.
func discoveryTags(c importDiscoveryCandidate) map[string]any {
	raw, ok := c.ResourceObject["tags"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	tags := make(map[string]any, len(raw))
	for k, v := range raw {
		switch v.(type) {
		case string, bool, float64:
			tags[k] = v
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// discoveryRerunHint tells the agent how to get tags and attributes when the
// query ran without generated configuration. Discovery guidance lives here.
func discoveryRerunHint(d *importDiscovery) string {
	if d.GenerateConfigOut == nil || *d.GenerateConfigOut {
		return ""
	}
	return " This query ran without generate_config_out, so rows carry identity only, with no tags or attributes. If the identities are not enough to choose the resources to import, ask the user whether to re-run the query with generate_config_out true; a re-run creates a new query run and new candidate IDs."
}
