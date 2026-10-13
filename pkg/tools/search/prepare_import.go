// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
)

const maxImportSelections = 100
const maxImportResultBytes = 256 << 10

var exactLockedVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)
var providerSourcePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+/[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$`)
var importMarkerPattern = regexp.MustCompile(`^qrm1-[a-f0-9]{64}$`)
var importTypePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

type ImportSelection struct {
	QueryResultMarker string `json:"query_result_marker"`
	ManagedType       string `json:"managed_type"`
}

type PrepareImportInput struct {
	OrganizationName string            `json:"organization_name"`
	WorkspaceName    string            `json:"workspace_name"`
	WorkspaceID      string            `json:"workspace_id,omitempty"`
	QueryRunID       string            `json:"query_run_id"`
	Selections       []ImportSelection `json:"selections"`
	LockedProviders  map[string]string `json:"locked_providers"`
}

type ImportCarryTarget struct {
	WorkspaceID      string            `json:"workspace_id"`
	OrganizationName string            `json:"organization_name"`
	TerraformVersion string            `json:"terraform_version,omitempty"`
	IdentitySupport  map[string]string `json:"identity_support"`
}

type ImportCarrySelection struct {
	QueryResultMarker     string         `json:"query_result_marker"`
	ListType              string         `json:"list_type"`
	ManagedType           string         `json:"managed_type"`
	Identity              map[string]any `json:"identity"`
	SearchIdentityVersion int            `json:"search_identity_version"`
}

type ImportCarry struct {
	QueryRunID      string                         `json:"query_run_id"`
	Providers       map[string]queryResultProvider `json:"providers"`
	Target          ImportCarryTarget              `json:"target"`
	LockedProviders map[string]string              `json:"locked_providers"`
	SelectedResults []ImportCarrySelection         `json:"selected_results"`
	SelectionDigest string                         `json:"selection_digest"`
}

type PreparedImportType struct {
	ProviderSource        string         `json:"provider_source"`
	SearchProviderVersion string         `json:"search_provider_version"`
	LockedProviderVersion string         `json:"locked_provider_version"`
	ManagedType           string         `json:"managed_type"`
	ManagedTypeSupport    string         `json:"managed_type_support"`
	IdentitySupport       string         `json:"identity_support"`
	SchemaStatus          string         `json:"schema_status"`
	IdentityCompatibility string         `json:"identity_compatibility"`
	ManagedSchema         map[string]any `json:"managed_schema,omitempty"`
	IdentitySchema        map[string]any `json:"identity_schema,omitempty"`
}

type HistoricalSchemaSource struct {
	StateVersionID   string `json:"state_version_id"`
	RunID            string `json:"run_id"`
	PlanID           string `json:"plan_id"`
	TerraformVersion string `json:"terraform_version,omitempty"`
	EvidenceRole     string `json:"evidence_role"`
}

type PrepareImportResult struct {
	Status                            string                  `json:"status"`
	Diagnostics                       []string                `json:"diagnostics"`
	NextAction                        string                  `json:"next_action"`
	QueryRunID                        string                  `json:"query_run_id,omitempty"`
	SourceOrganizationName            string                  `json:"source_organization_name,omitempty"`
	SourceWorkspaceName               string                  `json:"source_workspace_name,omitempty"`
	SourceWorkspaceID                 string                  `json:"source_workspace_id,omitempty"`
	OrganizationName                  string                  `json:"organization_name,omitempty"`
	TargetWorkspaceName               string                  `json:"target_workspace_name,omitempty"`
	WorkspaceID                       string                  `json:"workspace_id,omitempty"`
	ExecutionMode                     string                  `json:"execution_mode,omitempty"`
	TargetTerraformSetting            string                  `json:"target_terraform_setting,omitempty"`
	StateProducingRunTerraformVersion string                  `json:"state_producing_run_terraform_version,omitempty"`
	SchemaSource                      *HistoricalSchemaSource `json:"schema_source,omitempty"`
	VCSLinked                         bool                    `json:"vcs_linked"`
	Types                             []PreparedImportType    `json:"types,omitempty"`
	SelectedResults                   []queryResultEvidence   `json:"selected_results,omitempty"`
	Carry                             *ImportCarry            `json:"carry,omitempty"`
}

func importDigest(c ImportCarry) (string, error) {
	data, err := json.Marshal(struct {
		QueryRunID      string                         `json:"query_run_id"`
		Providers       map[string]queryResultProvider `json:"providers"`
		Target          ImportCarryTarget              `json:"target"`
		LockedProviders map[string]string              `json:"locked_providers"`
		SelectedResults []ImportCarrySelection         `json:"selected_results"`
	}{c.QueryRunID, c.Providers, c.Target, c.LockedProviders, c.SelectedResults})
	if err != nil {
		return "", queryEvidenceError("carry_invalid")
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func checkImportResultBudget(v any) error {
	data, err := json.Marshal(v)
	if err != nil || len(data) > maxImportResultBytes/2 {
		// Results are returned in structured and JSON-text form. Cap their
		// combined content before either transport serializes its envelope.
		return queryEvidenceError("import_response_limit")
	}
	return nil
}

func FormatImportResult(v any) (string, error) {
	if err := checkImportResultBudget(v); err != nil {
		return "", err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", queryEvidenceError("import_response_invalid")
	}
	return string(data), nil
}

func validImportInput(input PrepareImportInput) bool {
	if !validImportWorkspaceName(input.OrganizationName) || !validImportWorkspaceName(input.WorkspaceName) || input.WorkspaceID != "" && !validImportWorkspaceName(input.WorkspaceID) || !validQueryRunID(input.QueryRunID) || len(input.Selections) == 0 || len(input.Selections) > maxImportSelections || len(input.LockedProviders) == 0 {
		return false
	}
	for source, version := range input.LockedProviders {
		if len(source) > 256 || len(version) > 128 || !providerSourcePattern.MatchString(source) || !exactLockedVersion.MatchString(version) {
			return false
		}
	}
	seen := map[string]bool{}
	for _, s := range input.Selections {
		if !importMarkerPattern.MatchString(s.QueryResultMarker) || len(s.ManagedType) > 256 || !importTypePattern.MatchString(s.ManagedType) || seen[s.QueryResultMarker] {
			return false
		}
		seen[s.QueryResultMarker] = true
	}
	return true
}

var importWorkspaceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,256}$`)

func validImportWorkspaceName(name string) bool { return importWorkspaceNamePattern.MatchString(name) }

func allowedImportOrganization(org string) bool {
	configured := os.Getenv(client.OrganizationAllowlistEnv)
	if configured == "" {
		return true
	}
	allowed, err := client.ParseOrganizationAllowlistCSV(configured)
	if err != nil {
		return false
	}
	for _, name := range allowed {
		if strings.EqualFold(name, org) {
			return true
		}
	}
	return false
}

func terraformImportSyntaxVersion(setting string) (old, known bool) {
	// HCP plans may report a prerelease Terraform build. Do not mistake a
	// well-formed suffix for an unknown version, but never accept arbitrary
	// trailing text as proof of import-block support.
	parts := terraformImportVersionPattern.FindStringSubmatch(setting)
	if parts == nil {
		return false, false
	}
	major, e1 := strconv.Atoi(parts[1])
	minor, e2 := strconv.Atoi(parts[2])
	patch, e3 := strconv.Atoi(parts[3])
	if e1 != nil || e2 != nil || e3 != nil || major < 0 || minor < 0 || patch < 0 {
		return false, false
	}
	return major < 1 || major == 1 && minor < 5, true
}

var terraformImportVersionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?(?:\+[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

// Historical state/plan data is optional context: inaccessible state does not
// prove an empty target. Never use the most recent Run as a substitute.
func readHistoricalSchema(ctx context.Context, c *tfe.Client, workspaceID string) (*HistoricalSchemaSource, map[string]json.RawMessage, map[string]json.RawMessage) {
	state, err := c.StateVersions.ReadCurrent(ctx, workspaceID)
	if err != nil || state == nil || state.ID == "" || state.Run == nil || state.Run.ID == "" {
		return nil, nil, nil
	}
	run, err := c.Runs.Read(ctx, state.Run.ID)
	if err != nil || run == nil || run.ID != state.Run.ID || run.Workspace == nil || run.Workspace.ID != workspaceID {
		return nil, nil, nil
	}
	source := &HistoricalSchemaSource{StateVersionID: state.ID, RunID: run.ID, TerraformVersion: run.TerraformVersion, EvidenceRole: "historical_target_plan"}
	if run.Plan == nil || run.Plan.ID == "" {
		return source, nil, nil
	}
	plan, err := c.Plans.Read(ctx, run.Plan.ID)
	if err != nil || plan == nil || plan.ID != run.Plan.ID {
		return source, nil, nil
	}
	endpoint := "runs/" + url.PathEscape(run.ID) + "/plan/json-schema"
	req, err := c.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return source, nil, nil
	}
	buf := &queryBoundedWriter{limit: 32 << 20}
	if err = req.Do(ctx, buf); err != nil {
		return source, nil, nil
	}
	var artifact struct {
		FormatVersion string `json:"format_version"`
		Providers     map[string]struct {
			Resources  map[string]json.RawMessage `json:"resource_schemas"`
			Identities map[string]json.RawMessage `json:"resource_identity_schemas"`
		} `json:"provider_schemas"`
	}
	// The historical schema is not required for preparation. Reject malformed
	// or unsupported artifacts without treating them as evidence of absence.
	if len(buf.buffer.Bytes()) == 0 || decodeQueryEvidenceLimit(buf.buffer.Bytes(), &artifact, 32<<20, 64) != nil || !regexp.MustCompile(`^1\.[0-9]+$`).MatchString(artifact.FormatVersion) {
		return source, nil, nil
	}
	resources, identities := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	for provider, schemas := range artifact.Providers {
		// A missing provider entry is unknown, never evidence that a
		// managed type is absent from the user's locked provider release.
		resources[provider+"/"] = json.RawMessage(`{}`)
		for kind, raw := range schemas.Resources {
			resources[provider+"/"+kind] = raw
		}
		for kind, raw := range schemas.Identities {
			identities[provider+"/"+kind] = raw
		}
	}
	source.PlanID = plan.ID
	return source, resources, identities
}

func ReadPrepareImport(ctx context.Context, c *tfe.Client, input PrepareImportInput) (*PrepareImportResult, error) {
	if !validImportInput(input) {
		return nil, queryEvidenceError("prepare_input_invalid")
	}
	prov, err := readQueryDiscoveryProvenance(ctx, c, input.QueryRunID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(prov.Organization, input.OrganizationName) || !allowedImportOrganization(prov.Organization) {
		return nil, queryEvidenceError("query_organization_mismatch")
	}
	w, err := c.Workspaces.Read(ctx, input.OrganizationName, input.WorkspaceName)
	if err != nil || w == nil {
		return nil, queryEvidenceError("target_read_unavailable")
	}
	if w.ID == "" || w.Name != input.WorkspaceName || w.Organization == nil || !strings.EqualFold(w.Organization.Name, prov.Organization) || !allowedImportOrganization(w.Organization.Name) {
		return nil, queryEvidenceError("target_ownership_unverified")
	}
	if !validImportWorkspaceName(w.ID) || !validImportWorkspaceName(prov.WorkspaceID) || !validImportWorkspaceName(prov.WorkspaceName) || unsafeDiscoveryValue(w.TerraformVersion) {
		return nil, queryEvidenceError("target_evidence_invalid")
	}
	out := &PrepareImportResult{Status: "blocked", Diagnostics: []string{}, QueryRunID: input.QueryRunID,
		SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID,
		OrganizationName: w.Organization.Name, TargetWorkspaceName: w.Name, WorkspaceID: w.ID, ExecutionMode: w.ExecutionMode,
		TargetTerraformSetting: w.TerraformVersion, VCSLinked: w.VCSRepo != nil}
	blocked := func(code, action string) (*PrepareImportResult, error) {
		out.Diagnostics = []string{code}
		out.NextAction = action
		return out, checkImportResultBudget(out)
	}
	if input.WorkspaceID != "" && input.WorkspaceID != w.ID {
		return blocked("target_mismatch", "Confirm the intended target workspace before preparing again; nothing was uploaded or planned.")
	}
	if w.ExecutionMode != "remote" && w.ExecutionMode != "agent" {
		return blocked("target_execution_mode_not_hcp", "This target cannot provide the intended HCP-executed speculative Plan; do not plan with this workflow.")
	}
	if old, known := terraformImportSyntaxVersion(w.TerraformVersion); known && old {
		return blocked("target_terraform_below_floor", "The target's current Terraform setting is below 1.5; do not silently change it or run a plan.")
	}
	// The log is read only after the inexpensive source and target checks.
	checkedSource, evidence, err := readQueryDiscovery(ctx, c, input.QueryRunID)
	if err != nil {
		return nil, err
	}
	if checkedSource == nil || checkedSource.WorkspaceID != prov.WorkspaceID || !strings.EqualFold(checkedSource.Organization, prov.Organization) {
		return nil, queryEvidenceError("query_provenance_changed")
	}
	// Reuse the summary's whole-QueryRun selectability guards. A later
	// oversized or sensitive row cannot be skipped just because the caller
	// happened to select a different marker from the first page.
	if _, err := pageQueryDiscovery(checkedSource, evidence, QueryDiscoveryFilter{Limit: 1}); err != nil {
		return nil, err
	}
	byMarker := make(map[string]queryResultEvidence, len(evidence.Results))
	for _, result := range evidence.Results {
		byMarker[result.Marker] = result
	}
	carry := &ImportCarry{QueryRunID: input.QueryRunID, Providers: map[string]queryResultProvider{}, Target: ImportCarryTarget{WorkspaceID: w.ID, OrganizationName: w.Organization.Name, TerraformVersion: w.TerraformVersion, IdentitySupport: map[string]string{}}, LockedProviders: input.LockedProviders, SelectedResults: []ImportCarrySelection{}}
	seenTypes := map[string]bool{}
	for _, selection := range input.Selections {
		found, ok := byMarker[selection.QueryResultMarker]
		// The list type is not necessarily a plural of the managed type.
		// Keep the caller-proposed managed type and Search list type separate;
		// the optional target schema and finished Plan supply target-side checks.
		if !ok || !providerSourcePattern.MatchString(found.Provider.Source) || !exactLockedVersion.MatchString(found.Provider.Version) || !strings.HasPrefix(selection.ManagedType, strings.Split(found.Provider.Source, "/")[2]+"_") {
			return nil, queryEvidenceError("selection_invalid")
		}
		if _, ok := input.LockedProviders[found.Provider.Source]; !ok {
			return nil, queryEvidenceError("lock_versions_invalid")
		}
		if unsafeDiscoveryValue(found.Address) || unsafeDiscoveryValue(found.DisplayName) || unsafeDiscoveryValue(found.Identity) || unsafeDiscoveryValue(found.ResourceObject) || unsafeDiscoveryValue(found.Configuration) || unsafeDiscoveryValue(found.ImportConfig) {
			return nil, queryEvidenceError("query_sensitive_result")
		}
		out.SelectedResults = append(out.SelectedResults, found)
		carry.Providers[found.ResourceType] = found.Provider
		carry.SelectedResults = append(carry.SelectedResults, ImportCarrySelection{QueryResultMarker: found.Marker, ListType: found.ResourceType, ManagedType: selection.ManagedType, Identity: found.Identity, SearchIdentityVersion: found.IdentityVersion})
		if !seenTypes[selection.ManagedType] {
			seenTypes[selection.ManagedType] = true
			carry.Target.IdentitySupport[selection.ManagedType] = "unknown"
			out.Types = append(out.Types, PreparedImportType{ProviderSource: found.Provider.Source, SearchProviderVersion: found.Provider.Version, LockedProviderVersion: input.LockedProviders[found.Provider.Source], ManagedType: selection.ManagedType, ManagedTypeSupport: "unknown", IdentitySupport: "unknown", SchemaStatus: "unknown", IdentityCompatibility: "unverified"})
		}
	}
	// Optional historical evidence is deliberately read after selection is
	// established. It never substitutes for the local locked release.
	schemaSource, resourceSchemas, identitySchemas := readHistoricalSchema(ctx, c, w.ID)
	if schemaSource != nil {
		out.StateProducingRunTerraformVersion = schemaSource.TerraformVersion
		if schemaSource.PlanID != "" {
			out.SchemaSource = schemaSource
		}
	}
	for i := range out.Types {
		kind := out.Types[i].ProviderSource + "/" + out.Types[i].ManagedType
		if _, providerRead := resourceSchemas[out.Types[i].ProviderSource+"/"]; !providerRead {
			continue
		}
		out.Types[i].SchemaStatus = "available"
		if _, exists := resourceSchemas[kind]; exists {
			out.Types[i].ManagedTypeSupport = "supported_historically"
			var schema map[string]any
			if raw := resourceSchemas[kind]; len(raw) < 16<<10 && decodeQueryEvidenceLimit(raw, &schema, 16<<10, 32) == nil && !unsafeDiscoveryValue(schema) {
				out.Types[i].ManagedSchema = schema
			} else {
				out.Types[i].SchemaStatus = "unavailable"
				out.Types[i].ManagedTypeSupport = "unknown"
				continue
			}
			if _, found := identitySchemas[kind]; found {
				var identity map[string]any
				if raw := identitySchemas[kind]; len(raw) < 16<<10 && decodeQueryEvidenceLimit(raw, &identity, 16<<10, 32) == nil && !unsafeDiscoveryValue(identity) {
					out.Types[i].IdentitySupport = "supported_historically"
					out.Types[i].IdentitySchema = identity
				}
			} else if old, known := terraformVersionBefore(schemaSource.TerraformVersion, 1, 12); known && !old {
				out.Types[i].IdentitySupport = "none_historically"
			}
		} else {
			out.Types[i].ManagedTypeSupport = "absent_historically"
		}
		carry.Target.IdentitySupport[out.Types[i].ManagedType] = out.Types[i].IdentitySupport
	}
	carry.SelectionDigest, err = importDigest(*carry)
	if err != nil {
		return nil, err
	}
	out.Carry = carry
	out.Status = "prepared"
	out.NextAction = "Inspect the existing user-designated local tree, exact lock releases, selected observations and versioned resource/import docs; the proposed managed type is not proved by the Search list type, and historical schema does not attest the lock, a proposed import block, or provider scope. Author one reviewed resource and import block per result at distinct addresses. Review changed files and sensitive upload scope. In the execution context that will run Terraform, review the user-approved cloud or remote-backend configuration and confirm the effective CLI host, organization, project (when relevant) and workspace with the user. Explicit cloud settings take precedence over their fallbacks; account for applicable CLI fallbacks and workspace-selection overrides without dumping environment contents; if the mapping is ambiguous, consult Terraform's cloud-connection and CLI-environment documentation and ask the user to resolve it. Confirm only that authentication for the intended host is available through the user's existing CLI setup. Do not read, print, request, copy or set credential values. MCP server configuration and authentication are separate from Terraform CLI configuration and authentication. Resolve a specific destination mismatch before starting a Run. Confirm both initiating CLI and HCP execution support import blocks (>=1.5); an init that creates a workspace/project or migrates state needs separate permission. After explicit approval for one speculative plan, an example from the confirmed directory is terraform plan -no-color -input=false (or terraform -chdir=<directory> plan -no-color -input=false), without -out or apply. These flags aid non-interactive review, not proof of a plan-only Run. The CLI, not Search MCP, uploads the configuration and creates the Run. Compare its Run URL with the intended host/org/workspace, then verify the exact Run's workspace and plan_only=true. If its Run URL or outcome is uncertain, reconcile before retrying; keep carry unchanged for verification."
	if err := checkImportResultBudget(out); err != nil {
		return nil, err
	}
	return out, nil
}

func terraformVersionBefore(setting string, major, minor int) (bool, bool) {
	parts := strings.Split(strings.TrimPrefix(setting, "v"), ".")
	if len(parts) != 3 {
		return false, false
	}
	ma, e1 := strconv.Atoi(parts[0])
	mi, e2 := strconv.Atoi(parts[1])
	_, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return false, false
	}
	return ma < major || ma == major && mi < minor, true
}
