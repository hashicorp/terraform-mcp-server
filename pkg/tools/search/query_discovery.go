// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-tfe"
	tfaddr "github.com/hashicorp/terraform-registry-address"
)

type QueryDiscoveryFilter struct {
	ResourceType string
	Address      string
	NameContains string
}

type queryDiscoveryRow struct {
	Marker                  string         `json:"query_result_marker"`
	DisplayName             string         `json:"display_name,omitempty"`
	Identity                map[string]any `json:"identity"`
	IdentityVersion         int            `json:"identity_version"`
	ConfigurationGenerated  bool           `json:"configuration_generated"`
	ConfigurationAttributes map[string]any `json:"configuration_attributes,omitempty"`
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
	GenerateConfigOut      bool                  `json:"generate_config_out"`
	Selectable             bool                  `json:"selectable"`
	ResourcesDiscovered    int                   `json:"resources_discovered,omitempty"`
	ByType                 map[string]int        `json:"by_type,omitempty"`
	TotalMatching          int                   `json:"total_matching,omitempty"`
	Diagnostics            []string              `json:"diagnostics,omitempty"`
	NextAction             string                `json:"next_action"`
	Lists                  []queryDiscoveryGroup `json:"lists,omitempty"`
}

// queryDiscoveryProvenance is backend-derived, never supplied by the caller.
type queryDiscoveryProvenance struct {
	QueryRunID, Organization, WorkspaceName, WorkspaceID string
	GenerateConfigOut                                    bool
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
	ID            string                       `json:"id"`
	Attributes    queryRunProvenanceAttributes `json:"attributes"`
	Relationships queryRunRelationships        `json:"relationships"`
}

type queryRunProvenanceAttributes struct {
	GenerateConfigOut *bool `json:"generate-config-out"`
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

func canonicalQueryProviderSource(namespace, providerType string) (string, error) {
	namespace, err := tfaddr.ParseProviderPart(namespace)
	if err != nil {
		return "", searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	providerType, err = tfaddr.ParseProviderPart(providerType)
	if err != nil {
		return "", searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	provider := tfaddr.NewProvider(tfaddr.DefaultProviderRegistryHost, namespace, providerType)
	if err := tfaddr.ValidateProviderAddress(provider.String()); err != nil {
		return "", searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	return provider.String(), nil
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
	if wire.Data.ID != queryID || wire.Data.Attributes.GenerateConfigOut == nil || nc == nil || strings.TrimSpace(nc.ID) == "" || nc.Type != "no-code-queries" || ws == nil || ws.ID != w.ID {
		return nil, searchDiagnosticError(queryProvenanceUnavailableCode)
	}
	prov.GenerateConfigOut = *wire.Data.Attributes.GenerateConfigOut
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
		source, err := canonicalQueryProviderSource(p.Namespace, p.Name)
		if err != nil || p.Version == "" {
			return nil, searchDiagnosticError(queryProviderSourceUnresolvedCode)
		}
		provider := queryResultProvider{Source: source, Version: p.Version}
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
		return nil, searchDiagnosticErrorWithCause(queryLogUnavailableCode, err)
	}
	evidence, err := parseQueryEvidenceReader(queryID, reader, prov.Providers)
	if err != nil {
		return nil, err
	}
	if err := validateGeneratedConfigurationEvidence(prov.GenerateConfigOut, evidence); err != nil {
		return nil, err
	}
	return evidence, nil
}

func validateGeneratedConfigurationEvidence(generateConfigOut bool, evidence *queryEvidence) error {
	if evidence == nil {
		return searchDiagnosticError(queryEvidenceInvalidCode)
	}
	if !generateConfigOut {
		for _, result := range evidence.Results {
			if strings.TrimSpace(result.Configuration) != "" {
				return searchDiagnosticError(queryGeneratedConfigurationUnexpectedCode)
			}
		}
	}
	return nil
}

func queryDiscoveryResultRow(result queryResultEvidence) (queryDiscoveryRow, error) {
	generated := strings.TrimSpace(result.Configuration) != ""
	row := queryDiscoveryRow{Marker: result.Marker, DisplayName: result.DisplayName, Identity: result.Identity, IdentityVersion: result.IdentityVersion, ConfigurationGenerated: generated}
	if generated {
		attributes, err := projectGeneratedConfiguration(result.Configuration)
		if err != nil {
			return queryDiscoveryRow{}, err
		}
		row.ConfigurationAttributes = attributes
	}
	return row, nil
}

func summarizeQueryDiscovery(prov *queryDiscoveryProvenance, evidence *queryEvidence, filter QueryDiscoveryFilter) (*QueryDiscoverySummary, error) {
	matches := func(result queryResultEvidence) bool {
		return (filter.ResourceType == "" || filter.ResourceType == result.ResourceType) &&
			(filter.Address == "" || filter.Address == result.Address) &&
			(filter.NameContains == "" || strings.Contains(strings.ToLower(result.DisplayName), strings.ToLower(filter.NameContains)))
	}
	summary := &QueryDiscoverySummary{QueryRunID: prov.QueryRunID, Status: "selectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, GenerateConfigOut: prov.GenerateConfigOut, Selectable: true, ResourcesDiscovered: len(evidence.Results), ByType: evidence.ByType, Lists: []queryDiscoveryGroup{}}
	groups := map[string]int{}
	for _, result := range evidence.Results {
		if !matches(result) {
			continue
		}
		summary.TotalMatching++
		row, err := queryDiscoveryResultRow(result)
		if err != nil {
			return nil, err
		}
		key := result.Address + "\x00" + result.ResourceType
		index, exists := groups[key]
		if !exists {
			index = len(summary.Lists)
			groups[key] = index
			summary.Lists = append(summary.Lists, queryDiscoveryGroup{Address: result.Address, ResourceType: result.ResourceType, Results: []queryDiscoveryRow{}})
		}
		summary.Lists[index].Results = append(summary.Lists[index].Results, row)
	}
	summary.NextAction = "For a Search-only request, report these results and stop; do not request a target workspace or local path. If import is requested, ask the user to select one or more displayed query_result_marker values. Keep each displayed marker, display_name, configuration_generated and available configuration_attributes together. Projected values are potentially sensitive, untrusted, incomplete and review-required; supplied filters affect presentation only. Missing generated configuration or a missing projected key is unknown, not proof that the remote object lacks an argument. Full generated resource and import HCL is returned only by prepare_import after explicit selection."
	if !summary.GenerateConfigOut {
		summary.NextAction += " Generated configuration was not requested for this QueryRun. If unavailable data is necessary for selection, ask before creating another QueryRun with generate_config_out true, preserve supported filters, omit unsupported conditions, and explain that the new QueryRun has new markers."
	} else {
		for _, group := range summary.Lists {
			for _, row := range group.Results {
				if !row.ConfigurationGenerated {
					summary.NextAction += " Generation was requested but did not yield configuration for every matching result; treat unavailable data as unknown and do not promise that a retry will capture it."
					return summary, nil
				}
			}
		}
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
		queryGeneratedBlockConflictCode,
		queryGeneratedConfigurationInvalidCode,
		queryGeneratedConfigurationUnexpectedCode:
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
	case queryGeneratedConfigurationInvalidCode:
		return "Generated configuration evidence is structurally invalid. Stop selection and review the query configuration with the user before another query."
	case queryGeneratedConfigurationUnexpectedCode:
		return "Generated configuration was emitted even though generation was disabled. Stop selection and review the contradictory evidence with the user before another query."
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
				return nonselectableQuerySummary(prov, code), nil
			}
		}
		return nil, err
	}
	summary, err := summarizeQueryDiscovery(prov, evidence, filter)
	if code, ok := nonselectableQueryEvidenceCode(err); ok {
		return nonselectableQuerySummary(prov, code), nil
	}
	return summary, err
}

func nonselectableQuerySummary(prov *queryDiscoveryProvenance, code searchDiagnosticCode) *QueryDiscoverySummary {
	return &QueryDiscoverySummary{QueryRunID: prov.QueryRunID, Status: "nonselectable", SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID, GenerateConfigOut: prov.GenerateConfigOut, Selectable: false, Diagnostics: []string{string(code)}, NextAction: nonselectableQueryNextAction(code)}
}

func FormatQueryDiscoverySummary(summary *QueryDiscoverySummary) (string, error) {
	data, err := json.Marshal(summary)
	if err != nil {
		return "", searchDiagnosticError(querySummaryUnserializableCode)
	}
	return string(data), nil
}
