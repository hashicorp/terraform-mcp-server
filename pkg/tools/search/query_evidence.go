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

type queryResultMarkerPayload struct {
	QueryRunID      string         `json:"query_run_id"`
	ProviderSource  string         `json:"provider_source"`
	ProviderVersion string         `json:"provider_version"`
	ListType        string         `json:"list_type"`
	Identity        map[string]any `json:"identity"`
}

type queryLogRecord struct {
	Type       string               `json:"type"`
	Found      *queryFoundResource  `json:"list_resource_found"`
	Complete   *queryListCompletion `json:"list_complete"`
	Diagnostic *queryLogDiagnostic  `json:"diagnostic"`
}

type queryFoundResource struct {
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
}

type queryListCompletion struct {
	Address      string `json:"address"`
	ResourceType string `json:"resource_type"`
	Total        int    `json:"total"`
}

type queryLogDiagnostic struct {
	Severity string `json:"severity"`
}

// The marker is deterministic within one QueryRun but does not authenticate
// either a QueryRun or the identity of a cloud object. V0 candidate- values
// are deliberately not accepted as V1 markers.
func queryResultMarker(queryID string, provider queryResultProvider, listType string, identity map[string]any) (string, error) {
	canonical, err := json.Marshal(queryResultMarkerPayload{
		QueryRunID:      queryID,
		ProviderSource:  provider.Source,
		ProviderVersion: provider.Version,
		ListType:        listType,
		Identity:        identity,
	})
	if err != nil {
		return "", searchDiagnosticError(queryIdentityInvalidCode)
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

// decodeQueryEvidence rejects duplicate JSON keys before decoding. It does not
// return or log untrusted JSON on failure.
func decodeQueryEvidence(raw []byte, target any) error {
	if err := validateQueryEvidenceJSON(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return searchDiagnosticError(queryEvidenceInvalidCode)
	}
	return nil
}

// validateQueryEvidenceJSON rejects duplicate keys and trailing JSON values
// without imposing application-defined byte or nesting-depth limits.
func validateQueryEvidenceJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return searchDiagnosticError(queryEvidenceInvalidCode)
		}
		delim, isDelim := token.(json.Delim)
		if !isDelim {
			return nil
		}
		if delim != '{' && delim != '[' {
			return searchDiagnosticError(queryEvidenceInvalidCode)
		}
		keys := make(map[string]bool)
		for decoder.More() {
			if delim == '{' {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || keys[name] {
					return searchDiagnosticError(queryEvidenceInvalidCode)
				}
				keys[name] = true
			}
			if err := walk(); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return searchDiagnosticError(queryEvidenceInvalidCode)
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return searchDiagnosticError(queryEvidenceInvalidCode)
	}
	return nil
}

// parseQueryEvidence makes the whole QueryRun nonselectable when any found
// record is malformed, duplicated, lacks a valid version/identity, or has
// incomplete list totals. It never returns a partial selection on failure.
func parseQueryEvidence(queryID string, data []byte, providers map[string]queryResultProvider) (*queryEvidence, error) {
	if queryID == "" {
		return nil, searchDiagnosticError(queryEvidenceInvalidCode)
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
				return nil, searchDiagnosticError(queryEvidenceInvalidCode)
			}
			continue
		}
		var record queryLogRecord
		if decodeQueryEvidence(line, &record) != nil {
			return nil, searchDiagnosticError(queryEvidenceInvalidCode)
		}
		switch record.Type {
		case "list_resource_found":
			f := record.Found
			if f == nil || f.Address == "" || f.ResourceType == "" || len(f.Identity) == 0 {
				return nil, searchDiagnosticError(queryIdentityInvalidCode)
			}
			version, ok := validIdentityVersion(f.Version)
			if !ok {
				return nil, searchDiagnosticError(queryIdentityVersionInvalidCode)
			}
			provider, ok := providers[f.ResourceType]
			if !ok || provider.Source == "" || provider.Version == "" {
				return nil, searchDiagnosticError(queryProviderSourceUnresolvedCode)
			}
			marker, err := queryResultMarker(queryID, provider, f.ResourceType, f.Identity)
			if err != nil || seen[marker] {
				return nil, searchDiagnosticError(queryDuplicateOrInvalidIdentityCode)
			}
			seen[marker] = true
			if f.Config != nil && f.Configuration != nil && *f.Config != *f.Configuration || f.ImportConfig != nil && f.ImportBlock != nil && *f.ImportConfig != *f.ImportBlock {
				return nil, searchDiagnosticError(queryGeneratedBlockConflictCode)
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
			out.Results = append(out.Results, result)
			out.ByType[f.ResourceType]++
			counts[f.Address]++
		case "list_complete":
			if record.Complete == nil || record.Complete.Address == "" || record.Complete.Total < 0 {
				return nil, searchDiagnosticError(queryEvidenceInvalidCode)
			}
			if _, ok := totals[record.Complete.Address]; ok {
				return nil, searchDiagnosticError(queryEvidenceInvalidCode)
			}
			totals[record.Complete.Address] = record.Complete.Total
			completeTypes[record.Complete.Address] = record.Complete.ResourceType
		case "diagnostic":
			if record.Diagnostic == nil || strings.ToLower(record.Diagnostic.Severity) != "warning" {
				return nil, searchDiagnosticError(queryReportedErrorCode)
			}
		case "version", "log", "list_start":
			if record.Found != nil || record.Complete != nil || record.Diagnostic != nil {
				return nil, searchDiagnosticError(queryEvidenceInvalidCode)
			}
		default:
			// Progress and policy messages do not contribute result rows. Keep
			// unknown result-bearing records nonselectable rather than mistaking
			// them for benign progress messages.
			if record.Found != nil || record.Complete != nil || record.Diagnostic != nil || strings.Contains(record.Type, "resource_found") || strings.HasPrefix(record.Type, "list_") || strings.Contains(record.Type, "diagnostic") {
				return nil, searchDiagnosticError(queryEvidenceInvalidCode)
			}
		}
	}
	if len(totals) == 0 {
		return nil, searchDiagnosticError(queryEvidenceIncompleteCode)
	}
	for addr, total := range totals {
		if counts[addr] != total {
			return nil, searchDiagnosticError(queryEvidenceIncompleteCode)
		}
	}
	for addr := range counts {
		if _, ok := totals[addr]; !ok {
			return nil, searchDiagnosticError(queryEvidenceIncompleteCode)
		}
	}
	for _, result := range out.Results {
		if kind := completeTypes[result.Address]; kind != "" && kind != result.ResourceType {
			return nil, searchDiagnosticError(queryEvidenceIncompleteCode)
		}
	}
	return out, nil
}
