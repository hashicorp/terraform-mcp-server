// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
)

const maxQuerySummaryBytes = 64 << 10
const maxQueryRowBytes = 32 << 10

type QueryDiscoveryFilter struct {
	ResourceType string
	Address      string
	NameContains string
}

type queryDiscoveryRow struct {
	Marker             string         `json:"query_result_marker"`
	DisplayName        string         `json:"display_name,omitempty"`
	Identity           map[string]any `json:"identity"`
	IdentityVersion    int            `json:"identity_version"`
	Tags               map[string]any `json:"tags,omitempty"`
	AttributesCaptured bool           `json:"attributes_captured"`
}

type queryDiscoveryGroup struct {
	Address      string              `json:"address"`
	ResourceType string              `json:"resource_type"`
	Results      []queryDiscoveryRow `json:"results"`
}

type QueryDiscoverySummary struct {
	QueryRunID             string                `json:"query_run_id"`
	Status                 string                `json:"status"`
	SourceOrganizationName string                `json:"source_organization_name,omitempty"`
	SourceWorkspaceName    string                `json:"source_workspace_name,omitempty"`
	SourceWorkspaceID      string                `json:"source_workspace_id,omitempty"`
	LogDigest              string                `json:"log_digest,omitempty"`
	Selectable             bool                  `json:"selectable"`
	ResourcesDiscovered    int                   `json:"resources_discovered,omitempty"`
	ByType                 map[string]int        `json:"by_type,omitempty"`
	TotalMatching          int                   `json:"total_matching,omitempty"`
	RowsWithoutAttributes  int                   `json:"rows_without_attributes,omitempty"`
	Returned               int                   `json:"returned,omitempty"`
	Diagnostics            []string              `json:"diagnostics,omitempty"`
	NextAction             string                `json:"next_action"`
	Lists                  []queryDiscoveryGroup `json:"lists,omitempty"`
}

// queryDiscoveryProvenance is backend-derived, never supplied by the caller.
type queryDiscoveryProvenance struct {
	QueryRunID, Organization, WorkspaceName, WorkspaceID string
	Providers                                            map[string]queryResultProvider
}

type queryBoundedWriter struct {
	buffer    bytes.Buffer
	limit     int
	overLimit bool
}

func (b *queryBoundedWriter) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		b.overLimit = true
		return 0, queryEvidenceError("query_evidence_size_limit")
	}
	return b.buffer.Write(p)
}

func readQueryBackendJSON(ctx context.Context, c *tfe.Client, endpoint string) ([]byte, error) {
	req, err := c.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, queryEvidenceError("query_backend_unavailable")
	}
	buf := &queryBoundedWriter{limit: maxQueryEvidenceBytes}
	if err := req.Do(ctx, buf); err != nil {
		return nil, queryEvidenceError("query_backend_unavailable")
	}
	return buf.buffer.Bytes(), nil
}

type queryWireRelation struct {
	Data *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"data"`
}

func readQueryDiscoveryProvenance(ctx context.Context, c *tfe.Client, queryID string) (*queryDiscoveryProvenance, error) {
	run, err := c.QueryRuns.Read(ctx, queryID)
	if err != nil {
		return nil, queryEvidenceError("query_run_unavailable")
	}
	if run == nil || run.ID != queryID || run.Workspace == nil || run.Workspace.ID == "" {
		return nil, queryEvidenceError("query_ownership_unverified")
	}
	w, err := c.Workspaces.ReadByID(ctx, run.Workspace.ID)
	if err != nil || w == nil || w.ID != run.Workspace.ID || w.Name == "" || w.Organization == nil || w.Organization.Name == "" {
		return nil, queryEvidenceError("query_ownership_unverified")
	}
	// Both server transports can authorize a token for an allowlisted org
	// without checking the derived QueryRun's org. Enforce that boundary here.
	if configured := os.Getenv(client.OrganizationAllowlistEnv); configured != "" {
		allowed, err := client.ParseOrganizationAllowlistCSV(configured)
		if err != nil {
			return nil, queryEvidenceError("query_organization_not_allowed")
		}
		match := false
		for _, name := range allowed {
			match = match || strings.EqualFold(name, w.Organization.Name)
		}
		if !match {
			return nil, queryEvidenceError("query_organization_not_allowed")
		}
	}
	prov := &queryDiscoveryProvenance{QueryRunID: queryID, Organization: w.Organization.Name, WorkspaceName: w.Name, WorkspaceID: w.ID}
	if run.Status == tfe.QueryRunErrored || run.Status == tfe.QueryRunCanceled {
		return prov, queryEvidenceError("query_not_selectable")
	}
	if run.Status != tfe.QueryRunFinished {
		return prov, queryEvidenceError("query_not_finished")
	}
	raw, err := readQueryBackendJSON(ctx, c, "queries/"+url.PathEscape(queryID))
	if err != nil {
		return nil, err
	}
	var wire struct {
		Data struct {
			ID            string `json:"id"`
			Relationships struct {
				NoCode    queryWireRelation `json:"no-code-query"`
				Workspace queryWireRelation `json:"workspace"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := decodeQueryEvidence(raw, &wire); err != nil {
		return nil, err
	}
	nc, ws := wire.Data.Relationships.NoCode.Data, wire.Data.Relationships.Workspace.Data
	if wire.Data.ID != queryID || nc == nil || nc.ID == "" || nc.Type != "no-code-queries" || ws == nil || ws.ID != w.ID {
		return nil, queryEvidenceError("query_provenance_unavailable")
	}
	raw, err = readQueryBackendJSON(ctx, c, "search/no-code-query/"+url.PathEscape(nc.ID))
	if err != nil {
		return nil, err
	}
	var stored struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				Providers []struct {
					Namespace string `json:"namespace"`
					Name      string `json:"name"`
					Version   string `json:"version"`
					Resources []struct {
						Body struct {
							Type       string `json:"resource_type"`
							HyphenType string `json:"resource-type"`
						} `json:"body"`
					} `json:"no-code-query-resources"`
				} `json:"no-code-query-providers"`
			} `json:"attributes"`
			Relationships struct {
				Workspace queryWireRelation `json:"workspace"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if decodeQueryEvidence(raw, &stored) != nil || stored.Data.ID != nc.ID || stored.Data.Relationships.Workspace.Data == nil || stored.Data.Relationships.Workspace.Data.ID != w.ID {
		return nil, queryEvidenceError("query_provenance_unavailable")
	}
	providers := map[string]queryResultProvider{}
	for _, p := range stored.Data.Attributes.Providers {
		if p.Namespace == "" || p.Name == "" || p.Version == "" {
			return nil, queryEvidenceError("query_provider_source_unresolved")
		}
		provider := queryResultProvider{Source: "registry.terraform.io/" + p.Namespace + "/" + p.Name, Version: p.Version}
		for _, r := range p.Resources {
			kind := r.Body.Type
			if kind == "" {
				kind = r.Body.HyphenType
			}
			if kind == "" || r.Body.Type != "" && r.Body.HyphenType != "" && r.Body.Type != r.Body.HyphenType {
				return nil, queryEvidenceError("query_provider_source_unresolved")
			}
			if previous, exists := providers[kind]; exists && previous != provider {
				return nil, queryEvidenceError("query_provider_ambiguous")
			}
			providers[kind] = provider
		}
	}
	if len(providers) == 0 {
		return nil, queryEvidenceError("query_provider_source_unresolved")
	}
	prov.Providers = providers
	return prov, nil
}

func readQueryDiscovery(ctx context.Context, c *tfe.Client, queryID string) (*queryDiscoveryProvenance, *queryEvidence, error) {
	prov, err := readQueryDiscoveryProvenance(ctx, c, queryID)
	if err != nil {
		return prov, nil, err
	}
	reader, err := c.QueryRuns.Logs(ctx, queryID)
	if err != nil {
		return prov, nil, queryEvidenceError("query_log_unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxQueryEvidenceBytes+1))
	if err != nil {
		return prov, nil, queryEvidenceError("query_log_unavailable")
	}
	if len(data) > maxQueryEvidenceBytes {
		return prov, nil, queryEvidenceError("query_evidence_size_limit")
	}
	evidence, err := parseQueryEvidence(queryID, data, prov.Providers)
	if err != nil {
		return prov, nil, err
	}
	return prov, evidence, nil
}

func queryDiscoveryTags(value map[string]any) map[string]any {
	raw, ok := value["tags"].(map[string]any)
	if !ok {
		return nil
	}
	tags := make(map[string]any)
	for key, tag := range raw {
		switch tag.(type) {
		case string, bool, json.Number:
			tags[key] = tag
		}
	}
	return tags
}

func queryDiscoveryResultRow(result queryResultEvidence) queryDiscoveryRow {
	return queryDiscoveryRow{Marker: result.Marker, DisplayName: result.DisplayName, Identity: result.Identity, IdentityVersion: result.IdentityVersion, Tags: queryDiscoveryTags(result.ResourceObject), AttributesCaptured: result.ResourceObject != nil}
}

// Do not expose signed artifact capabilities hidden in provider-supplied
// display names, identity keys/values or tags. An ordinary resource URL
// without signed-capability evidence is not rejected just for being a URL.
func unsafeDiscoveryValue(value any) bool {
	switch v := value.(type) {
	case string:
		for _, part := range strings.Fields(v) {
			part = strings.Trim(part, `"'()<>[],;`)
			u, err := url.Parse(part)
			if err != nil || u.Scheme != "https" && u.Scheme != "http" {
				continue
			}
			if u.User != nil || strings.Contains(strings.ToLower(u.Hostname()), "archivist") {
				return true
			}
			for key := range u.Query() {
				switch strings.ToLower(key) {
				case "x-amz-signature", "x-goog-signature", "signature", "sig", "token", "access_token", "x-amz-credential":
					return true
				}
			}
		}
	case map[string]any:
		for key, entry := range v {
			if unsafeDiscoveryValue(key) || unsafeDiscoveryValue(entry) {
				return true
			}
		}
	case []any:
		for _, entry := range v {
			if unsafeDiscoveryValue(entry) {
				return true
			}
		}
	}
	return false
}

func validateQueryDiscoveryEvidence(evidence *queryEvidence) error {
	// Check every record before returning any selectable summary. A later
	// oversized or sensitive row cannot be hidden by presentation filters.
	for _, result := range evidence.Results {
		row := queryDiscoveryResultRow(result)
		if unsafeDiscoveryValue(result.Address) || unsafeDiscoveryValue(result.ResourceType) || unsafeDiscoveryValue(row.DisplayName) || unsafeDiscoveryValue(row.Identity) || unsafeDiscoveryValue(row.Tags) {
			return queryEvidenceError("query_sensitive_result")
		}
		encoded, err := json.Marshal(row)
		if err != nil || len(encoded) > maxQueryRowBytes {
			return queryEvidenceError("query_summary_oversized")
		}
	}
	return nil
}

func summarizeQueryDiscovery(prov *queryDiscoveryProvenance, evidence *queryEvidence, filter QueryDiscoveryFilter) (*QueryDiscoverySummary, error) {
	if err := validateQueryDiscoveryEvidence(evidence); err != nil {
		return nil, err
	}
	matches := func(result queryResultEvidence) bool {
		return (filter.ResourceType == "" || filter.ResourceType == result.ResourceType) &&
			(filter.Address == "" || filter.Address == result.Address) &&
			(filter.NameContains == "" || strings.Contains(strings.ToLower(result.DisplayName), strings.ToLower(filter.NameContains)))
	}
	summary := &QueryDiscoverySummary{QueryRunID: prov.QueryRunID, Status: "selectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, LogDigest: evidence.LogDigest, Selectable: true, ResourcesDiscovered: len(evidence.Results), ByType: evidence.ByType, Lists: []queryDiscoveryGroup{}}
	groups := map[string]int{}
	for _, result := range evidence.Results {
		if !matches(result) {
			continue
		}
		summary.TotalMatching++
		if result.ResourceObject == nil {
			summary.RowsWithoutAttributes++
		}
		row := queryDiscoveryResultRow(result)
		key := result.Address + "\x00" + result.ResourceType
		index, exists := groups[key]
		if !exists {
			index = len(summary.Lists)
			groups[key] = index
			summary.Lists = append(summary.Lists, queryDiscoveryGroup{Address: result.Address, ResourceType: result.ResourceType, Results: []queryDiscoveryRow{}})
		}
		summary.Lists[index].Results = append(summary.Lists[index].Results, row)
		summary.Returned++
	}
	if filter.ResourceType != "" || filter.Address != "" || filter.NameContains != "" {
		summary.NextAction = "These are all results matching the supplied summary filters, not all results of the query or all cloud resources. Choose query_result_marker values with the user only if import is requested."
	} else {
		summary.NextAction = "These are all results of this query, not all cloud resources. Choose query_result_marker values with the user only if import is requested."
	}
	summary.NextAction += " Keep each shown result's query_result_marker, display_name and any tags, including in script output; tag filtering is not supported. Missing tags do not prove a resource is untagged."
	if summary.RowsWithoutAttributes > 0 {
		summary.NextAction += " For rows with attributes_captured false, tags are unknown. If tags matter, ask before a new query (for example with generate_config_out true); it creates new markers and may still lack tags."
	}
	data, err := json.Marshal(summary)
	if err != nil || len(data) > maxQuerySummaryBytes {
		return nil, queryEvidenceError("query_summary_oversized")
	}
	return summary, nil
}

// ReadQueryDiscoverySummary shares Search discovery across the two MCP SDKs.
// Only a safely observed completed QueryRun with invalid result evidence can
// return a factual nonselectable result; backend/transport failures are errors.
func ReadQueryDiscoverySummary(ctx context.Context, c *tfe.Client, queryID string, filter QueryDiscoveryFilter) (*QueryDiscoverySummary, error) {
	if !validQueryRunID(queryID) {
		return nil, queryEvidenceError("query_run_id_invalid")
	}
	prov, evidence, err := readQueryDiscovery(ctx, c, queryID)
	if err != nil {
		if prov != nil {
			switch err.Error() {
			case "query_not_selectable", "query_identity_version_invalid", "query_evidence_incomplete", "query_duplicate_or_invalid_identity", "query_identity_invalid", "query_reported_error", "query_generated_block_conflict":
				return &QueryDiscoverySummary{QueryRunID: queryID, Status: "nonselectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, Selectable: false, Diagnostics: []string{err.Error()}, NextAction: "This QueryRun does not have complete selectable evidence. Do not select or import from it; review the query and ask before starting another."}, nil
			}
		}
		return nil, err
	}
	summary, err := summarizeQueryDiscovery(prov, evidence, filter)
	if err != nil && err.Error() == "query_summary_oversized" {
		return &QueryDiscoverySummary{QueryRunID: queryID, Status: "nonselectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, Selectable: false, ResourcesDiscovered: len(evidence.Results), ByType: evidence.ByType, Diagnostics: []string{"query_summary_oversized"}, NextAction: "The complete query summary exceeds the response limit. Do not select or import from a partial result; use narrower summary filters or refine and run a narrower Search query."}, nil
	}
	return summary, err
}

func FormatQueryDiscoverySummary(summary *QueryDiscoverySummary) (string, error) {
	data, err := json.Marshal(summary)
	if err != nil || len(data) > maxQuerySummaryBytes {
		return "", queryEvidenceError("query_summary_oversized")
	}
	return string(data), nil
}
