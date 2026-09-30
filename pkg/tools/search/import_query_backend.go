// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
)

// The SDK has no typed provider-schema API or query -> no-code relationship.
// This helper only bounds a response written by go-tfe. Authentication, request
// construction, TLS, redirects and cancellation all remain in the SDK client.
type importBoundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *importBoundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, importEvidenceFailure("evidence_size_limit")
	}
	return b.buffer.Write(p)
}

func readImportBackendJSON(ctx context.Context, c *tfe.Client, endpoint string, limit int) ([]byte, int, error) {
	req, err := c.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	var status int
	ctx = tfe.ContextWithResponseHeaderHook(ctx, func(code int, _ http.Header) { status = code })
	b := &importBoundedBuffer{limit: limit}
	err = req.Do(ctx, b)
	return b.buffer.Bytes(), status, err
}

func importReadError(err error, status int) error {
	var evidence *importEvidenceError
	if errors.As(err, &evidence) {
		return evidence
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return importEvidenceFailure("evidence_read_interrupted")
	}
	if status == 401 || status == 403 {
		return importEvidenceFailure("evidence_access_denied")
	}
	return importEvidenceFailure("backend_evidence_unavailable")
}

type importRelated struct {
	Data *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"data"`
}

type importDiscoveryCandidate struct {
	CandidateID     string            `json:"candidate_id"`
	Address         string            `json:"address"`
	ResourceType    string            `json:"resource_type"`
	DisplayName     string            `json:"display_name,omitempty"`
	Identity        map[string]any    `json:"identity"`
	IdentityVersion *int              `json:"identity_version,omitempty"`
	Provider        workspaceProvider `json:"provider"`
	ResourceObject  map[string]any    `json:"resource_object,omitempty"`
	Configuration   string            `json:"configuration,omitempty"`
	ImportConfig    string            `json:"import_configuration,omitempty"`
}

// Query logs currently use config/import_config. Keep the public candidate
// names stable while also accepting the older configuration spellings.
type importDiscoveryFound struct {
	importDiscoveryCandidate
	ConfigurationWire *string `json:"configuration"`
	ImportWire        *string `json:"import_configuration"`
	Config            *string `json:"config"`
	ImportConfig      *string `json:"import_config"`
}

func (f *importDiscoveryFound) candidate() (importDiscoveryCandidate, error) {
	c := f.importDiscoveryCandidate
	if f.Config != nil && f.ConfigurationWire != nil && *f.Config != *f.ConfigurationWire {
		return c, importEvidenceFailure("query_generated_block_conflict")
	}
	if f.ImportConfig != nil && f.ImportWire != nil && *f.ImportConfig != *f.ImportWire {
		return c, importEvidenceFailure("query_generated_block_conflict")
	}
	if f.Config != nil {
		c.Configuration = *f.Config
	} else if f.ConfigurationWire != nil {
		c.Configuration = *f.ConfigurationWire
	}
	if f.ImportConfig != nil {
		c.ImportConfig = *f.ImportConfig
	} else if f.ImportWire != nil {
		c.ImportConfig = *f.ImportWire
	}
	if len(c.Configuration) > 32*1024 || len(c.ImportConfig) > 32*1024 {
		return c, importEvidenceFailure("query_generated_block_size_limit")
	}
	return c, nil
}

type importDiscovery struct {
	QueryRunID    string                     `json:"query_run_id"`
	WorkspaceID   string                     `json:"workspace_id"`
	NoCodeQueryID string                     `json:"no_code_query_id"`
	Provenance    string                     `json:"provenance"`
	Candidates    []importDiscoveryCandidate `json:"candidates"`
}

// Query inputs are read from the selected query's backend relationship, never
// from the caller's query_configuration or today's provider catalog.
func readImportDiscovery(ctx context.Context, c *tfe.Client, queryID string) (*importDiscovery, error) {
	q, err := c.QueryRuns.Read(ctx, queryID)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if q.ID != queryID || q.Status != tfe.QueryRunFinished || q.Workspace == nil || q.Workspace.ID == "" {
		return nil, importEvidenceFailure("query_evidence_unverified")
	}
	w, err := c.Workspaces.ReadByID(ctx, q.Workspace.ID)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if w.ID != q.Workspace.ID || w.Organization == nil || w.Organization.Name == "" {
		return nil, importEvidenceFailure("query_ownership_unverified")
	}
	if err := client.AuthorizeOrganization(ctx, w.Organization.Name); err != nil {
		return nil, importEvidenceFailure("organization_not_allowed")
	}
	raw, status, err := readImportBackendJSON(ctx, c, "queries/"+url.PathEscape(queryID), maxImportEvidenceBytes)
	if err != nil || status != 200 {
		return nil, importReadError(err, status)
	}
	var wire struct {
		Data struct {
			ID            string `json:"id"`
			Relationships struct {
				NoCode    importRelated `json:"no-code-query"`
				Workspace importRelated `json:"workspace"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := decodeImportEvidenceJSON(raw, &wire); err != nil {
		return nil, err
	}
	nc, ws := wire.Data.Relationships.NoCode.Data, wire.Data.Relationships.Workspace.Data
	if wire.Data.ID != queryID || nc == nil || nc.ID == "" || nc.Type != "no-code-queries" || ws == nil || ws.ID != w.ID {
		return nil, importEvidenceFailure("query_provenance_unavailable")
	}
	raw, status, err = readImportBackendJSON(ctx, c, "search/no-code-query/"+url.PathEscape(nc.ID), maxImportEvidenceBytes)
	if err != nil || status != 200 {
		return nil, importReadError(err, status)
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
				Workspace importRelated `json:"workspace"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := decodeImportEvidenceJSON(raw, &stored); err != nil {
		return nil, err
	}
	if stored.Data.ID != nc.ID || stored.Data.Relationships.Workspace.Data == nil || stored.Data.Relationships.Workspace.Data.ID != w.ID {
		return nil, importEvidenceFailure("query_provenance_unavailable")
	}
	providers := map[string]workspaceProvider{}
	for _, p := range stored.Data.Attributes.Providers {
		if !importInputName(p.Namespace) || !importInputName(p.Name) {
			return nil, importEvidenceFailure("query_provider_source_unresolved")
		}
		provider := workspaceProvider{Source: "registry.terraform.io/" + p.Namespace + "/" + p.Name, Name: p.Name, Version: p.Version}
		for _, r := range p.Resources {
			resourceType := r.Body.Type
			if resourceType == "" {
				resourceType = r.Body.HyphenType
			}
			if resourceType == "" || (r.Body.Type != "" && r.Body.HyphenType != "" && r.Body.Type != r.Body.HyphenType) {
				return nil, importEvidenceFailure("query_provider_source_unresolved")
			}
			if existing, ok := providers[resourceType]; ok && existing != provider {
				return nil, importEvidenceFailure("query_provider_ambiguous")
			}
			providers[resourceType] = provider
		}
	}
	logs, err := c.QueryRuns.Logs(ctx, queryID)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	data, err := io.ReadAll(io.LimitReader(logs, maxImportEvidenceBytes+1))
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if len(data) > maxImportEvidenceBytes {
		return nil, importEvidenceFailure("query_evidence_size_limit")
	}
	candidates, err := parseImportDiscovery(data, queryID, providers)
	if err != nil {
		return nil, err
	}
	return &importDiscovery{QueryRunID: queryID, WorkspaceID: w.ID, NoCodeQueryID: nc.ID, Provenance: "query_bound_no_code_selections", Candidates: candidates}, nil
}

func parseImportDiscovery(data []byte, queryID string, providers map[string]workspaceProvider) ([]importDiscoveryCandidate, error) {
	var candidates []importDiscoveryCandidate
	seen := map[string]bool{}
	counts, totals := map[string]int{}, map[string]int{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("{")) {
			if bytes.Contains(line, []byte("list_resource_found")) || bytes.Contains(line, []byte("diagnostic")) || bytes.HasPrefix(line, []byte("[")) {
				return nil, importEvidenceFailure("query_evidence_invalid")
			}
			continue
		}
		var record struct {
			Type       string                `json:"type"`
			Found      *importDiscoveryFound `json:"list_resource_found"`
			Complete   *queryListCompletion  `json:"list_complete"`
			Diagnostic *struct {
				Severity string `json:"severity"`
			} `json:"diagnostic"`
		}
		if err := decodeImportEvidenceJSON(line, &record); err != nil {
			return nil, err
		}
		switch record.Type {
		case "diagnostic":
			if record.Diagnostic == nil || record.Diagnostic.Severity != "warning" {
				return nil, importEvidenceFailure("query_reported_error")
			}
		case "list_resource_found":
			if record.Found == nil {
				return nil, importEvidenceFailure("query_evidence_invalid")
			}
			candidate, err := record.Found.candidate()
			if err != nil {
				return nil, err
			}
			provider, ok := providers[candidate.ResourceType]
			if !ok || candidate.Address == "" {
				return nil, importEvidenceFailure("query_provider_source_unresolved")
			}
			identity := candidate.Identity
			if len(identity) == 0 {
				return nil, importEvidenceFailure("query_identity_invalid")
			}
			encoded, _ := json.Marshal(map[string]any{"query_run_id": queryID, "provider_source": provider.Source, "provider_version": provider.Version, "list_type": candidate.ResourceType, "identity": identity})
			candidate.CandidateID = "candidate-" + strings.TrimPrefix(importEvidenceDigest(encoded), "sha256:")
			candidate.Provider = provider
			if seen[candidate.CandidateID] {
				return nil, importEvidenceFailure("query_duplicate_identity")
			}
			seen[candidate.CandidateID] = true
			counts[candidate.Address]++
			candidates = append(candidates, candidate)
			if len(candidates) > 100 {
				return nil, importEvidenceFailure("query_selection_limit")
			}
		case "list_complete":
			if record.Complete == nil || record.Complete.Total < 0 {
				return nil, importEvidenceFailure("query_evidence_invalid")
			}
			if _, exists := totals[record.Complete.Address]; exists {
				return nil, importEvidenceFailure("query_evidence_invalid")
			}
			totals[record.Complete.Address] = record.Complete.Total
		}
	}
	if len(totals) == 0 {
		return nil, importEvidenceFailure("query_evidence_incomplete")
	}
	for address, count := range counts {
		if total, ok := totals[address]; !ok || count != total {
			return nil, importEvidenceFailure("query_evidence_incomplete")
		}
	}
	for address, total := range totals {
		if counts[address] != total {
			return nil, importEvidenceFailure("query_evidence_incomplete")
		}
	}
	return candidates, nil
}

// Read metadata only. The SDK returns download URLs, but this workflow does not
// follow them or use provider resource counts to infer a provider release.
func readImportCurrentState(ctx context.Context, c *tfe.Client, workspaceID string) (*tfe.StateVersion, error) {
	var status int
	ctx = tfe.ContextWithResponseHeaderHook(ctx, func(code int, _ http.Header) { status = code })
	sv, err := c.StateVersions.ReadCurrent(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, tfe.ErrResourceNotFound) {
			return nil, importEvidenceFailure("current_state_unavailable_or_inaccessible")
		}
		return nil, importReadError(err, status)
	}
	if sv == nil || sv.ID == "" {
		return nil, importEvidenceFailure("current_state_unavailable_or_inaccessible")
	}
	return sv, nil
}

// The current state's actual run relationship selects the schema source. Its CV
// may differ from today's proposal baseline, and Workspace.CurrentRun is irrelevant.
func readImportSchemaRun(ctx context.Context, c *tfe.Client, workspaceID string, sv *tfe.StateVersion) (*tfe.Run, error) {
	if sv.Run == nil || sv.Run.ID == "" {
		return nil, importEvidenceFailure("state_producing_run_missing")
	}
	r, err := c.Runs.Read(ctx, sv.Run.ID)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if r.ID != sv.Run.ID || r.Workspace == nil || r.Workspace.ID != workspaceID {
		return nil, importEvidenceFailure("schema_source_workspace_mismatch")
	}
	if r.Plan == nil || r.Plan.ID == "" {
		return nil, importEvidenceFailure("schema_source_plan_unavailable")
	}
	return r, nil
}

func readImportManagedSchema(ctx context.Context, c *tfe.Client, runID, source, managedType string) (json.RawMessage, json.RawMessage, string, error) {
	endpoint := "runs/" + url.PathEscape(runID) + "/plan/json-schema"
	raw, status, err := readImportBackendJSON(ctx, c, endpoint, maxImportSchemaBytes)
	// An expired presigned download can return 403. Reacquire through the stable
	// endpoint once; do not cache the URL or introduce a custom redirect transport.
	// A persistent denial remains an access diagnostic, not proof of expiry.
	if status == http.StatusForbidden && ctx.Err() == nil {
		raw, status, err = readImportBackendJSON(ctx, c, endpoint, maxImportSchemaBytes)
	}
	if err != nil && (status == 0 || status == 200) {
		return nil, nil, "", importReadError(err, status)
	}
	if code := importSchemaHTTPStatus(status); code != "" {
		return nil, nil, "", importEvidenceFailure(code)
	}
	if err != nil {
		return nil, nil, "", importReadError(err, status)
	}
	var schema struct {
		FormatVersion string `json:"format_version"`
		Providers     map[string]struct {
			Resources  map[string]json.RawMessage `json:"resource_schemas"`
			Identities map[string]json.RawMessage `json:"resource_identity_schemas"`
		} `json:"provider_schemas"`
	}
	if err := decodeImportEvidenceJSONLimit(raw, &schema, maxImportSchemaBytes); err != nil {
		return nil, nil, "", err
	}
	if !importCompatibleFormat(schema.FormatVersion) {
		return nil, nil, "", importEvidenceFailure("schema_format_unsupported")
	}
	p, ok := schema.Providers[source]
	if !ok {
		return nil, nil, "", importEvidenceFailure("schema_provider_missing")
	}
	resource, ok := p.Resources[managedType]
	if !ok {
		return nil, nil, "", importEvidenceFailure("managed_schema_type_missing")
	}
	var managed struct {
		Block map[string]json.RawMessage `json:"block"`
	}
	if json.Unmarshal(resource, &managed) != nil || managed.Block == nil {
		return nil, nil, "", importEvidenceFailure("managed_schema_invalid")
	}
	return resource, p.Identities[managedType], importEvidenceDigest(raw), nil
}

func importCompatibleFormat(version string) bool {
	parts := strings.Split(version, ".")
	return len(parts) == 2 && parts[0] == "1" && parts[1] != "" && strings.IndexFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' }) < 0
}

func importDiagnosticCode(err error) string {
	var evidence *importEvidenceError
	if errors.As(err, &evidence) {
		return evidence.Code
	}
	return "backend_evidence_unavailable"
}
