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
	"sort"
	"strings"

	"github.com/hashicorp/go-tfe"
	version "github.com/hashicorp/go-version"
	tfaddr "github.com/hashicorp/terraform-registry-address"
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
	OrganizationName string            `json:"terraform_org_name,omitempty" jsonschema:"Optional explicit confirmed target organization; omit to request native user input with the proposed TF_CLOUD_ORGANIZATION default"`
	WorkspaceName    string            `json:"workspace_name,omitempty" jsonschema:"Optional confirmed target workspace override; omit to use the confirmed TF_MCP_IMPORT_WORKSPACE or TF_WORKSPACE default"`
	WorkspaceID      string            `json:"workspace_id,omitempty"`
	QueryRunID       string            `json:"query_run_id"`
	Selections       []ImportSelection `json:"selections"`
	LockedProviders  map[string]string `json:"locked_providers" jsonschema:"Exact versions keyed by provider source from .terraform.lock.hcl. Public namespace/type shorthand is accepted; custom/private sources require hostname/namespace/type. Sources are returned in canonical complete form."`
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
	LockedProviders map[string]string              `json:"locked_providers" jsonschema:"Agent-reported exact lock-file versions keyed by canonical hostname/namespace/type provider source"`
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
		return "", searchDiagnosticError(importContextInvalidCode)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

func FormatImportResult(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", searchDiagnosticError(importResponseInvalidCode)
	}
	return string(data), nil
}

// PrepareImportToolOutput performs the transport-independent prepare_import
// call and returns both representations required by the MCP SDK adapters.
func PrepareImportToolOutput(ctx context.Context, c *tfe.Client, input PrepareImportInput) (*PrepareImportResult, string, error) {
	result, err := ReadPrepareImport(ctx, c, input)
	if err != nil {
		return nil, "", fmt.Errorf("preparing the import selection: %w", err)
	}
	text, err := FormatImportResult(result)
	if err != nil {
		return nil, "", fmt.Errorf("formatting the prepare_import result: %w", err)
	}
	return result, text, nil
}

// validNormalizedPrepareImportInput validates an input after its opaque HCP
// identifiers have been trimmed by ReadPrepareImport.
func validNormalizedPrepareImportInput(input PrepareImportInput) bool {
	return ValidatePrepareImportInput(input) == nil
}

func normalizePrepareImportInput(input PrepareImportInput) PrepareImportInput {
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.WorkspaceName = strings.TrimSpace(input.WorkspaceName)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.QueryRunID = strings.TrimSpace(input.QueryRunID)
	return input
}

// ValidatePrepareImportInput performs transport-independent validation before
// either MCP adapter acquires a backend client.
func ValidatePrepareImportInput(input PrepareImportInput) error {
	input = normalizePrepareImportInput(input)
	if input.OrganizationName == "" || input.WorkspaceName == "" || input.QueryRunID == "" || len(input.Selections) == 0 || len(input.LockedProviders) == 0 {
		return searchDiagnosticError(prepareInputInvalidCode)
	}
	if _, err := canonicalizeLockedProviders(input.LockedProviders); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, s := range input.Selections {
		if !importMarkerPattern.MatchString(s.QueryResultMarker) || len(s.ManagedType) > 256 || !importTypePattern.MatchString(s.ManagedType) || seen[s.QueryResultMarker] {
			return searchDiagnosticError(prepareInputInvalidCode)
		}
		seen[s.QueryResultMarker] = true
	}
	return nil
}

func canonicalizeLockedProviders(locked map[string]string) (map[string]string, error) {
	sources := make([]string, 0, len(locked))
	for source := range locked {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	canonical := make(map[string]string, len(locked))
	for _, source := range sources {
		version := locked[source]
		if len(source) > 256 {
			return nil, invalidLockedProviderSource(source)
		}
		provider, err := tfaddr.ParseProviderSource(source)
		if err != nil {
			return nil, invalidLockedProviderSource(source)
		}
		if !provider.HasKnownNamespace() {
			detail := "locked_providers provider sources require a namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl"
			if safeProviderSourceForDiagnostic(source) {
				detail = fmt.Sprintf("locked_providers key %q lacks a provider namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl", source)
			}
			return nil, searchDiagnosticErrorWithDetail(lockedProviderSourceInvalidCode, detail)
		}
		if err := provider.Validate(); err != nil {
			return nil, invalidLockedProviderSource(source)
		}
		if len(version) > 128 || !exactLockedVersion.MatchString(version) {
			return nil, searchDiagnosticErrorWithDetail(lockVersionsInvalidCode, fmt.Sprintf("locked_providers[%q] must contain the exact selected version from .terraform.lock.hcl, not a constraint", provider.String()))
		}
		canonicalSource := provider.String()
		if previous, exists := canonical[canonicalSource]; exists && previous != version {
			return nil, searchDiagnosticErrorWithDetail(lockVersionsInvalidCode, fmt.Sprintf("multiple locked_providers entries resolve to %q but report different exact versions", canonicalSource))
		}
		canonical[canonicalSource] = version
	}
	return canonical, nil
}

func invalidLockedProviderSource(source string) error {
	detail := "locked_providers key is not a valid provider source; use public namespace/type shorthand or a complete hostname/namespace/type address from .terraform.lock.hcl"
	if safeProviderSourceForDiagnostic(source) {
		detail = fmt.Sprintf("locked_providers key %q is not a valid provider source; use public namespace/type shorthand or a complete hostname/namespace/type address from .terraform.lock.hcl", source)
	}
	return searchDiagnosticErrorWithDetail(lockedProviderSourceInvalidCode, detail)
}

func safeProviderSourceForDiagnostic(source string) bool {
	if source == "" || len(source) > 256 {
		return false
	}
	for _, r := range source {
		if r < 0x20 || r == 0x7f {
			return false
		}
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
	if buf.Len() == 0 || decodeQueryEvidence(buf.Bytes(), &artifact) != nil || !terraformPlanFormatVersionPattern.MatchString(artifact.FormatVersion) {
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
	input = normalizePrepareImportInput(input)
	if err := ValidatePrepareImportInput(input); err != nil {
		return nil, err
	}
	canonicalLockedProviders, err := canonicalizeLockedProviders(input.LockedProviders)
	if err != nil {
		return nil, err
	}
	input.LockedProviders = canonicalLockedProviders
	prov, err := readQueryDiscoveryProvenance(ctx, c, input.QueryRunID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(prov.Organization, input.OrganizationName) {
		return nil, searchDiagnosticError(queryOrganizationMismatchCode)
	}
	w, err := c.Workspaces.Read(ctx, input.OrganizationName, input.WorkspaceName)
	if err != nil || w == nil {
		return nil, searchDiagnosticError(targetReadUnavailableCode)
	}
	if w.ID == "" || w.Name != input.WorkspaceName || w.Organization == nil || !strings.EqualFold(w.Organization.Name, prov.Organization) {
		return nil, searchDiagnosticError(targetOwnershipUnverifiedCode)
	}
	if prov.WorkspaceID == "" || prov.WorkspaceName == "" {
		return nil, searchDiagnosticError(targetEvidenceInvalidCode)
	}
	out := &PrepareImportResult{Status: "blocked", Diagnostics: []string{}, QueryRunID: input.QueryRunID,
		SourceOrganizationName: prov.Organization, SourceWorkspaceName: prov.WorkspaceName, SourceWorkspaceID: prov.WorkspaceID,
		OrganizationName: w.Organization.Name, TargetWorkspaceName: w.Name, WorkspaceID: w.ID, ExecutionMode: w.ExecutionMode,
		TargetTerraformSetting: w.TerraformVersion, VCSLinked: w.VCSRepo != nil}
	blocked := func(code string) (*PrepareImportResult, error) {
		out.Diagnostics = []string{code}
		out.NextAction = prepareImportBlockedAction(code)
		return out, nil
	}
	if input.WorkspaceID != "" && input.WorkspaceID != w.ID {
		return blocked("target_mismatch")
	}
	if w.ExecutionMode != "remote" && w.ExecutionMode != "agent" {
		return blocked("target_execution_mode_not_hcp")
	}
	if old, known := terraformImportSyntaxVersion(w.TerraformVersion); known && old {
		return blocked("target_terraform_below_floor")
	}
	// The log is read only after the inexpensive source and target checks.
	evidence, err := readQueryEvidence(ctx, c, input.QueryRunID, prov)
	if err != nil {
		return nil, err
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
		if !ok || len(found.Provider.Source) > 256 || !providerSourcePattern.MatchString(found.Provider.Source) || !exactLockedVersion.MatchString(found.Provider.Version) || !strings.HasPrefix(selection.ManagedType, strings.Split(found.Provider.Source, "/")[2]+"_") {
			return nil, searchDiagnosticError(selectionInvalidCode)
		}
		if _, ok := input.LockedProviders[found.Provider.Source]; !ok {
			return nil, searchDiagnosticErrorWithDetail(lockVersionsInvalidCode, fmt.Sprintf("locked_providers must include the exact provider source %q required by the selected Search result; copy it from .terraform.lock.hcl", found.Provider.Source))
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
			if raw := resourceSchemas[kind]; decodeQueryEvidence(raw, &schema) == nil {
				out.Types[i].ManagedSchema = schema
			} else {
				out.Types[i].SchemaStatus = "unavailable"
				out.Types[i].ManagedTypeSupport = "unknown"
				continue
			}
			if _, found := identitySchemas[kind]; found {
				var identity map[string]any
				if raw := identitySchemas[kind]; decodeQueryEvidence(raw, &identity) == nil {
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
	out.NextAction = "Inspect the confirmed existing local tree and author one resource and import block per selected result at reviewed exact addresses, using the target's exact locked provider releases and effective provider scope. Treat returned drafts and historical schema as qualified evidence, then review changed files with the user. A speculative Plan requires separate approval."
	return out, nil
}

func prepareImportBlockedAction(code string) string {
	switch code {
	case "target_mismatch":
		return "Confirm the intended target workspace before preparing again; nothing was uploaded or planned."
	case "target_execution_mode_not_hcp":
		return "Choose a target with supported HCP execution mode or stop; do not change workspace mode automatically."
	case "target_terraform_below_floor":
		return "Resolve target Terraform-version compatibility; do not change the workspace version or plan automatically."
	default:
		return "Resolve the target eligibility issue before preparing again; nothing was uploaded or planned."
	}
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
