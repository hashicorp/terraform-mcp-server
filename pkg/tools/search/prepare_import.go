// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/go-tfe"
	version "github.com/hashicorp/go-version"
)

var exactLockedVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)
var providerSourcePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+/[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+$`)
var importMarkerPattern = regexp.MustCompile(`^qrm1-[a-f0-9]{64}$`)
var importTypePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
var terraformPlanFormatVersionPattern = regexp.MustCompile(`^1\.[0-9]+$`)

var minimumImportBlockVersion = version.Must(version.NewVersion("1.5.0"))
var minimumResourceIdentityVersion = version.Must(version.NewVersion("1.12.0"))

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

type ImportContextTarget struct {
	WorkspaceID      string            `json:"workspace_id" jsonschema:"Backend-derived target workspace ID"`
	OrganizationName string            `json:"organization_name" jsonschema:"Backend-derived target organization name"`
	TerraformVersion string            `json:"terraform_version,omitempty" jsonschema:"Optional target workspace Terraform version"`
	IdentitySupport  map[string]string `json:"identity_support" jsonschema:"Identity support status by proposed managed resource type"`
}

type ImportContextSelection struct {
	QueryResultMarker     string         `json:"query_result_marker" jsonschema:"Unchanged marker for the selected QueryRun result"`
	ListType              string         `json:"list_type" jsonschema:"Search list-resource type"`
	ManagedType           string         `json:"managed_type" jsonschema:"Caller-proposed target managed resource type"`
	Identity              map[string]any `json:"identity" jsonschema:"Search identity object for the selected result"`
	SearchIdentityVersion int            `json:"search_identity_version" jsonschema:"Search identity schema version"`
}

type ImportContext struct {
	QueryRunID      string                         `json:"query_run_id" jsonschema:"QueryRun containing the selected Search results"`
	Providers       map[string]queryResultProvider `json:"providers" jsonschema:"QueryRun provider source and version by list-resource type"`
	Target          ImportContextTarget            `json:"target" jsonschema:"Backend-derived target workspace context"`
	LockedProviders map[string]string              `json:"locked_providers" jsonschema:"Agent-reported exact lock-file versions by provider source"`
	SelectedResults []ImportContextSelection       `json:"selected_results" jsonschema:"Selected QueryRun results carried into Plan verification"`
	SelectionDigest string                         `json:"selection_digest" jsonschema:"Copy-forward integrity digest for the import context"`
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
	ImportContext                     *ImportContext          `json:"import_context,omitempty"`
}

type importContextDigestPayload struct {
	QueryRunID      string                         `json:"query_run_id"`
	Providers       map[string]queryResultProvider `json:"providers"`
	Target          ImportContextTarget            `json:"target"`
	LockedProviders map[string]string              `json:"locked_providers"`
	SelectedResults []ImportContextSelection       `json:"selected_results"`
}

func importDigest(c ImportContext) (string, error) {
	data, err := json.Marshal(importContextDigestPayload{
		QueryRunID:      c.QueryRunID,
		Providers:       c.Providers,
		Target:          c.Target,
		LockedProviders: c.LockedProviders,
		SelectedResults: c.SelectedResults,
	})
	if err != nil {
		return "", queryEvidenceError("import_context_invalid")
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func FormatImportResult(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", queryEvidenceError("import_response_invalid")
	}
	return string(data), nil
}

// validNormalizedPrepareImportInput validates an input after its opaque HCP
// identifiers have been trimmed by ReadPrepareImport.
func validNormalizedPrepareImportInput(input PrepareImportInput) bool {
	if input.OrganizationName == "" || input.WorkspaceName == "" || input.QueryRunID == "" || len(input.Selections) == 0 || len(input.LockedProviders) == 0 {
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

func terraformImportSyntaxVersion(setting string) (old, known bool) {
	return terraformVersionBefore(setting, minimumImportBlockVersion)
}

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
	var buf bytes.Buffer
	if err = req.Do(ctx, &buf); err != nil {
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
	if buf.Len() == 0 || decodeQueryEvidenceLimit(buf.Bytes(), &artifact, 32<<20, 64) != nil || !terraformPlanFormatVersionPattern.MatchString(artifact.FormatVersion) {
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
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.WorkspaceName = strings.TrimSpace(input.WorkspaceName)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.QueryRunID = strings.TrimSpace(input.QueryRunID)
	if !validNormalizedPrepareImportInput(input) {
		return nil, queryEvidenceError("prepare_input_invalid")
	}
	prov, err := readQueryDiscoveryProvenance(ctx, c, input.QueryRunID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(prov.Organization, input.OrganizationName) {
		return nil, queryEvidenceError("query_organization_mismatch")
	}
	w, err := c.Workspaces.Read(ctx, input.OrganizationName, input.WorkspaceName)
	if err != nil || w == nil {
		return nil, queryEvidenceError("target_read_unavailable")
	}
	if w.ID == "" || w.Name != input.WorkspaceName || w.Organization == nil || !strings.EqualFold(w.Organization.Name, prov.Organization) {
		return nil, queryEvidenceError("target_ownership_unverified")
	}
	if prov.WorkspaceID == "" || prov.WorkspaceName == "" {
		return nil, queryEvidenceError("target_evidence_invalid")
	}
	out := &PrepareImportResult{Status: "blocked", Diagnostics: []string{}, QueryRunID: input.QueryRunID,
		SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID,
		OrganizationName: w.Organization.Name, TargetWorkspaceName: w.Name, WorkspaceID: w.ID, ExecutionMode: w.ExecutionMode,
		TargetTerraformSetting: w.TerraformVersion, VCSLinked: w.VCSRepo != nil}
	blocked := func(code, action string) (*PrepareImportResult, error) {
		out.Diagnostics = []string{code}
		out.NextAction = action
		return out, nil
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
	byMarker := make(map[string]queryResultEvidence, len(evidence.Results))
	for _, result := range evidence.Results {
		byMarker[result.Marker] = result
	}
	importContext := &ImportContext{QueryRunID: input.QueryRunID, Providers: map[string]queryResultProvider{}, Target: ImportContextTarget{WorkspaceID: w.ID, OrganizationName: w.Organization.Name, TerraformVersion: w.TerraformVersion, IdentitySupport: map[string]string{}}, LockedProviders: input.LockedProviders, SelectedResults: []ImportContextSelection{}}
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
		out.SelectedResults = append(out.SelectedResults, found)
		importContext.Providers[found.ResourceType] = found.Provider
		importContext.SelectedResults = append(importContext.SelectedResults, ImportContextSelection{QueryResultMarker: found.Marker, ListType: found.ResourceType, ManagedType: selection.ManagedType, Identity: found.Identity, SearchIdentityVersion: found.IdentityVersion})
		if !seenTypes[selection.ManagedType] {
			seenTypes[selection.ManagedType] = true
			importContext.Target.IdentitySupport[selection.ManagedType] = "unknown"
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
			if raw := resourceSchemas[kind]; len(raw) < 16<<10 && decodeQueryEvidenceLimit(raw, &schema, 16<<10, 32) == nil {
				out.Types[i].ManagedSchema = schema
			} else {
				out.Types[i].SchemaStatus = "unavailable"
				out.Types[i].ManagedTypeSupport = "unknown"
				continue
			}
			if _, found := identitySchemas[kind]; found {
				var identity map[string]any
				if raw := identitySchemas[kind]; len(raw) < 16<<10 && decodeQueryEvidenceLimit(raw, &identity, 16<<10, 32) == nil {
					out.Types[i].IdentitySupport = "supported_historically"
					out.Types[i].IdentitySchema = identity
				}
			} else if old, known := terraformVersionBefore(schemaSource.TerraformVersion, minimumResourceIdentityVersion); known && !old {
				out.Types[i].IdentitySupport = "none_historically"
			}
		} else {
			out.Types[i].ManagedTypeSupport = "absent_historically"
		}
		importContext.Target.IdentitySupport[out.Types[i].ManagedType] = out.Types[i].IdentitySupport
	}
	importContext.SelectionDigest, err = importDigest(*importContext)
	if err != nil {
		return nil, err
	}
	out.ImportContext = importContext
	out.Status = "prepared"
	out.NextAction = "Inspect the existing user-designated local tree, exact lock releases, selected observations, relevant module calls and instances, provider mappings, and existing variables and references. The proposed managed type is not proved by the Search list type, and historical schema does not attest the lock, a proposed import block, or provider scope. First author for the selected identities, effective provider scope and target locked provider's managed/identity schema. Consult that release's resource/import documentation and the target Terraform version's official import and module documentation; search_providers with the locked provider_version followed by get_provider_details can help when available. Treat generated blocks as drafts, make evidence-backed adaptations and author one reviewed resource and import block per result at distinct exact addresses. If the intended address is in an editable local module, check its calls and instances for sibling effects; do not edit remote-source modules. Review changed files and sensitive upload scope. " +
		"In the execution context that will run Terraform, review the user-approved cloud or remote-backend configuration and confirm the effective CLI host, organization, project (when relevant) and workspace with the user. Explicit cloud settings take precedence over their fallbacks; account for applicable CLI fallbacks and workspace-selection overrides without dumping environment contents; if the mapping is ambiguous, consult Terraform's cloud-connection and CLI-environment documentation and ask the user to resolve it. Confirm only that authentication for the intended host is available through the user's existing CLI setup. Do not read, print, request, copy or set credential values. MCP server configuration and authentication are separate from Terraform CLI configuration and authentication. Resolve a specific destination mismatch before starting a Run. Confirm both initiating CLI and HCP execution support import blocks (>=1.5); an init that creates a workspace/project or migrates state needs separate permission. After explicit approval for one speculative plan, an example from the confirmed directory is terraform plan -no-color -input=false (or terraform -chdir=<directory> plan -no-color -input=false), without -out or apply. These flags aid non-interactive review, not proof of a plan-only Run. The CLI, not Search MCP, uploads the configuration and creates the Run. Compare its Run URL with the intended host/org/workspace, then verify the exact Run's workspace and plan_only=true. If its Run URL or outcome is uncertain, reconcile before retrying; keep import context unchanged for verification. Approval is for one speculative plan only. This Search-to-import workflow ends after Plan review and handoff of the proposed HCL and findings; do not apply or run terraform import as its next step."
	return out, nil
}

func terraformVersionBefore(setting string, minimum *version.Version) (bool, bool) {
	parsed, err := version.NewSemver(setting)
	if err != nil {
		return false, false
	}
	// This is a feature-line comparison: prerelease Terraform builds from a
	// core release that contains the feature qualify even though SemVer orders
	// them before the final release. For example, 1.5.0-beta has import-block
	// syntax and 1.12.0-dev has resource identity support.
	return parsed.Core().LessThan(minimum.Core()), true
}
