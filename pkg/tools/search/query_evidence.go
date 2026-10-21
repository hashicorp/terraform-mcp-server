// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bufio"
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
	Configuration   string              `json:"configuration,omitempty"`
	ImportConfig    string              `json:"import_configuration,omitempty"`
}

type queryEvidence struct {
	Results []queryResultEvidence
	ByType  map[string]int
}

// queryEvidenceCollector receives each result only after the shared parser has
// validated its identity, provider provenance, marker, and generated-block
// aliases. Collectors decide which validated values must survive the current
// tool call; whole-query validation remains parser-owned.
type queryEvidenceCollector interface {
	CollectQueryEvidence(queryResultEvidence) error
	FinishQueryEvidence() error
}

type allQueryEvidenceCollector struct {
	evidence *queryEvidence
}

func newAllQueryEvidenceCollector() *allQueryEvidenceCollector {
	return &allQueryEvidenceCollector{evidence: &queryEvidence{Results: []queryResultEvidence{}, ByType: map[string]int{}}}
}

func (c *allQueryEvidenceCollector) CollectQueryEvidence(result queryResultEvidence) error {
	c.evidence.Results = append(c.evidence.Results, result)
	c.evidence.ByType[result.ResourceType]++
	return nil
}

func (c *allQueryEvidenceCollector) FinishQueryEvidence() error { return nil }

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

// queryLogReadBufferSize controls go-tfe LogReader range-request size. It is
// not an artifact or NDJSON record-size limit.
const queryLogReadBufferSize = 64 * 1024

// maxQueryLogBytes is a temporary outer safety ceiling for decoded QueryRun
// log bytes. It bounds remote evidence consumption; it is not a product result
// limit, retained-heap guarantee, or MCP response-size guarantee.
const maxQueryLogBytes int64 = 128 << 20

type queryLogLimitReader struct {
	reader    io.Reader
	remaining int64
}

func newQueryLogLimitReader(reader io.Reader, limit int64) io.Reader {
	return &queryLogLimitReader{reader: reader, remaining: limit}
}

func (r *queryLogLimitReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, searchDiagnosticError(queryLogSizeLimitCode)
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	if int64(n) > r.remaining {
		return 0, searchDiagnosticError(queryLogSizeLimitCode)
	}
	r.remaining -= int64(n)
	return n, err
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
	return parseQueryEvidenceReader(queryID, bytes.NewReader(data), providers)
}

type queryEvidenceParser struct {
	queryID       string
	providers     map[string]queryResultProvider
	collector     queryEvidenceCollector
	counts        map[string]int
	totals        map[string]int
	seen          map[string]bool
	completeTypes map[string]string
	foundTypes    map[string]string
	mixedTypes    map[string]bool
}

func newQueryEvidenceParser(queryID string, providers map[string]queryResultProvider, collector queryEvidenceCollector) (*queryEvidenceParser, error) {
	if queryID == "" || collector == nil {
		return nil, searchDiagnosticError(queryEvidenceInvalidCode)
	}
	return &queryEvidenceParser{
		queryID:       queryID,
		providers:     providers,
		collector:     collector,
		counts:        map[string]int{},
		totals:        map[string]int{},
		seen:          map[string]bool{},
		completeTypes: map[string]string{},
		foundTypes:    map[string]string{},
		mixedTypes:    map[string]bool{},
	}, nil
}

// parseQueryEvidenceReader processes QueryRun NDJSON one record at a time. The
// explicit buffer size keeps go-tfe LogReader range requests efficient without
// imposing an artifact or record-size limit.
func parseQueryEvidenceReader(queryID string, reader io.Reader, providers map[string]queryResultProvider) (*queryEvidence, error) {
	collector := newAllQueryEvidenceCollector()
	err := parseQueryEvidenceReaderWithCollector(queryID, reader, providers, collector)
	if err != nil {
		return nil, err
	}
	return collector.evidence, nil
}

func parseQueryEvidenceReaderWithCollector(queryID string, reader io.Reader, providers map[string]queryResultProvider, collector queryEvidenceCollector) error {
	parser, err := newQueryEvidenceParser(queryID, providers, collector)
	if err != nil {
		return err
	}
	buffered := bufio.NewReaderSize(reader, queryLogReadBufferSize)
	for {
		line, readErr := buffered.ReadBytes('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			if _, ok := searchDiagnosticCodeFromError(readErr); ok {
				return readErr
			}
			return searchDiagnosticErrorWithCause(queryLogUnavailableCode, readErr)
		}
		if err := parser.consumeLine(line); err != nil {
			return err
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	return parser.finish()
}

// consumeLine separates ordinary QueryRun log text from Terraform's NDJSON
// records. Result-bearing text that is damaged or structurally unexpected is
// rejected rather than silently treated as progress output.
func (p *queryEvidenceParser) consumeLine(line []byte) error {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}
	if !bytes.HasPrefix(line, []byte{'{'}) {
		if bytes.HasPrefix(line, []byte{'['}) || bytes.Contains(line, []byte("list_resource_found")) || bytes.Contains(line, []byte("list_complete")) || bytes.Contains(line, []byte("diagnostic")) {
			return searchDiagnosticError(queryEvidenceInvalidCode)
		}
		return nil
	}
	var record queryLogRecord
	if decodeQueryEvidence(line, &record) != nil {
		return searchDiagnosticError(queryEvidenceInvalidCode)
	}
	return p.consumeRecord(record)
}

func (p *queryEvidenceParser) consumeRecord(record queryLogRecord) error {
	switch record.Type {
	case "list_resource_found":
		return p.consumeFound(record.Found)
	case "list_complete":
		return p.consumeCompletion(record.Complete)
	case "diagnostic":
		return p.consumeDiagnostic(record.Diagnostic)
	case "version", "log", "list_start":
		if record.Found != nil || record.Complete != nil || record.Diagnostic != nil {
			return searchDiagnosticError(queryEvidenceInvalidCode)
		}
		return nil
	default:
		// Progress and policy messages do not contribute result rows. Keep
		// unknown result-bearing records nonselectable rather than mistaking
		// them for benign progress messages.
		if record.Found != nil || record.Complete != nil || record.Diagnostic != nil || strings.Contains(record.Type, "resource_found") || strings.HasPrefix(record.Type, "list_") || strings.Contains(record.Type, "diagnostic") {
			return searchDiagnosticError(queryEvidenceInvalidCode)
		}
		return nil
	}
}

func (p *queryEvidenceParser) consumeFound(found *queryFoundResource) error {
	if found == nil || found.Address == "" || found.ResourceType == "" || len(found.Identity) == 0 {
		return searchDiagnosticError(queryIdentityInvalidCode)
	}
	version, ok := validIdentityVersion(found.Version)
	if !ok {
		return searchDiagnosticError(queryIdentityVersionInvalidCode)
	}
	provider, ok := p.providers[found.ResourceType]
	if !ok || provider.Source == "" || provider.Version == "" {
		return searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	marker, err := queryResultMarker(p.queryID, provider, found.ResourceType, found.Identity)
	if err != nil || p.seen[marker] {
		return searchDiagnosticError(queryDuplicateOrInvalidIdentityCode)
	}
	p.seen[marker] = true
	if found.Config != nil && found.Configuration != nil && *found.Config != *found.Configuration || found.ImportConfig != nil && found.ImportBlock != nil && *found.ImportConfig != *found.ImportBlock {
		return searchDiagnosticError(queryGeneratedBlockConflictCode)
	}
	result := queryResultEvidence{
		Marker: marker, Address: found.Address, ResourceType: found.ResourceType,
		DisplayName: found.DisplayName, Identity: found.Identity, IdentityVersion: version,
		Provider: provider,
	}
	if found.Config != nil && strings.TrimSpace(*found.Config) != "" {
		result.Configuration = *found.Config
	} else if found.Configuration != nil && strings.TrimSpace(*found.Configuration) != "" {
		result.Configuration = *found.Configuration
	}
	if found.ImportConfig != nil {
		result.ImportConfig = *found.ImportConfig
	} else if found.ImportBlock != nil {
		result.ImportConfig = *found.ImportBlock
	}
	if previous := p.foundTypes[found.Address]; previous == "" {
		p.foundTypes[found.Address] = found.ResourceType
	} else if previous != found.ResourceType {
		p.mixedTypes[found.Address] = true
	}
	p.counts[found.Address]++
	return p.collector.CollectQueryEvidence(result)
}

func (p *queryEvidenceParser) consumeCompletion(complete *queryListCompletion) error {
	if complete == nil || complete.Address == "" || complete.Total < 0 {
		return searchDiagnosticError(queryEvidenceInvalidCode)
	}
	if _, ok := p.totals[complete.Address]; ok {
		return searchDiagnosticError(queryEvidenceInvalidCode)
	}
	p.totals[complete.Address] = complete.Total
	p.completeTypes[complete.Address] = complete.ResourceType
	return nil
}

func (p *queryEvidenceParser) consumeDiagnostic(diagnostic *queryLogDiagnostic) error {
	if diagnostic == nil || strings.ToLower(diagnostic.Severity) != "warning" {
		return searchDiagnosticError(queryReportedErrorCode)
	}
	return nil
}

func (p *queryEvidenceParser) finish() error {
	if len(p.totals) == 0 {
		return searchDiagnosticError(queryEvidenceIncompleteCode)
	}
	for addr, total := range p.totals {
		if p.counts[addr] != total {
			return searchDiagnosticError(queryEvidenceIncompleteCode)
		}
	}
	for addr := range p.counts {
		if _, ok := p.totals[addr]; !ok {
			return searchDiagnosticError(queryEvidenceIncompleteCode)
		}
	}
	for addr, kind := range p.completeTypes {
		if kind != "" && p.counts[addr] > 0 && (p.mixedTypes[addr] || p.foundTypes[addr] != kind) {
			return searchDiagnosticError(queryEvidenceIncompleteCode)
		}
	}
	return p.collector.FinishQueryEvidence()
}
