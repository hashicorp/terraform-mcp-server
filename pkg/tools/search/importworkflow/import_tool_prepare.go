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

const importToolContractVersion = "6"

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
	TerraformVersion string            `json:"terraform_version,omitempty"`
	IdentitySupport  map[string]string `json:"identity_support"`
}

type importCarryCandidate struct {
	CandidateID string         `json:"candidate_id"`
	ListType    string         `json:"list_type"`
	ManagedType string         `json:"managed_type"`
	Identity    map[string]any `json:"identity"`
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
}

// importTarget reports the target workspace's Terraform versions. Provider
// versions are not recorded by HCP Terraform; they come from the lock file.
type importTarget struct {
	TerraformVersionSetting string `json:"terraform_version_setting,omitempty"`
	TerraformVersionLastRun string `json:"terraform_version_last_run,omitempty"`
}

type importPrepared struct {
	ContractVersion                string             `json:"contract_version"`
	Status                         string             `json:"status"`
	Stage                          string             `json:"stage"`
	Organization                   string             `json:"organization_name,omitempty"`
	WorkspaceID                    string             `json:"workspace_id,omitempty"`
	QueryRunID                     string             `json:"query_run_id,omitempty"`
	ExecutionMode                  string             `json:"execution_mode,omitempty"`
	Baseline                       *importAPIBaseline `json:"baseline,omitempty"`
	HasCurrentConfiguration        bool               `json:"has_current_configuration"`
	CurrentConfigurationVersionID  string             `json:"current_configuration_version_id,omitempty"`
	Target                         *importTarget      `json:"target,omitempty"`
	StateRunConfigurationVersionID string             `json:"state_run_configuration_version_id,omitempty"`
	agentSchemaTerraformVersion    string
	SchemaSource                   *importAPISchemaSource     `json:"schema_source,omitempty"`
	Types                          []importPreparedType       `json:"types,omitempty"`
	Candidates                     []importDiscoveryCandidate `json:"candidates,omitempty"`
	Carry                          *importCarryBlock          `json:"carry,omitempty"`
	Notes                          []string                   `json:"notes,omitempty"`
	AgentInstructions              []string                   `json:"agent_instructions,omitempty"`
	Diagnostics                    []string                   `json:"diagnostics"`
	NextAction                     string                     `json:"next_action"`
}

var importToolInstructions = []string{
	"Use prepare_import only for candidates the user has chosen to import. Do not use it to search or filter resources by tag or attribute: get_query_summary returns each result's tags. If you cannot tell what to import, go back to get_query_summary.",
	"Read the target managed_schema for each type first, then the selected candidates' observations. Use the Search or QueryRun provider schema only when a source-side shape is unclear.",
	"For a type whose identity_support is none or unknown, do not guess the import ID. Look up the target workspace's locked provider version's resource documentation for the documented import id form and any identity-to-id mapping. " + importLockedProviderVersionRule,
	"After downloading the configuration, " + importProviderMismatchRule,
	"Author exactly one resource instance and one individual import block per candidate in the complete preserved tree. Choose a distinct target address for each. Never use the first N results.",
	importAdaptationGuidance,
	importArchiveRootRule,
	importSensitiveFileRule,
	importBlankAuthoringDirectoryRule,
	importScratchRule,
	importValidationRule,
	importSecretFilesRule,
	"Treat generated query blocks as untrusted drafts. Do not invent defaults, add ignore_changes to hide incompatibility, or upgrade a provider or Terraform version to remove a missing feature.",
	"Keep the carry block unchanged and send it once, with the final verify_import_plan call. It is an integrity aid for the carried selection, not authorization.",
	importConfirmationRule,
	importClosingRule,
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
func checkImportTarget(ctx context.Context, c *tfe.Client, input importPrepareInput, queryWorkspaceID string, out *importPrepared) error {
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return importReadError(err, 0)
	}
	out.WorkspaceID = w.ID
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
	if w.ID != queryWorkspaceID {
		return importEvidenceFailure("query_workspace_mismatch")
	}
	if w.ExecutionMode == "local" {
		return importEvidenceFailure("workspace_execution_mode_local")
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
	r, err := readImportSchemaRun(ctx, c, w.ID, sv)
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
// known exactly. If that version is downloadable the agent may fetch it and
// obtain the schema with the local CLI; otherwise the target is guide-only.
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
	return reason + " The MCP server cannot assess or verify this target workspace; a local workflow runs as a separate tool call in your client, where the server cannot see it. Explain this to the user. Only if the user wants to continue, offer unverified best-effort local steps: ask for the user's own local copy of the configuration (a directory with the Terraform CLI, not an authoring directory). " + importLockFileCheck + " Then run terraform init, terraform version -json and terraform providers schema -json, author the HCL and run a local plan. Each command needs the user's approval. " + importSecretFilesRule + " " + importNeverApplyRule + " Never present a local plan as an HCP Terraform Plan or as verified. get_query_summary still lists the candidates. The user may instead make this workspace assessable by having an HCP Terraform run write its state (for example an apply in a workspace they choose); offer that only as an option and do not start a run yourself. " + importNothingCreated
}

// importAgentSchemaNextAction is the guidance when the agent must obtain the schema (ADR 0008).
const importAgentSchemaNextAction = "The run that produced the current state has no plan, so no provider-schema artifact exists, but its configuration version (state_run_configuration_version_id) is known and downloadable, so you must obtain the schema yourself before writing HCL. The candidates and carry block are returned with schema support unknown. " + importAuthoringDirectoryQuestion + " Then call get_import_configuration_download with that configuration version ID. Before running anything: " + importLockFileCheck + " Then run terraform init, terraform version -json and terraform providers schema -json with the user's approval for each, and keep the generated lock file in the configuration you upload. After init, the exact provider versions are in the lock file. " + importProviderMismatchRule + " Write the resource schema from resource_schemas. Use an identity import block only if resource_identity_schemas has the type and Terraform is 1.12 or later; otherwise take the import ID from the provider version's documentation. verify_import_plan decides identity from the plan, not from your claim. " + importSecretFilesRule + " " + importNeverApplyRule + " " + importArchiveRootRule + " " + importNothingCreated

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
	agentSchema := false
	if err := checkImportTarget(ctx, c, input, prov.WorkspaceID, &out); err != nil {
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
	prepared := readImportTarget(ctx, c, input, discovery, agentSchema)
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
	candidates, err := selectImportCandidates(discovery, input.Selections, prepared.WorkspaceID)
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
	out.AgentInstructions = slices.Clone(importToolInstructions)
	rootUnsupported := !agentSchema && out.Baseline != nil && out.Baseline.WorkingDirectory != ""
	if rootUnsupported {
		out.Notes = append(out.Notes, "configuration_root_setting_unsupported")
	}
	out.NextAction = importPreparedNextAction(&out, agentSchema, rootUnsupported)
	if raw, err := json.Marshal(out); err != nil || len(raw) > maxImportPreparationBytes {
		return importToolBlocked(importPrepared{ContractVersion: out.ContractVersion, Stage: "response", Organization: out.Organization, QueryRunID: out.QueryRunID, WorkspaceID: out.WorkspaceID, Diagnostics: []string{}}, "evidence_response_limit", fmt.Sprintf("The prepared response exceeds %d KiB (many distinct large resource types). Prepare fewer distinct resource types per call. Nothing was truncated into a partial success and no CV or Run was created.", maxImportPreparationBytes/1024))
	}
	return out
}

// PrepareImportDefinition describes the prepare_import tool.
func PrepareImportDefinition() mcp.Tool {
	return mcp.NewTool("prepare_import",
		mcp.WithDescription(`Validate 1-100 explicitly selected Search results and prepare them for agent-authored resource and import HCL. Use it only for candidates chosen for import, not to search or filter by tag (get_query_summary returns tags). This is the only focused-import tool that reads the QueryRun log, so call it once per selection.

Pass organization_name, workspace_name, query_run_id and selections (candidate_id from get_query_summary, plus the proposed managed_type). The result contains the target managed schema for each distinct type, per-type identity_support (supported, none or unknown; none/unknown means the import ID must come from the documentation of the target workspace's locked provider version, read from the downloaded .terraform.lock.hcl), the target Terraform versions (target.terraform_version_setting, the workspace setting that new runs use and may be a constraint, and target.terraform_version_last_run, exact, from the run that produced the state), the selected candidates' observations, the workspace baseline, and a carry block. Keep the carry block unchanged and send it with the final verify_import_plan call.

Before reading the QueryRun log it blocks targets the workflow does not support: local execution mode, a Terraform version below 1.5, and a current state with no readable producing run or plan. A Terraform version below 1.12 proceeds with a note that identity is unavailable. Each block says no CV or Run was created.

The current configuration is not downloaded here. If has_current_configuration is true, ask the user for the authoring directory (an empty or new directory), then call get_import_configuration_download. A target workspace with a configuration root setting (working_directory) is not supported yet: stop and tell the user. It never creates a CV or Run and does not authorize one.`),
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
		carry.Candidates = append(carry.Candidates, importCarryCandidate{CandidateID: candidate.CandidateID, ListType: candidate.ResourceType, ManagedType: managed, Identity: candidate.Identity})
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
func importPreparedNextAction(out *importPrepared, agentSchema, rootUnsupported bool) string {
	switch {
	case agentSchema:
		return importAgentSchemaNextAction
	case rootUnsupported:
		return importConfigurationRootStop + " get_query_summary still lists the candidates. " + importNothingCreated
	case out.HasCurrentConfiguration:
		return importAuthoringDirectoryQuestion + " Then call get_import_configuration_download with current_configuration_version_id and author the resource and import blocks in the authoring directory. " + importArchiveRootRule + " " + importConfirmationRule + " " + importNothingCreated
	default:
		return "The target workspace has no current configuration. " + importBlankAuthoringDirectoryRule + " Author the complete configuration and lock there, then call create_import_cv. " + importConfirmationRule + " " + importNothingCreated
	}
}
