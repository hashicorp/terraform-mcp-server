// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// discoveryPage lists guidance fields (notes, next_cursor, next_action) before
// lists so a client that truncates a large page still sees them.
type discoveryPage struct {
	QueryRunID            string           `json:"query_run_id"`
	LogDigest             string           `json:"log_digest"`
	ResourcesDiscovered   int              `json:"resources_discovered"`
	ByType                map[string]int   `json:"by_type"`
	TotalMatching         int              `json:"total_matching"`
	RowsWithoutAttributes int              `json:"rows_without_attributes,omitempty"`
	Returned              int              `json:"returned"`
	HasMore               bool             `json:"has_more"`
	Remaining             int              `json:"remaining"`
	Notes                 []string         `json:"notes,omitempty"`
	NextCursor            string           `json:"next_cursor,omitempty"`
	NextAction            string           `json:"next_action"`
	Lists                 []discoveryGroup `json:"lists"`
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

	size, lastReturned, more, consumed := 0, "", false, 0
	perList := map[string]int{}
	for _, c := range d.Candidates {
		perList[c.Address]++
	}
	var selected []importDiscoveryCandidate
	for i, c := range d.Candidates {
		page.ByType[c.ResourceType]++
		if !matches(c) {
			continue
		}
		page.TotalMatching++
		if len(c.ResourceObject) == 0 {
			page.RowsWithoutAttributes++
		}
		if i < start {
			consumed++
			continue
		}
		if more {
			continue
		}
		// Size is measured on the flat row, which overstates the grouped size.
		encoded, _ := json.Marshal(discoveryRow{CandidateID: c.CandidateID, DisplayName: c.DisplayName, Identity: c.Identity, Tags: discoveryTags(c)})
		if len(selected) >= limit || (len(selected) > 0 && size+len(encoded) > maxDiscoveryPageBytes) {
			more = true
			continue
		}
		size += len(encoded)
		consumed++
		selected = append(selected, c)
		lastReturned = c.CandidateID
	}
	page.Returned = len(selected)
	page.HasMore = more
	page.Remaining = page.TotalMatching - consumed
	page.Lists = groupDiscoveryRows(selected, func(c importDiscoveryCandidate) discoveryRow {
		return discoveryRow{CandidateID: c.CandidateID, DisplayName: c.DisplayName, Tags: discoveryTags(c)}
	})
	switch {
	case page.TotalMatching > 0 && page.RowsWithoutAttributes == page.TotalMatching:
		page.Notes = append(page.Notes, "resource_attributes_not_captured")
	case page.RowsWithoutAttributes > 0:
		page.Notes = append(page.Notes, "resource_attributes_partly_captured")
	}
	atLimit := false
	for _, n := range perList {
		if n == defaultListLimit {
			atLimit = true
		}
	}
	if atLimit {
		page.Notes = append(page.Notes, "list_total_equals_default_limit")
	}
	if more {
		page.NextCursor = encodeDiscoveryCursor(discoveryCursor{QueryRunID: d.QueryRunID, LogDigest: d.LogDigest, LastID: lastReturned})
		page.NextAction = fmt.Sprintf("This is a partial page: %d of %d matching rows returned and %d remain. Continue from next_cursor by passing it as after; do not restart without after. %s", page.Returned, page.TotalMatching, page.Remaining, discoveryCandidateIDHint) + discoveryAttributesHint(page) + discoveryLimitHint(atLimit)
	} else {
		page.NextAction = "These are all the results this query returned. " + discoveryCompletenessCaveat + " Select up to 100 candidate_id values. " + discoveryCandidateIDHint + discoveryAttributesHint(page) + discoveryLimitHint(atLimit)
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

// defaultListLimit is Terraform's default for a list block's limit argument. The
// QueryRun log reports only each list's total, not the configured limit, so a
// total equal to the default is the only signal the server has.
const defaultListLimit = 100

// discoveryCompletenessCaveat applies to every query: a query only sees what its
// list arguments cover, so the result is never proof that nothing else exists.
const discoveryCompletenessCaveat = "A query only sees what its filters and list arguments cover (for example one region or a filtered attribute), so more matching resources may exist beyond these."

// discoveryCandidateIDHint tells the agent that prepare_import needs the IDs,
// so it keeps them instead of re-reading pages after the user confirms.
const discoveryCandidateIDHint = "prepare_import needs the candidate_id values: keep the IDs of every resource you present to the user and pass them after confirmation instead of paging the query again."

// discoveryAttributesHint tells the agent whether rows carry resource attributes
// such as tags, and how to get them. Discovery guidance lives here, not in
// prepare_import. A row without tags means the resource has none only when its
// attributes were captured.
func discoveryAttributesHint(p *discoveryPage) string {
	switch {
	case p.TotalMatching > 0 && p.RowsWithoutAttributes == p.TotalMatching:
		return " No row carries resource attributes, so an absent tags field means tags were not captured, not that the resource has none. If the identities are not enough to choose the resources to import, ask the user whether to re-run the query with generate_config_out true; a re-run creates a new query run and new candidate IDs."
	case p.RowsWithoutAttributes > 0:
		return fmt.Sprintf(" %d matching rows carry no resource attributes, so an absent tags field on those rows means not captured; on other rows it means the resource has no tags. Re-running the query with generate_config_out true captures attributes for every row and creates a new query run and new candidate IDs.", p.RowsWithoutAttributes)
	}
	return ""
}

// discoveryLimitHint warns when a list returned exactly Terraform's default limit.
func discoveryLimitHint(atLimit bool) string {
	if !atLimit {
		return ""
	}
	return fmt.Sprintf(" A list returned exactly %d results, Terraform's default list limit, so it may have been cut off. Compare with the limit you set; if more could exist, ask the user whether to re-run with a higher limit or a narrower query.", defaultListLimit)
}
