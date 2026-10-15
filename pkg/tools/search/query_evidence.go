// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Query evidence is read in full before any result becomes selectable. The
// response budget is separate from this raw log bound.
const maxQueryEvidenceBytes = 4 << 20
const maxQueryEvidenceDepth = 32

type queryEvidenceError string

func (e queryEvidenceError) Error() string { return string(e) }

type queryResultProvider struct {
	Source  string `json:"source"`
	Version string `json:"version"`
}

type queryResultEvidence struct {
	Marker          string              `json:"query_result_marker"`
	Address         string              `json:"address"`
	ResourceType    string              `json:"resource_type"`
	DisplayName     string              `json:"display_name,omitempty"`
	Identity        map[string]any      `json:"identity"`
	IdentityVersion int                 `json:"identity_version"`
	Provider        queryResultProvider `json:"provider"`
	ResourceObject  map[string]any      `json:"resource_object,omitempty"`
	Configuration   string              `json:"configuration,omitempty"`
	ImportConfig    string              `json:"import_configuration,omitempty"`
}

type queryEvidence struct {
	Results []queryResultEvidence
	ByType  map[string]int
}

// The marker is deterministic within one QueryRun but does not authenticate
// either a QueryRun or the identity of a cloud object. V0 candidate- values
// are deliberately not accepted as V1 markers.
func queryResultMarker(queryID string, provider queryResultProvider, listType string, identity map[string]any) (string, error) {
	canonical, err := json.Marshal(struct {
		QueryRunID      string         `json:"query_run_id"`
		ProviderSource  string         `json:"provider_source"`
		ProviderVersion string         `json:"provider_version"`
		ListType        string         `json:"list_type"`
		Identity        map[string]any `json:"identity"`
	}{queryID, provider.Source, provider.Version, listType, identity})
	if err != nil {
		return "", queryEvidenceError("query_identity_invalid")
	}
	return fmt.Sprintf("qrm1-%x", sha256.Sum256(canonical)), nil
}

func validIdentityVersion(raw json.RawMessage) (int, bool) {
	s := string(bytes.TrimSpace(raw))
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, strconv.IntSize)
	return int(n), err == nil
}

// decodeQueryEvidence rejects duplicate JSON keys and deep nesting before
// decoding. It does not return or log untrusted JSON on failure.
func decodeQueryEvidence(raw []byte, target any) error {
	return decodeQueryEvidenceLimit(raw, target, maxQueryEvidenceBytes, maxQueryEvidenceDepth)
}

func decodeQueryEvidenceLimit(raw []byte, target any, maxBytes, maxDepth int) error {
	if len(raw) > maxBytes {
		return queryEvidenceError("query_evidence_size_limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > maxDepth {
			return queryEvidenceError("query_evidence_depth_limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return queryEvidenceError("query_evidence_invalid")
		}
		delim, isDelim := token.(json.Delim)
		if !isDelim {
			return nil
		}
		if delim != '{' && delim != '[' {
			return queryEvidenceError("query_evidence_invalid")
		}
		keys := make(map[string]bool)
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || keys[name] {
					return queryEvidenceError("query_evidence_invalid")
				}
				keys[name] = true
			}
			if err := walk(depth + 1); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return queryEvidenceError("query_evidence_invalid")
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return queryEvidenceError("query_evidence_invalid")
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return queryEvidenceError("query_evidence_invalid")
	}
	return nil
}

// parseQueryEvidence makes the whole QueryRun nonselectable when any found
// record is malformed, duplicated, lacks a valid version/identity, or has
// incomplete list totals. It never returns a partial selection on failure.
func parseQueryEvidence(queryID string, data []byte, providers map[string]queryResultProvider) (*queryEvidence, error) {
	if queryID == "" || len(data) > maxQueryEvidenceBytes {
		return nil, queryEvidenceError("query_evidence_size_limit")
	}
	out := &queryEvidence{Results: []queryResultEvidence{}, ByType: map[string]int{}}
	counts, totals, seen := map[string]int{}, map[string]int{}, map[string]bool{}
	completeTypes := map[string]string{}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		// QueryRun logs also contain ordinary text and JSON progress events.
		// Like the V0 reader, ignore text that cannot carry a result, but never
		// silently skip a damaged found/completion/diagnostic record or an
		// unexpected JSON array. The complete log is still checked below.
		if !bytes.HasPrefix(line, []byte{'{'}) {
			if bytes.HasPrefix(line, []byte{'['}) || bytes.Contains(line, []byte("list_resource_found")) || bytes.Contains(line, []byte("list_complete")) || bytes.Contains(line, []byte("diagnostic")) {
				return nil, queryEvidenceError("query_evidence_invalid")
			}
			continue
		}
		var record struct {
			Type  string `json:"type"`
			Found *struct {
				Address       string          `json:"address"`
				ResourceType  string          `json:"resource_type"`
				DisplayName   string          `json:"display_name"`
				Identity      map[string]any  `json:"identity"`
				Version       json.RawMessage `json:"identity_version"`
				Resource      map[string]any  `json:"resource_object"`
				Config        *string         `json:"config"`
				ImportConfig  *string         `json:"import_config"`
				Configuration *string         `json:"configuration"`
				ImportBlock   *string         `json:"import_configuration"`
			} `json:"list_resource_found"`
			Complete *struct {
				Address      string `json:"address"`
				ResourceType string `json:"resource_type"`
				Total        int    `json:"total"`
			} `json:"list_complete"`
			Diagnostic *struct {
				Severity string `json:"severity"`
			} `json:"diagnostic"`
		}
		if decodeQueryEvidence(line, &record) != nil {
			return nil, queryEvidenceError("query_evidence_invalid")
		}
		switch record.Type {
		case "list_resource_found":
			f := record.Found
			if f == nil || f.Address == "" || f.ResourceType == "" || len(f.Identity) == 0 {
				return nil, queryEvidenceError("query_identity_invalid")
			}
			version, ok := validIdentityVersion(f.Version)
			if !ok {
				return nil, queryEvidenceError("query_identity_version_invalid")
			}
			provider, ok := providers[f.ResourceType]
			if !ok || provider.Source == "" || provider.Version == "" {
				return nil, queryEvidenceError("query_provider_source_unresolved")
			}
			marker, err := queryResultMarker(queryID, provider, f.ResourceType, f.Identity)
			if err != nil || seen[marker] {
				return nil, queryEvidenceError("query_duplicate_or_invalid_identity")
			}
			seen[marker] = true
			if f.Config != nil && f.Configuration != nil && *f.Config != *f.Configuration || f.ImportConfig != nil && f.ImportBlock != nil && *f.ImportConfig != *f.ImportBlock {
				return nil, queryEvidenceError("query_generated_block_conflict")
			}
			result := queryResultEvidence{Marker: marker, Address: f.Address, ResourceType: f.ResourceType, DisplayName: f.DisplayName, Identity: f.Identity, IdentityVersion: version, Provider: provider, ResourceObject: f.Resource}
			if f.Config != nil {
				result.Configuration = *f.Config
			} else if f.Configuration != nil {
				result.Configuration = *f.Configuration
			}
			if f.ImportConfig != nil {
				result.ImportConfig = *f.ImportConfig
			} else if f.ImportBlock != nil {
				result.ImportConfig = *f.ImportBlock
			}
			if len(result.Configuration) > 32<<10 || len(result.ImportConfig) > 32<<10 {
				return nil, queryEvidenceError("query_generated_block_size_limit")
			}
			out.Results = append(out.Results, result)
			out.ByType[f.ResourceType]++
			counts[f.Address]++
		case "list_complete":
			if record.Complete == nil || record.Complete.Address == "" || record.Complete.Total < 0 {
				return nil, queryEvidenceError("query_evidence_invalid")
			}
			if _, ok := totals[record.Complete.Address]; ok {
				return nil, queryEvidenceError("query_evidence_invalid")
			}
			totals[record.Complete.Address] = record.Complete.Total
			completeTypes[record.Complete.Address] = record.Complete.ResourceType
		case "diagnostic":
			if record.Diagnostic == nil || strings.ToLower(record.Diagnostic.Severity) != "warning" {
				return nil, queryEvidenceError("query_reported_error")
			}
		case "version", "log", "list_start":
			if record.Found != nil || record.Complete != nil || record.Diagnostic != nil {
				return nil, queryEvidenceError("query_evidence_invalid")
			}
		default:
			// Progress and policy messages do not contribute result rows. Keep
			// unknown result-bearing records nonselectable rather than mistaking
			// them for benign progress messages.
			if record.Found != nil || record.Complete != nil || record.Diagnostic != nil || strings.Contains(record.Type, "resource_found") || strings.HasPrefix(record.Type, "list_") || strings.Contains(record.Type, "diagnostic") {
				return nil, queryEvidenceError("query_evidence_invalid")
			}
		}
	}
	if len(totals) == 0 {
		return nil, queryEvidenceError("query_evidence_incomplete")
	}
	for addr, total := range totals {
		if counts[addr] != total {
			return nil, queryEvidenceError("query_evidence_incomplete")
		}
	}
	for addr := range counts {
		if _, ok := totals[addr]; !ok {
			return nil, queryEvidenceError("query_evidence_incomplete")
		}
	}
	for _, result := range out.Results {
		if kind := completeTypes[result.Address]; kind != "" && kind != result.ResourceType {
			return nil, queryEvidenceError("query_evidence_incomplete")
		}
	}
	return out, nil
}
