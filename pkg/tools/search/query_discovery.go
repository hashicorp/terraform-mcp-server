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
	"strings"

	"github.com/hashicorp/go-tfe"
)

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
	ObservedAttributes map[string]any `json:"observed_attributes,omitempty"`
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
	Selectable             bool                  `json:"selectable"`
	ResourcesDiscovered    int                   `json:"resources_discovered,omitempty"`
	ByType                 map[string]int        `json:"by_type,omitempty"`
	TotalMatching          int                   `json:"total_matching,omitempty"`
	RowsWithoutAttributes  int                   `json:"rows_without_attributes,omitempty"`
	Diagnostics            []string              `json:"diagnostics,omitempty"`
	NextAction             string                `json:"next_action"`
	Lists                  []queryDiscoveryGroup `json:"lists,omitempty"`
}

// queryDiscoveryProvenance is backend-derived, never supplied by the caller.
type queryDiscoveryProvenance struct {
	QueryRunID, Organization, WorkspaceName, WorkspaceID string
	Providers                                            map[string]queryResultProvider
}

func readQueryBackendJSON(ctx context.Context, c *tfe.Client, endpoint string) ([]byte, error) {
	req, err := c.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, searchDiagnosticError(queryBackendUnavailableCode)
	}
	var buf bytes.Buffer
	if err := req.Do(ctx, &buf); err != nil {
		return nil, searchDiagnosticError(queryBackendUnavailableCode)
	}
	return buf.Bytes(), nil
}

type queryWireRelation struct {
	Data *queryWireResourceIdentifier `json:"data"`
}

type queryWireResourceIdentifier struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type queryRunProvenanceResponse struct {
	Data queryRunProvenanceData `json:"data"`
}

type queryRunProvenanceData struct {
	ID            string                `json:"id"`
	Relationships queryRunRelationships `json:"relationships"`
}

type queryRunRelationships struct {
	NoCode    queryWireRelation `json:"no-code-query"`
	Workspace queryWireRelation `json:"workspace"`
}

type queryStoredNoCodeResponse struct {
	Data queryStoredNoCodeData `json:"data"`
}

type queryStoredNoCodeData struct {
	ID            string                         `json:"id"`
	Attributes    queryStoredNoCodeAttributes    `json:"attributes"`
	Relationships queryStoredNoCodeRelationships `json:"relationships"`
}

type queryStoredNoCodeAttributes struct {
	Providers []queryStoredProvider `json:"no-code-query-providers"`
}

type queryStoredNoCodeRelationships struct {
	Workspace queryWireRelation `json:"workspace"`
}

type queryStoredProvider struct {
	Namespace string                `json:"namespace"`
	Name      string                `json:"name"`
	Version   string                `json:"version"`
	Resources []queryStoredResource `json:"no-code-query-resources"`
}

type queryStoredResource struct {
	Body queryStoredResourceBody `json:"body"`
}

type queryStoredResourceBody struct {
	Type       string `json:"resource_type"`
	HyphenType string `json:"resource-type"`
}

func readQueryDiscoveryProvenance(ctx context.Context, c *tfe.Client, queryID string) (*queryDiscoveryProvenance, error) {
	run, err := c.QueryRuns.Read(ctx, queryID)
	if err != nil {
		return nil, searchDiagnosticError(queryRunUnavailableCode)
	}
	w, err := readQueryRunWorkspaceWithOrgPolicy(ctx, ctx, c, queryID, run)
	if err != nil {
		return nil, err
	}
	if w.Name == "" {
		return nil, searchDiagnosticError(queryOwnershipUnverifiedCode)
	}
	prov := &queryDiscoveryProvenance{QueryRunID: queryID, Organization: w.Organization.Name, WorkspaceName: w.Name, WorkspaceID: w.ID}
	if run.Status == tfe.QueryRunErrored || run.Status == tfe.QueryRunCanceled {
		return prov, searchDiagnosticError(queryNotSelectableCode)
	}
	if run.Status != tfe.QueryRunFinished {
		return prov, searchDiagnosticError(queryNotFinishedCode)
	}
	raw, err := readQueryBackendJSON(ctx, c, "queries/"+url.PathEscape(queryID))
	if err != nil {
		return nil, err
	}
	var wire queryRunProvenanceResponse
	if err := decodeQueryEvidence(raw, &wire); err != nil {
		return nil, err
	}
	nc, ws := wire.Data.Relationships.NoCode.Data, wire.Data.Relationships.Workspace.Data
	if wire.Data.ID != queryID || nc == nil || strings.TrimSpace(nc.ID) == "" || nc.Type != "no-code-queries" || ws == nil || ws.ID != w.ID {
		return nil, searchDiagnosticError(queryProvenanceUnavailableCode)
	}
	raw, err = readQueryBackendJSON(ctx, c, "search/no-code-query/"+url.PathEscape(nc.ID))
	if err != nil {
		return nil, err
	}
	var stored queryStoredNoCodeResponse
	if decodeQueryEvidence(raw, &stored) != nil || stored.Data.ID != nc.ID || stored.Data.Relationships.Workspace.Data == nil || stored.Data.Relationships.Workspace.Data.ID != w.ID {
		return nil, searchDiagnosticError(queryProvenanceUnavailableCode)
	}
	providers := map[string]queryResultProvider{}
	for _, p := range stored.Data.Attributes.Providers {
		if p.Namespace == "" || p.Name == "" || p.Version == "" {
			return nil, searchDiagnosticError(queryProviderSourceUnresolvedCode)
		}
		provider := queryResultProvider{Source: "registry.terraform.io/" + p.Namespace + "/" + p.Name, Version: p.Version}
		for _, r := range p.Resources {
			kind := r.Body.Type
			if kind == "" {
				kind = r.Body.HyphenType
			}
			if kind == "" || r.Body.Type != "" && r.Body.HyphenType != "" && r.Body.Type != r.Body.HyphenType {
				return nil, searchDiagnosticError(queryProviderSourceUnresolvedCode)
			}
			if previous, exists := providers[kind]; exists && previous != provider {
				return nil, searchDiagnosticError(queryProviderAmbiguousCode)
			}
			providers[kind] = provider
		}
	}
	if len(providers) == 0 {
		return nil, searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	prov.Providers = providers
	return prov, nil
}

func readQueryDiscovery(ctx context.Context, c *tfe.Client, queryID string) (*queryDiscoveryProvenance, *queryEvidence, error) {
	prov, err := readQueryDiscoveryProvenance(ctx, c, queryID)
	if err != nil {
		return prov, nil, err
	}
	evidence, err := readQueryEvidence(ctx, c, queryID, prov)
	if err != nil {
		return prov, nil, err
	}
	return prov, evidence, nil
}

// readQueryEvidence reads and validates a QueryRun log using already-verified
// backend provenance. Callers that have resolved provenance should reuse it
// rather than repeat the QueryRun, workspace, and stored-query reads.
func readQueryEvidence(ctx context.Context, c *tfe.Client, queryID string, prov *queryDiscoveryProvenance) (*queryEvidence, error) {
	reader, err := c.QueryRuns.Logs(ctx, queryID)
	if err != nil {
		return nil, searchDiagnosticError(queryLogUnavailableCode)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, searchDiagnosticError(queryLogUnavailableCode)
	}
	evidence, err := parseQueryEvidence(queryID, data, prov.Providers)
	if err != nil {
		return nil, err
	}
	return evidence, nil
}

func queryDiscoveryObservedAttributes(provider queryResultProvider, value map[string]any) map[string]any {
	// This is an intentionally small provider-qualified metadata projection,
	// not a general claim that arbitrary resource attributes are safe to return.
	var names []string
	switch strings.ToLower(provider.Source) {
	case "registry.terraform.io/hashicorp/aws":
		names = []string{"tags", "tags_all"}
	case "registry.terraform.io/hashicorp/azurerm":
		names = []string{"tags"}
	case "registry.terraform.io/hashicorp/google", "registry.terraform.io/hashicorp/google-beta":
		names = []string{"labels", "terraform_labels", "effective_labels", "resource_manager_tags"}
	default:
		return nil
	}

	observed := make(map[string]any)
	for _, name := range names {
		raw, ok := value[name].(map[string]any)
		if !ok {
			continue
		}
		scalars := make(map[string]any)
		for key, item := range raw {
			switch item.(type) {
			case string, bool, json.Number:
				scalars[key] = item
			}
		}
		if len(scalars) > 0 {
			observed[name] = scalars
		}
	}
	return observed
}

func queryDiscoveryResultRow(result queryResultEvidence) queryDiscoveryRow {
	return queryDiscoveryRow{Marker: result.Marker, DisplayName: result.DisplayName, Identity: result.Identity, IdentityVersion: result.IdentityVersion, ObservedAttributes: queryDiscoveryObservedAttributes(result.Provider, result.ResourceObject), AttributesCaptured: result.ResourceObject != nil}
}

func summarizeQueryDiscovery(prov *queryDiscoveryProvenance, evidence *queryEvidence, filter QueryDiscoveryFilter) (*QueryDiscoverySummary, error) {
	matches := func(result queryResultEvidence) bool {
		return (filter.ResourceType == "" || filter.ResourceType == result.ResourceType) &&
			(filter.Address == "" || filter.Address == result.Address) &&
			(filter.NameContains == "" || strings.Contains(strings.ToLower(result.DisplayName), strings.ToLower(filter.NameContains)))
	}
	summary := &QueryDiscoverySummary{QueryRunID: prov.QueryRunID, Status: "selectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, Selectable: true, ResourcesDiscovered: len(evidence.Results), ByType: evidence.ByType, Lists: []queryDiscoveryGroup{}}
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
	}
	summary.NextAction = "For a Search-only request, report these results and stop; do not request a target workspace or local path. If import is requested, ask the user to select one or more displayed query_result_marker values. Keep each displayed marker, display_name and available observed_attributes together; supplied filters affect presentation only, and missing projected metadata does not prove the resource lacks it."
	if summary.RowsWithoutAttributes > 0 {
		summary.NextAction += " Some matching rows have attributes_captured false, so additional result data is unknown. If that data is necessary for selection, ask before creating another QueryRun. If approved and not already enabled, repeat the same schema-backed query with generate_config_out true, preserving supported filters and omitting unsupported conditions. This requests provider resource objects and generated HCL but does not guarantee capture; the new QueryRun has new markers, and this summary exposes only its declared projections rather than arbitrary resource attributes."
	}
	return summary, nil
}

// nonselectableQueryEvidenceCode identifies evidence failures that safely produce a factual nonselectable result.
func nonselectableQueryEvidenceCode(err error) (searchDiagnosticCode, bool) {
	code, ok := searchDiagnosticCodeFromError(err)
	if !ok {
		return "", false
	}
	switch code {
	case queryNotSelectableCode,
		queryIdentityVersionInvalidCode,
		queryEvidenceIncompleteCode,
		queryDuplicateOrInvalidIdentityCode,
		queryIdentityInvalidCode,
		queryReportedErrorCode,
		queryGeneratedBlockConflictCode:
		return code, true
	default:
		return "", false
	}
}

func nonselectableQueryNextAction(code searchDiagnosticCode) string {
	switch code {
	case queryNotSelectableCode:
		return "Report that this QueryRun failed or was canceled and has no selectable result. Do not import from it; ask the user before starting another query."
	case queryIdentityVersionInvalidCode:
		return "The entire QueryRun is nonselectable because identity-version evidence is invalid. Do not select any result; review with the user before another query."
	case queryEvidenceIncompleteCode:
		return "Required whole-query evidence is incomplete. Do not present a prefix as complete or select from it; ask the user before another query."
	case queryDuplicateOrInvalidIdentityCode:
		return "Duplicate or conflicting identity evidence makes the entire QueryRun nonselectable. Stop selection and ask the user before another query."
	case queryIdentityInvalidCode:
		return "Required result identity evidence is invalid. Stop selection, do not import, and ask the user before another query."
	case queryReportedErrorCode:
		return "Report that the QueryRun output recorded an error and has no selectable result. Review it with the user before another query."
	case queryGeneratedBlockConflictCode:
		return "Generated configuration evidence conflicts. Stop selection and review the query configuration with the user before another query."
	default:
		return "This QueryRun has no selectable result. Do not import from it; review with the user before another query."
	}
}

// ReadQueryDiscoverySummary shares Search discovery across the two MCP SDKs.
// Only a safely observed completed QueryRun with invalid result evidence can
// return a factual nonselectable result; backend/transport failures are errors.
func ReadQueryDiscoverySummary(ctx context.Context, c *tfe.Client, queryID string, filter QueryDiscoveryFilter) (*QueryDiscoverySummary, error) {
	queryID = strings.TrimSpace(queryID)
	if queryID == "" {
		return nil, searchDiagnosticError(queryRunIDInvalidCode)
	}
	prov, evidence, err := readQueryDiscovery(ctx, c, queryID)
	if err != nil {
		if prov != nil {
			if code, ok := nonselectableQueryEvidenceCode(err); ok {
				return &QueryDiscoverySummary{QueryRunID: queryID, Status: "nonselectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, Selectable: false, Diagnostics: []string{string(code)}, NextAction: nonselectableQueryNextAction(code)}, nil
			}
		}
		return nil, err
	}
	return summarizeQueryDiscovery(prov, evidence, filter)
}

func FormatQueryDiscoverySummary(summary *QueryDiscoverySummary) (string, error) {
	data, err := json.Marshal(summary)
	if err != nil {
		return "", searchDiagnosticError(querySummaryUnserializableCode)
	}
	return string(data), nil
}
