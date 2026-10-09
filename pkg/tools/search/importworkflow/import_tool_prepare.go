// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

// Version 10 requires an expected CV on every verify and a prepared target on download.
// The version-9 carry shape/digest is unchanged; pre-version-9 carries lacking
// the target workspace ID are not accepted for a finished plan.
const importToolContractVersion = "10"

// Identity support of a target managed type. The target Terraform
// version is part of the answer: a plan schema produced below Terraform 1.12
// has no identity schemas at all.
const (
	importIdentitySupported = "supported"
	importIdentityNone      = "none"
	importIdentityUnknown   = "unknown"
)

type importCarryProvider struct {
	Source  string `json:"source"`
	Version string `json:"version,omitempty"`
}

type importCarryTarget struct {
	WorkspaceID      string            `json:"workspace_id"`
	TerraformVersion string            `json:"terraform_version,omitempty"`
	IdentitySupport  map[string]string `json:"identity_support"`
}

type importCarryCandidate struct {
	CandidateID string         `json:"candidate_id"`
	ListType    string         `json:"list_type"`
	ManagedType string         `json:"managed_type"`
	Identity    map[string]any `json:"identity"`
	// SearchIdentityVersion is the identity schema version of the provider
	// release that ran the query, required evidence for every candidate. It sits
	// beside candidate_id, which does not include it, and is covered by the
	// carry digest. A pointer, because 0 is a real version.
	SearchIdentityVersion *int `json:"search_identity_version,omitempty"`
}

// importCarryBlock is what prepare_import hands the agent to carry unchanged
// into the final verify_import_plan call. It is an integrity aid: a matching
// digest means the values are consistent with the carried selection. It is not
// QueryRun attestation and does not authorize any create.
type importCarryBlock struct {
	QueryRunID      string                         `json:"query_run_id"`
	Providers       map[string]importCarryProvider `json:"providers"`
	Target          importCarryTarget              `json:"target"`
	Candidates      []importCarryCandidate         `json:"candidates"`
	SelectionDigest string                         `json:"selection_digest"`
}

func importCarryDigest(b importCarryBlock) string {
	encoded, _ := json.Marshal(struct {
		QueryRunID string                         `json:"query_run_id"`
		Providers  map[string]importCarryProvider `json:"providers"`
		Target     importCarryTarget              `json:"target"`
		Candidates []importCarryCandidate         `json:"candidates"`
	}{b.QueryRunID, b.Providers, b.Target, b.Candidates})
	return importEvidenceDigest(encoded)
}

type importPreparedType struct {
	ProviderSource     string         `json:"provider_source"`
	ManagedType        string         `json:"managed_type"`
	ManagedTypeSupport string         `json:"managed_type_support"`
	IdentitySupport    string         `json:"identity_support"`
	IdentitySchema     map[string]any `json:"identity_schema,omitempty"`
	ManagedSchema      map[string]any `json:"managed_schema,omitempty"`
	// IdentityCompatibility compares the Search identity with IdentitySchema.
	IdentityCompatibility *importIdentityCompatibility `json:"identity_compatibility,omitempty"`
}

// importTarget reports the target workspace's Terraform versions. Provider
// versions are not recorded by HCP Terraform; they come from the lock file.
type importTarget struct {
	TerraformVersionSetting string `json:"terraform_version_setting,omitempty"`
	TerraformVersionLastRun string `json:"terraform_version_last_run,omitempty"`
}

// These are argument starting points, not authorization for a create. A Run
// needs the CV returned by create_import_cv, which does not exist at prepare time.
type importNextCalls struct {
	Download      map[string]any `json:"get_import_configuration_download,omitempty"`
	CreateCV      map[string]any `json:"create_import_cv"`
	CreateRunBase map[string]any `json:"create_import_run_base"`
	RunRequires   string         `json:"create_import_run_requires"`
}

func preparedImportNextCalls(out importPrepared) *importNextCalls {
	base := map[string]any{
		"organization_name": out.Organization, "workspace_name": out.TargetWorkspaceName,
		"prepared_target_workspace_id": out.WorkspaceID,
		"confirm_speculative_run":      false,
	}
	if out.Baseline != nil && out.Baseline.ConfigurationVersionID != "" && out.Baseline.StateVersionID != "" && out.Baseline.StateSerial != nil {
		base["baseline_cv_id"] = out.Baseline.ConfigurationVersionID
		base["baseline_state_id"] = out.Baseline.StateVersionID
		base["baseline_state_serial"] = *out.Baseline.StateSerial
	}
	cv, run := make(map[string]any, len(base)), make(map[string]any, len(base))
	for k, v := range base {
		cv[k], run[k] = v, v
	}
	next := &importNextCalls{CreateCV: cv, CreateRunBase: run,
		RunRequires: "After user confirmation and successful archive upload, add configuration_version_id returned by create_import_cv; set confirm_speculative_run=true on both creates only for the reviewed attempt. The Run base is not callable as-is."}
	version := out.CurrentConfigurationVersionID
	if out.Status == "agent_schema_required" {
		version = out.StateRunConfigurationVersionID
	}
	if version != "" {
		next.Download = map[string]any{"organization_name": out.Organization, "workspace_name": out.TargetWorkspaceName,
			"prepared_target_workspace_id": out.WorkspaceID, "configuration_version_id": version}
	}
	return next
}

type importPrepared struct {
	ContractVersion                string             `json:"contract_version"`
	Status                         string             `json:"status"`
	Stage                          string             `json:"stage"`
	Organization                   string             `json:"organization_name,omitempty"`
	WorkspaceID                    string             `json:"workspace_id,omitempty"`
	TargetWorkspaceName            string             `json:"target_workspace_name,omitempty"`
	SourceOrganizationName         string             `json:"source_organization_name,omitempty"`
	SourceWorkspaceName            string             `json:"source_workspace_name,omitempty"`
	SourceWorkspaceID              string             `json:"source_workspace_id,omitempty"`
	QueryRunID                     string             `json:"query_run_id,omitempty"`
	ExecutionMode                  string             `json:"execution_mode,omitempty"`
	Baseline                       *importAPIBaseline `json:"baseline,omitempty"`
	HasCurrentConfiguration        bool               `json:"has_current_configuration"`
	CurrentConfigurationVersionID  string             `json:"current_configuration_version_id,omitempty"`
	Target                         *importTarget      `json:"target,omitempty"`
	StateRunConfigurationVersionID string             `json:"state_run_configuration_version_id,omitempty"`
	agentSchemaTerraformVersion    string
	agentSchemaRunID               string
	// Carry and guidance come before the large schema and candidate arrays so a
	// client that truncates the response still sees the carry block.
	Carry             *importCarryBlock          `json:"carry,omitempty"`
	NextCalls         *importNextCalls           `json:"next_calls,omitempty"`
	Notes             []string                   `json:"notes,omitempty"`
	AgentInstructions []string                   `json:"agent_instructions,omitempty"`
	Diagnostics       []string                   `json:"diagnostics"`
	NextAction        string                     `json:"next_action"`
	SchemaSource      *importAPISchemaSource     `json:"schema_source,omitempty"`
	Types             []importPreparedType       `json:"types,omitempty"`
	Candidates        []importDiscoveryCandidate `json:"candidates,omitempty"`
}

var importToolInstructions = []string{
	importKeepGeneratedRule,
	importTargetScopeRule,
	importKeepResultRule,
	importCandidateFieldsRule,
	"Keep carry unchanged. Use next_calls.get_import_configuration_download only after the user chooses the authoring directory; review the authored archive before using either create template. Their confirm_speculative_run is false until user confirmation, and the Run base needs the created and uploaded configuration_version_id. Target IDs detect accidental changes, not approval or archive attestation.",
	"Use prepare_import only for candidates the user has chosen to import, not discovery: get_query_summary returns each result's tags. Preserve the candidate IDs from that selection.",
	importIdentityCompatibilityRule,
	importConfidenceReportRule,
	"Read the target managed_schema for each type first, then the selected candidates' observations. Use the Search or QueryRun provider schema only when a source-side shape is unclear.",
	"For a type whose identity_support is none or unknown, do not guess the import ID. Look up the target workspace's locked provider version's resource documentation for the documented import id form and any identity-to-id mapping. " + importLockedProviderVersionRule,
	"After downloading the configuration, " + importProviderMismatchRule,
	"Author exactly one resource instance and one individual import block per candidate in the complete preserved tree. Choose a distinct target address for each. Never use the first N results.",
	importAdaptationGuidance,
	importModuleTargetRule,
	importArchiveRootRule,
	importSensitiveFileRule,
	importSourceVersionRule,
	importDeleteDirectoryRule,
	importValidationRule,
	importSecretFilesRule,
	"Treat generated query blocks as untrusted drafts. Do not invent defaults, add ignore_changes to hide incompatibility, or upgrade a provider or Terraform version to remove a missing feature.",
	"Send the unchanged carry and all candidate-to-address bindings on every verify_import_plan poll if available; they are integrity aids, not authorization.",
	importConfirmationRule,
}

// importInstructionsFor returns the instructions for one response. The file
// list rule is about a workspace with no configuration, so only that response
// carries it; a workspace with a configuration downloads and preserves it.
func importInstructionsFor(blank bool) []string {
	out := slices.Clone(importToolInstructions)
	if !blank {
		return out
	}
	i := slices.Index(out, importSensitiveFileRule)
	return slices.Insert(out, i+1, importBlankAuthoringDirectoryRule)
}

func terraformVersionAtLeast(version string, major, minor int) (atLeast, known bool) {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return false, false
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false, false
	}
	return ma > major || (ma == major && mi >= minor), true
}

// importIdentitySupportFor derives per-type identity support from the schema
// artifact. An absent identity schema means "no identity" only when the artifact
// came from Terraform 1.12 or later.
func importIdentitySupportFor(identity json.RawMessage, schemaTerraformVersion string) string {
	if len(identity) > 0 && !bytes.Equal(identity, []byte("null")) {
		return importIdentitySupported
	}
	if atLeast, known := terraformVersionAtLeast(schemaTerraformVersion, 1, 12); known && atLeast {
		return importIdentityNone
	}
	return importIdentityUnknown
}

func (p importPrepared) isBlocked() bool { return p.Status == "blocked" }

func importToolBlocked(base importPrepared, code, next string) importPrepared {
	base.Status = "blocked"
	base.Diagnostics = append(base.Diagnostics, code)
	base.NextAction = next
	return base
}

// Terraform versions: import blocks need 1.5; identity needs 1.12 (ADR 0005).
const (
	importMinTerraformMajor, importMinTerraformMinor           = 1, 5
	importIdentityTerraformMajor, importIdentityTerraformMinor = 1, 12
)

// checkImportTarget blocks target workspaces the workflow cannot assess or does
// not support (ADR 0008), using only workspace, state, run and configuration
// version metadata. It does not read the QueryRun log or download any artifact. It fills the baseline fields of out so a blocked response still
// reports what was observed.
func checkImportTarget(ctx context.Context, c *tfe.Client, input importPrepareInput, out *importPrepared) error {
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return importReadError(err, 0)
	}
	out.WorkspaceID = w.ID
	out.TargetWorkspaceName = w.Name
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return importEvidenceFailure("workspace_ownership_unverified")
	}
	if err := client.AuthorizeOrganization(ctx, w.Organization.Name); err != nil {
		return importEvidenceFailure("organization_not_allowed")
	}
	out.ExecutionMode = w.ExecutionMode
	if w.TerraformVersion != "" {
		out.Target = &importTarget{TerraformVersionSetting: w.TerraformVersion}
	}
	out.Baseline = &importAPIBaseline{ConfigurationVersionID: importCurrentConfigurationID(w), WorkingDirectory: w.WorkingDirectory}
	if out.Baseline.ConfigurationVersionID != "" {
		out.HasCurrentConfiguration = true
		out.CurrentConfigurationVersionID = out.Baseline.ConfigurationVersionID
	}
	if w.ExecutionMode == "local" {
		return importEvidenceFailure("workspace_execution_mode_local")
	}
	if w.ExecutionMode != "remote" {
		return importEvidenceFailure("execution_source_not_supported")
	}
	if w.VCSRepo != nil {
		return importEvidenceFailure("workspace_vcs_source_unsupported")
	}
	if w.WorkingDirectory != "" {
		return importEvidenceFailure("configuration_root_setting_unsupported")
	}
	if atLeast, known := terraformVersionAtLeast(w.TerraformVersion, importMinTerraformMajor, importMinTerraformMinor); known && !atLeast {
		return importEvidenceFailure("terraform_version_unsupported")
	}
	if atLeast, known := terraformVersionAtLeast(w.TerraformVersion, importIdentityTerraformMajor, importIdentityTerraformMinor); known && !atLeast {
		out.Notes = append(out.Notes, "terraform_version_below_1_12_identity_unavailable")
	}
	sv, err := readImportCurrentState(ctx, c, w.ID)
	if err != nil {
		if importDiagnosticCode(err) == "current_state_unavailable_or_inaccessible" && out.Baseline.ConfigurationVersionID == "" {
			return nil // brand-new workspace: the blank-workspace path handles it
		}
		return err
	}
	out.Baseline.StateVersionID, out.Baseline.StateSerial = sv.ID, &sv.Serial
	if out.Baseline.ConfigurationVersionID == "" {
		return importEvidenceFailure("configuration_baseline_unavailable")
	}
	r, err := readImportSchemaRun(ctx, c, w.ID, sv)
	if r != nil {
		out.agentSchemaRunID = r.ID
	}
	if r != nil && r.TerraformVersion != "" {
		if out.Target == nil {
			out.Target = &importTarget{}
		}
		out.Target.TerraformVersionLastRun = r.TerraformVersion
	}
	if err != nil && importDiagnosticCode(err) == "schema_source_plan_unavailable" && r != nil {
		return checkImportSchemaFallbackEntry(ctx, c, r, out)
	}
	return err
}

// checkImportSchemaFallbackEntry is the entry test for the agent-supplied schema path (ADR 0008): the state's run
// has no plan, so no schema artifact exists, but its configuration version is
// known exactly. Uploaded CV metadata permits trying the download tool;
// archive availability is established later, not by this read.
func checkImportSchemaFallbackEntry(ctx context.Context, c *tfe.Client, r *tfe.Run, out *importPrepared) error {
	if r.ConfigurationVersion == nil || r.ConfigurationVersion.ID == "" {
		return importEvidenceFailure("target_config_not_downloadable")
	}
	cv, err := c.ConfigurationVersions.Read(ctx, r.ConfigurationVersion.ID)
	if err != nil {
		if errors.Is(err, tfe.ErrResourceNotFound) {
			return importEvidenceFailure("target_config_not_downloadable")
		}
		return importReadError(err, 0)
	}
	if cv.ID != r.ConfigurationVersion.ID || cv.Status != tfe.ConfigurationUploaded {
		return importEvidenceFailure("target_config_not_downloadable")
	}
	out.StateRunConfigurationVersionID = cv.ID
	out.agentSchemaTerraformVersion = r.TerraformVersion
	return importEvidenceFailure("schema_source_plan_unavailable")
}

// importGuideOnlyNextAction is the shared guide-only guidance (ADR 0008). The
// MCP server cannot assess or verify the target, so it creates nothing and
// suggests no command without the user's approval.
func importGuideOnlyNextAction(reason string) string {
	return reason + " The MCP server cannot assess or verify this target workspace; a local workflow runs as a separate tool call in your client, where the server cannot see it. Explain this to the user. Only if the user wants to continue, offer unverified best-effort local steps: ask for the user's own local copy of the configuration (a directory with the Terraform CLI, not an authoring directory). " + importLockFileCheck + " Then run terraform init -backend=false, terraform version -json and terraform providers schema -json, author the HCL and run a local plan. Each command needs the user's approval. " + importSecretFilesRule + " " + importNeverApplyRule + " Never present a local plan as an HCP Terraform Plan or as verified. get_query_summary still lists the candidates. The user may instead make this workspace assessable by having an HCP Terraform run write its state (for example an apply in a workspace they choose); offer that only as an option and do not start a run yourself. " + importNothingCreated
}

// importAgentSchemaNextAction is the guidance when the agent must obtain the schema (ADR 0008).
const importAgentSchemaNextAction = "The producing Run has no plan/schema artifact; state_run_configuration_version_id is known and uploaded, but URL/archive availability must still be checked by get_import_configuration_download. The candidates and carry have unknown target schema support. " + importAuthoringDirectoryQuestion + " Call get_import_configuration_download with organization_name and workspace_name of the target, configuration_version_id=state_run_configuration_version_id and prepared_target_workspace_id=workspace_id (or use next_calls.get_import_configuration_download). Before running anything: " + importLockFileCheck + " Then run terraform init -backend=false, terraform version -json and terraform providers schema -json with approval for each, and preserve the lock file. " + importProviderMismatchRule + " Use resource_schemas for managed attributes; use an identity import block only when resource_identity_schemas includes the type and Terraform is 1.12 or later, otherwise obtain the import ID from the locked provider's documentation. Follow agent_instructions for archive, secret-file and confirmation rules. " + importNeverApplyRule + " " + importNothingCreated

// importBlockedNextAction gives the agent a specific next step per blocking code.
func importBlockedNextAction(code string) string {
	const none = " " + importNothingCreated
	switch code {
	case "workspace_execution_mode_local":
		return importGuideOnlyNextAction("This workspace uses local execution mode, which stores state only and has no HCP Terraform runs, plans or provider-schema artifacts.")
	case "terraform_version_unsupported":
		return "The workspace's Terraform version is below 1.5, which does not support import blocks. Ask the user to choose a workspace on Terraform 1.5 or later; do not change the workspace version yourself." + none
	case "state_producing_run_missing":
		return importGuideOnlyNextAction("The current state was not produced by an HCP Terraform run that this server can read. Either the state was written outside a run (local execution, state push or API upload) or the producing run was deleted; the two cannot be told apart here.")
	case "target_config_not_downloadable":
		return importGuideOnlyNextAction("The run that produced the current state has no plan, and the configuration version it used cannot be downloaded (it is missing, not uploaded, archived or errored).")
	case "schema_source_plan_unavailable":
		return importAgentSchemaNextAction
	case "query_identity_version_invalid":
		return "A found resource in this QueryRun has no valid identity_version (a nonnegative integer; 0 is valid), so the whole QueryRun is not selectable import evidence and no partial candidate list is offered. Tell the user; do not select from this QueryRun, infer a version or work around it. Running a new query is the user's decision." + none
	case "execution_source_not_supported":
		return "The target execution mode is not remote or cannot be resolved; this workflow cannot plan it. Choose a supported remote target with the user." + none
	case "workspace_vcs_source_unsupported":
		return "The target is VCS-backed; this workflow does not replace its configuration through API upload. Choose an API-upload-backed remote target with the user." + none
	case "configuration_root_setting_unsupported":
		return importConfigurationRootStop + none
	case "configuration_baseline_unavailable":
		return "The target has readable state but no current configuration version to preserve. Do not treat it as blank or upload an unrelated tree; resolve the authoritative target configuration before preparing again." + none
	default:
		return "Resolve the reported evidence diagnostic and call prepare_import again." + none
	}
}

// prepareImportTool is the single reader of the QueryRun log for the focused
// import workflow. It composes the existing preparation internals.
func prepareImportTool(ctx context.Context, c *tfe.Client, input importPrepareInput) importPrepared {
	out := importPrepared{ContractVersion: importToolContractVersion, Status: "blocked", Stage: "input_validation", Organization: input.Organization, QueryRunID: input.QueryID, Diagnostics: []string{}}
	fail := func(err error) importPrepared {
		code := importDiagnosticCode(err)
		return importToolBlocked(out, code, importBlockedNextAction(code))
	}
	// Check the target before the expensive QueryRun log read: a blocked
	// call must not pay for the log.
	prov, err := readImportQueryProvenance(ctx, c, input.QueryID)
	if err != nil {
		out.Stage = "query_selection"
		return fail(err)
	}
	out.Stage = "target"
	out.SourceWorkspaceID = prov.WorkspaceID
	out.SourceOrganizationName, out.SourceWorkspaceName = prov.Organization, prov.WorkspaceName
	agentSchema := false
	// The query and target may be distinct workspaces, but never distinct
	// organizations. The source ownership is established by the QueryRun read.
	if !strings.EqualFold(prov.Organization, input.Organization) {
		return fail(importEvidenceFailure("query_organization_mismatch"))
	}
	if err := checkImportTarget(ctx, c, input, &out); err != nil {
		if importDiagnosticCode(err) != "schema_source_plan_unavailable" || out.StateRunConfigurationVersionID == "" {
			return fail(err)
		}
		agentSchema = true
	}
	discovery, err := readImportDiscoveryLog(ctx, c, prov)
	if err != nil {
		out.Stage = "query_selection"
		return fail(err)
	}
	prepared := readImportTarget(ctx, c, input, discovery, out.WorkspaceID, agentSchema)
	if prepared.WorkspaceID != "" && prepared.WorkspaceID != out.WorkspaceID {
		return fail(importEvidenceFailure("prepared_target_workspace_mismatch"))
	}
	// A target may change while the QueryRun log is being read. The second
	// target read must not silently replace the state we assessed before it.
	initialBaseline := out.Baseline
	out.Stage, out.WorkspaceID, out.ExecutionMode, out.Baseline = prepared.Stage, prepared.WorkspaceID, prepared.ExecutionMode, prepared.Baseline
	out.SchemaSource = prepared.SchemaSource
	out.Notes = append(out.Notes, prepared.Notes...)
	if prepared.Status != "prepared" && prepared.Status != "ready_for_authoring" {
		out.Diagnostics = append(out.Diagnostics, prepared.Diagnostics...)
		code := ""
		if len(prepared.Diagnostics) > 0 {
			code = prepared.Diagnostics[len(prepared.Diagnostics)-1]
		}
		out.NextAction = importBlockedNextAction(code)
		return out
	}
	if initialBaseline != nil &&
		(out.Baseline == nil || initialBaseline.StateVersionID != out.Baseline.StateVersionID ||
			(initialBaseline.StateSerial == nil) != (out.Baseline.StateSerial == nil) ||
			(initialBaseline.StateSerial != nil && out.Baseline.StateSerial != nil && *initialBaseline.StateSerial != *out.Baseline.StateSerial) ||
			initialBaseline.ConfigurationVersionID != out.Baseline.ConfigurationVersionID) {
		return fail(importEvidenceFailure("baseline_changed"))
	}
	if (prepared.Status != "ready_for_authoring" || agentSchema) && (out.Baseline == nil || out.Baseline.ConfigurationVersionID == "" || out.Baseline.StateVersionID == "" || out.Baseline.StateSerial == nil) {
		return fail(importEvidenceFailure("configuration_baseline_unavailable"))
	}
	if agentSchema {
		// The fallback CV was learned before the log read. Bind it to the
		// final state baseline and the same producing Run before authoring.
		sv, err := readImportCurrentState(ctx, c, out.WorkspaceID)
		if err != nil {
			return fail(err)
		}
		if out.Baseline == nil || out.Baseline.StateSerial == nil || sv.ID != out.Baseline.StateVersionID || sv.Serial != *out.Baseline.StateSerial || sv.Run == nil || sv.Run.ID != out.agentSchemaRunID {
			return fail(importEvidenceFailure("baseline_changed"))
		}
		r, err := readImportSchemaRun(ctx, c, out.WorkspaceID, sv)
		if err == nil || importDiagnosticCode(err) != "schema_source_plan_unavailable" || r == nil || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != out.StateRunConfigurationVersionID || r.TerraformVersion != out.agentSchemaTerraformVersion {
			return fail(importEvidenceFailure("baseline_changed"))
		}
	}
	if discovery.WorkspaceID != prov.WorkspaceID {
		return fail(importEvidenceFailure("query_provenance_unavailable"))
	}
	candidates, err := selectImportCandidates(discovery, input.Selections, prov.WorkspaceID)
	if err != nil {
		return fail(err)
	}
	if prepared.Baseline != nil && prepared.Baseline.ConfigurationVersionID != "" {
		out.HasCurrentConfiguration = true
		out.CurrentConfigurationVersionID = prepared.Baseline.ConfigurationVersionID
	}

	schemaVersion := out.agentSchemaTerraformVersion
	if prepared.SchemaSource != nil {
		schemaVersion = prepared.SchemaSource.TerraformVersion
	}
	carry, wanted, typeKeys := newImportCarry(input, candidates, schemaVersion)
	carry.Target.WorkspaceID = out.WorkspaceID

	var entries map[string]importManagedSchemaEntry
	if prepared.SchemaSource != nil {
		entries, _, err = readImportManagedSchemaSet(ctx, c, prepared.SchemaSource.RunID, wanted)
		if err != nil {
			out.Stage = "managed_schema"
			return fail(err)
		}
	}
	types, err := importPreparedTypes(carry, entries, typeKeys, schemaVersion)
	if err != nil {
		return fail(err)
	}
	addImportIdentityCompatibility(types, candidates, input.Selections)
	out.Types = types
	carry.SelectionDigest = importCarryDigest(*carry)
	out.Candidates, out.Carry = candidates, carry
	out.Status, out.Stage = "prepared", "ready_for_authoring"
	if agentSchema {
		out.Status, out.Stage = "agent_schema_required", "agent_schema"
		out.Notes = append(out.Notes, "target_schema_not_read_from_hcp; the agent must obtain it")
		out.Diagnostics = append(out.Diagnostics, "schema_source_plan_unavailable")
	} else if prepared.Status == "ready_for_authoring" {
		out.Status = "ready_for_authoring"
		out.Notes = append(out.Notes, "blank_workspace_no_target_schema; managed_type_support and identity_support are unknown")
	}
	out.AgentInstructions = importInstructionsFor(prepared.Status == "ready_for_authoring" && !agentSchema)
	rootUnsupported := out.Baseline != nil && out.Baseline.WorkingDirectory != ""
	if rootUnsupported {
		out.Notes = append(out.Notes, "configuration_root_setting_unsupported")
	}
	out.NextAction = importPreparedNextAction(&out, agentSchema, rootUnsupported)
	out.NextCalls = preparedImportNextCalls(out)
	if raw, err := json.Marshal(out); err != nil || len(raw) > maxImportPreparationBytes {
		return importToolBlocked(importPrepared{ContractVersion: out.ContractVersion, Stage: "response", Organization: out.Organization, QueryRunID: out.QueryRunID, WorkspaceID: out.WorkspaceID, Diagnostics: []string{}}, "evidence_response_limit", fmt.Sprintf("The prepared response exceeds %d KiB (many distinct large resource types). Prepare fewer distinct resource types per call. Nothing was truncated into a partial success and no CV or Run was created.", maxImportPreparationBytes/1024))
	}
	return out
}

// PrepareImportDefinition describes the prepare_import tool.
func PrepareImportDefinition() mcp.Tool {
	return mcp.NewTool("prepare_import",
		mcp.WithDescription(`Required: organization_name and workspace_name of the user-confirmed import target, query_run_id and selections [{candidate_id, managed_type}] from get_query_summary. Validate 1-100 explicitly selected Search results and prepare them for agent-authored resource and import HCL. Before this call, ask whether to import into the same Search workspace or a different one in the same organization. Cross-organization import is not supported. The returned source_workspace_id identifies Search; workspace_id identifies the target. Carry workspace_id unchanged as prepared_target_workspace_id on download and both creates, and retain carry for verification; these checks do not authenticate approval or archive bytes. Use this tool only for candidates chosen for import, not to search or filter by tag. Do not re-page the query after user confirmation. Keep the complete response (unchanged carry, all types/schemas and every selected candidate's resource_object, configuration and import_configuration); if a script handles it, return or save all of that, not only the first candidate or a summary.

next_calls.get_import_configuration_download supplies argument keys when a current or permitted state-run CV ID is known; the download tool must still establish URL availability. next_calls.create_import_cv has the target and baseline arguments but confirm_speculative_run=false until explicit review/approval. next_calls.create_import_run_base is NOT a callable Run request: add configuration_version_id from the created/uploaded CV and set confirmation true only for the same reviewed attempt. For a verified blank target, baseline fields and download are absent. types, schema_source and target describe the target workspace; candidates and carry.providers describe the QueryRun, whose provider version can differ from the target's. The result contains the target managed schema for each distinct type (managed_schema, identity_schema), per-type identity_support, the verdict from the target's own schema (supported means the target schema reports identity, not that the proposed block or values are valid; none means the target's schema has no identity, Terraform 1.12 or later; unknown means the schema could not be read; for none or unknown the import ID must come from the documentation of the target workspace's locked provider version, read from the downloaded .terraform.lock.hcl), identity_compatibility (how the Search identity keys and identity schema version fit the target identity schema, with a short guidance sentence; if a type is not the same version, tell the user before authoring that identity stays unverified even if the plan only imports), the target Terraform versions (target.terraform_version_setting, the workspace setting that new runs use and may be a constraint, and target.terraform_version_last_run, exact, from the run that produced the state), the workspace baseline, a carry block, and for each selected candidate its resource_object (the attributes the Search provider observed, absent when the query did not capture them) and configuration and import_configuration (Search-generated HCL drafts of the resource block and the import block). Send the carry block unchanged with each verify_import_plan poll when bindings are known.

Before reading the QueryRun log it blocks targets the workflow does not support: local or agent execution mode, VCS-backed source, non-empty configuration root, a Terraform version below 1.5, and a current state with no readable producing run or plan. A Terraform version below 1.12 proceeds with a note that identity is unavailable. Each block says no CV or Run was created.

Every found resource in the QueryRun must carry a valid identity_version (a nonnegative integer; 0 is valid). If any does not, the whole QueryRun is not selectable import evidence: the call is blocked with query_identity_version_invalid and no partial candidate list is offered. Each selected candidate's version is carried in the carry block (search_identity_version) and covered by its digest; candidate_id does not include it.

The current configuration is not downloaded here. If has_current_configuration is true, ask the user for the authoring directory (an empty or new directory), then call get_import_configuration_download. If the target workspace is new (no configuration and no state), a directory that already holds the user's Terraform files is fine: tell the user, and upload an explicit, reviewed file list. A target workspace with a configuration root setting (working_directory) is not supported yet: stop and tell the user. It never creates a CV or Run and does not authorize one.`),
		mcp.WithTitleAnnotation("Prepare Search import selection"),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true), mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("organization_name", mcp.Required(), mcp.Description("HCP Terraform organization of the target workspace.")),
		mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Target workspace name.")),
		mcp.WithString("query_run_id", mcp.Required(), mcp.Description("Finished no-code query run that produced the selected candidates.")),
		mcp.WithArray("selections", mcp.Required(), mcp.Description("One to 100 explicit candidates from one finished QueryRun, from any page of get_query_summary."), mcp.MinItems(1), mcp.MaxItems(maxImportSelections), mcp.Items(map[string]any{
			"type": "object", "required": []string{"candidate_id", "managed_type"},
			"properties":           map[string]any{"candidate_id": map[string]any{"type": "string"}, "managed_type": map[string]any{"type": "string", "description": "Proposed target managed resource type."}},
			"additionalProperties": false,
		})),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithOutputSchema[importPrepared]())
}

// HandlePrepareImport serves prepare_import.
func HandlePrepareImport(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	var input importPrepareInput
	if err := decodeImportToolArguments(args, &input, maxPrepareArgumentBytes); err != nil {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "input_validation", Diagnostics: []string{}}, "import_input_invalid", "Supply organization_name, workspace_name, query_run_id and 1-100 selections with candidate_id and managed_type."))
	}
	if !importInputName(input.Organization) || !importInputName(input.Workspace) || !importInputName(input.QueryID) || len(input.Selections) < 1 || len(input.Selections) > maxImportSelections || !validImportSelections(input) {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "input_validation", Diagnostics: []string{}}, "import_selection_invalid", "Provide 1-100 distinct candidate IDs from one finished QueryRun, each with a managed_type."))
	}
	if err := client.AuthorizeOrganization(ctx, input.Organization); err != nil {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "authorization", Diagnostics: []string{}}, "organization_not_allowed", "Use an organization allowed by this server."))
	}
	ctx, cancel := context.WithTimeout(ctx, importToolRequestTimeout)
	defer cancel()
	c, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "client", Diagnostics: []string{}}, "backend_client_unavailable", "Supply current backend credentials and retry."))
	}
	return importToolResult(prepareImportTool(ctx, c, input))
}

func decodeImportToolArguments(args map[string]any, target any, limit int) error {
	raw, err := json.Marshal(args)
	if err != nil || len(raw) > limit {
		return importEvidenceFailure("input_invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func importToolResult(response any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(response)
	if err != nil {
		return mcp.NewToolResultError("response_encoding_failed"), nil
	}
	result := mcp.NewToolResultStructured(response, string(raw))
	if b, ok := response.(interface{ isBlocked() bool }); ok {
		result.IsError = b.isBlocked()
	}
	return result, nil
}

// newImportCarry builds the carry block for the selected candidates. It also
// returns the managed types wanted per provider source and the sorted
// "source/type" keys of the distinct types, so the schema artifact is read once.
func newImportCarry(input importPrepareInput, candidates []importDiscoveryCandidate, schemaVersion string) (*importCarryBlock, map[string][]string, []string) {
	carry := &importCarryBlock{QueryRunID: input.QueryID, Providers: map[string]importCarryProvider{}, Target: importCarryTarget{TerraformVersion: schemaVersion, IdentitySupport: map[string]string{}}}
	wanted := map[string][]string{}
	seenType := map[string]bool{}
	for i, candidate := range candidates {
		managed := input.Selections[i].ManagedType
		carry.Providers[candidate.ResourceType] = importCarryProvider{Source: candidate.Provider.Source, Version: candidate.Provider.Version}
		carry.Candidates = append(carry.Candidates, importCarryCandidate{CandidateID: candidate.CandidateID, ListType: candidate.ResourceType, ManagedType: managed, Identity: candidate.Identity, SearchIdentityVersion: candidate.IdentityVersion})
		key := candidate.Provider.Source + "/" + managed
		if !seenType[key] {
			seenType[key] = true
			wanted[candidate.Provider.Source] = append(wanted[candidate.Provider.Source], managed)
		}
	}
	keys := make([]string, 0, len(seenType))
	for key := range seenType {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return carry, wanted, keys
}

// importPreparedTypes describes each distinct managed type from the schema
// entries read for it, and records its identity support in the carry block. A
// type with no entry (a blank workspace, or the agent supplies the schema)
// stays unknown.
func importPreparedTypes(carry *importCarryBlock, entries map[string]importManagedSchemaEntry, keys []string, schemaVersion string) ([]importPreparedType, error) {
	types := make([]importPreparedType, 0, len(keys))
	for _, key := range keys {
		split := strings.LastIndex(key, "/")
		source, managed := key[:split], key[split+1:]
		t := importPreparedType{ProviderSource: source, ManagedType: managed, ManagedTypeSupport: "unknown", IdentitySupport: importIdentityUnknown}
		if entry, ok := entries[key]; ok {
			t.ManagedTypeSupport = "supported"
			t.IdentitySupport = importIdentitySupportFor(entry.Identity, schemaVersion)
			if err := decodeImportEvidenceJSONLimit(entry.Managed, &t.ManagedSchema, maxImportSchemaBytes); err != nil {
				return nil, err
			}
			if len(entry.Identity) > 0 {
				if err := decodeImportEvidenceJSON(entry.Identity, &t.IdentitySchema); err != nil {
					return nil, err
				}
			}
		}
		carry.Target.IdentitySupport[managed] = t.IdentitySupport
		types = append(types, t)
	}
	return types, nil
}

// importPreparedNextAction picks the next step for a prepared response: the
// agent-supplied schema path, an early stop for a configuration root setting,
// downloading the current configuration, or authoring in a blank workspace.
// identityCompatNextAction names the types whose identity fit is not confirmed so
// the agent tells the user before authoring. It is empty when every type has
// the same identity schema version.
func identityCompatNextAction(types []importPreparedType) string {
	seen := map[string]bool{}
	var names []string
	for _, t := range types {
		c := t.IdentityCompatibility
		if c == nil || c.Status == identityCompatSameVersion || seen[t.ManagedType] {
			continue
		}
		seen[t.ManagedType] = true
		names = append(names, t.ManagedType)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return " Identity compatibility is not confirmed for " + strings.Join(names, ", ") + " (types[].identity_compatibility); tell the user before authoring. " + importIdentityUnverifiedLead
}

func importPreparedNextAction(out *importPrepared, agentSchema, rootUnsupported bool) string {
	compat := identityCompatNextAction(out.Types)
	switch {
	case rootUnsupported:
		return importConfigurationRootStop + " get_query_summary still lists the candidates. " + importNothingCreated
	case agentSchema:
		return importAgentSchemaNextAction
	case out.HasCurrentConfiguration:
		return importKeepGeneratedShort + compat + " " + importTargetScopeShort + " " + importKeepResultShort + " " + importModuleTargetShort + " " + importAuthoringDirectoryQuestion + " Call get_import_configuration_download with organization_name and workspace_name of the target, configuration_version_id=current_configuration_version_id and prepared_target_workspace_id=workspace_id (or use next_calls.get_import_configuration_download); author the blocks in the preserved archive root. Review the exact archive and obtain confirmation before either create; follow agent_instructions for archive and confirmation details. " + importNothingCreated
	default:
		return "The target workspace has no current configuration. " + importKeepGeneratedShort + compat + " " + importKeepResultShort + " " + importModuleTargetShort + " " + importNewWorkspaceDirectoryQuestion + " " + importBlankAuthoringDirectoryRule + " Author the complete configuration and lock there. Review with the user before create_import_cv; follow agent_instructions for confirmation and archive rules. " + importNothingCreated
	}
}
