// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
)

type importPlannedResource struct {
	Address         string   `json:"address"`
	Mode            string   `json:"mode,omitempty"`
	Type            string   `json:"type,omitempty"`
	ProviderSource  string   `json:"provider_source,omitempty"`
	Actions         []string `json:"actions"`
	ImportPresent   bool     `json:"import_present"`
	ImportIDPresent bool     `json:"import_id_present"`
}

// Refresh drift is separate from planned resource_changes. Attribute values
// remain in the detailed plan JSON, not in the compact status response.
type importResourceDrift struct {
	Address string   `json:"address"`
	Actions []string `json:"actions"`
}

type importPlanFacts struct {
	FormatVersion           string                  `json:"format_version"`
	Selected                *importPlannedResource  `json:"selected,omitempty"`
	SelectedAddress         string                  `json:"selected_address"`
	SelectedEntries         int                     `json:"selected_entries"`
	ResourceChangeCount     int                     `json:"resource_change_count"`
	OtherManagedActions     []importPlannedResource `json:"other_managed_actions"`
	OtherImports            []importPlannedResource `json:"other_imports"`
	OtherManagedActionCount int                     `json:"other_managed_action_count"`
	OtherImportCount        int                     `json:"other_import_count"`
	OutputChangeCount       int                     `json:"output_change_count"`
	DriftCount              int                     `json:"drift_count"`
	DriftEntries            []importResourceDrift   `json:"drift_entries"`
	DeferredCount           int                     `json:"deferred_count"`
	Truncated               bool                    `json:"truncated"`
}

type importExecutionResponse struct {
	Stage                  string           `json:"stage"`
	ConfigurationVersionID string           `json:"configuration_version_id,omitempty"`
	ConfigurationStatus    string           `json:"configuration_status,omitempty"`
	UploadURL              string           `json:"upload_url,omitempty"`
	UploadInstructions     string           `json:"upload_instructions,omitempty"`
	RunID                  string           `json:"run_id,omitempty"`
	RunStatus              string           `json:"run_status,omitempty"`
	PlanID                 string           `json:"plan_id,omitempty"`
	PlanStatus             string           `json:"plan_status,omitempty"`
	TargetAddress          string           `json:"target_address,omitempty"`
	PlanFacts              *importPlanFacts `json:"plan_facts,omitempty"`
}

func importExecutionResult(input importPrepareInput) importPreparation {
	return importPreparation{ContractVersion: importPreparationContractVersion, Status: "blocked", Stage: input.Phase, Organization: input.Organization, QueryRunID: input.QueryID, Diagnostics: []string{}, NextAction: "Resolve the reported diagnostic before further execution. Do not blindly repeat a create request."}
}

func importExecutionFailure(result importPreparation, code string) importPreparation {
	result.Diagnostics = append(result.Diagnostics, code)
	return result
}

// Input failures occur before either create POST. Keep the phase code for
// existing callers, and add a field-level reason for the agent to repair.
func importPhaseInputFailure(result importPreparation, code, fieldCode, guidance string) importPreparation {
	result.Diagnostics = append(result.Diagnostics, code, fieldCode)
	result.NextAction = guidance + " No CV or Run was created by this request."
	return result
}

func createImportSpeculativeCV(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	// With no baseline or schema-source IDs, the target address distinguishes
	// a direct blank-workspace import from an optional provider-only schema probe.
	// Establish blankness from Atlas before POST in either case.
	if input.BaselineCVID == "" && input.BaselineStateID == "" && input.BaselineSerial == 0 && input.SchemaCVID == "" && input.SchemaRunID == "" {
		return createImportBlankCV(ctx, c, input, logger)
	}
	if input.TargetAddress != "" {
		if input.SchemaRunID != "" {
			return importPhaseInputFailure(result, "speculative_upload_input_invalid", "target_address_not_allowed_on_schema_upload", "Omit target_address on blank-workspace upload with bootstrap schema IDs; supply it later on the import plan.")
		}
		return importPhaseInputFailure(result, "speculative_upload_input_invalid", "target_address_not_allowed_on_existing_upload", "Omit target_address on existing-workspace upload; supply it later on the import plan after the reviewed archive is uploaded.")
	}
	if !input.ConfirmSpeculativeRun {
		return importPhaseInputFailure(result, "speculative_upload_input_invalid", "confirm_speculative_run_required", "After review, set confirm_speculative_run=true to authorize only a speculative CV create.")
	}
	if input.SchemaRunID == "" && (input.BaselineCVID == "" || input.BaselineStateID == "") {
		return importPhaseInputFailure(result, "speculative_upload_input_invalid", "baseline_ids_required", "Existing-workspace upload requires baseline_cv_id, baseline_state_id and baseline_state_serial from preparation; do not treat missing state as blank.")
	}
	if !input.ConfirmSpeculativeRun || input.ConfigurationVersionID != "" || input.RunID != "" || input.TargetAddress != "" || (input.SchemaRunID == "" && (!importInputName(input.BaselineCVID) || !importInputName(input.BaselineStateID))) || (input.SchemaRunID != "" && (input.BaselineCVID != "" || input.BaselineStateID != "" || input.BaselineSerial != 0)) {
		return importPhaseInputFailure(result, "speculative_upload_input_invalid", "upload_phase_fields_invalid", "Upload requires the verified baseline fields for an existing workspace OR bootstrap schema IDs for a blank workspace; omit configuration_version_id, run_id and target_address, and never mix baseline and schema IDs.")
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(importReadError(err, 0)))
	}
	result.WorkspaceID = w.ID
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return importExecutionFailure(result, "workspace_ownership_unverified")
	}
	if w.ExecutionMode != "remote" || w.WorkingDirectory != "" || w.VCSRepo != nil {
		return importExecutionFailure(result, "execution_source_not_supported")
	}
	prepared := prepareImportFromAPIs(ctx, c, input)
	if prepared.Status != "prepared" {
		return prepared
	}
	if prepared.Baseline.ConfigurationVersionID != input.BaselineCVID || prepared.Baseline.StateVersionID != input.BaselineStateID || (input.SchemaRunID == "" && (prepared.Baseline.StateSerial == nil || *prepared.Baseline.StateSerial != input.BaselineSerial)) {
		return importExecutionFailure(result, "baseline_changed")
	}
	result.Execution = &importExecutionResponse{Stage: "cv_create_outcome_unknown"}
	mutationClient, err := client.NewTfeClientForImportMutation(ctx, logger)
	if err != nil {
		return importExecutionFailure(result, "backend_client_unavailable")
	}
	no := false
	yes := true
	cv, err := mutationClient.ConfigurationVersions.Create(ctx, w.ID, tfe.ConfigurationVersionCreateOptions{AutoQueueRuns: &no, Speculative: &yes})
	if err != nil {
		result.NextAction = "Creation outcome is unknown. Reconcile workspace CVs in Atlas before creating another; the upload URL is only returned by a successful create response."
		return importExecutionFailure(result, "cv_create_outcome_unknown")
	}
	result.Execution.ConfigurationVersionID = cv.ID
	if cv.ID == "" || !cv.Speculative || cv.Provisional || cv.AutoQueueRuns || !validImportArtifactLocation(ctx, cv.UploadURL) {
		return importExecutionFailure(result, "cv_create_response_invalid")
	}
	result.Execution.Stage = "awaiting_agent_upload"
	result.Status, result.Stage = "awaiting_agent_upload", "upload"
	result.Execution.UploadURL = cv.UploadURL
	result.Execution.UploadInstructions = "Retain this one-use upload_url securely until the client-local PUT of the complete agent-owned .tar.gz succeeds (Content-Type: application/octet-stream). Do not log or persist the URL. Poll status using the workspace and configuration_version_id. If the URL is lost, status cannot reacquire it: reconcile the pending CV before considering a new reviewed speculative CV; do not blindly create another. Never upload individual .tf files or use this CV for apply."
	result.NextAction = "Agent uploads the complete archive directly; retain the one-use URL securely until transfer is confirmed. If it is lost, reconcile the pending CV before considering another reviewed create. MCP has not read configuration or created a run."
	return result
}

// A CV read has no workspace relationship in go-tfe. The workspace-scoped
// typed list establishes membership, with a strict scan bound (never treat an
// incomplete scan as proof of absence). The caller cannot assert ownership.
func readImportExecutionCV(ctx context.Context, c *tfe.Client, input importPrepareInput) (*tfe.Workspace, *tfe.ConfigurationVersion, error) {
	if !importInputName(input.ConfigurationVersionID) || input.QueryID != "" || len(input.Selections) != 0 || input.BaselineCVID != "" || input.BaselineStateID != "" || input.BaselineSerial != 0 {
		return nil, nil, importEvidenceFailure("execution_input_invalid")
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return nil, nil, importReadError(err, 0)
	}
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return nil, nil, importEvidenceFailure("workspace_ownership_unverified")
	}
	found := false
	for page := 1; page <= 20; page++ {
		list, err := c.ConfigurationVersions.List(ctx, w.ID, &tfe.ConfigurationVersionListOptions{ListOptions: tfe.ListOptions{PageNumber: page, PageSize: 100}})
		if err != nil {
			return nil, nil, importReadError(err, 0)
		}
		if list == nil || list.Pagination == nil {
			return nil, nil, importEvidenceFailure("execution_cv_membership_unverified")
		}
		for _, item := range list.Items {
			if item != nil && item.ID == input.ConfigurationVersionID {
				found = true
			}
		}
		if found {
			break
		}
		if list.Pagination.NextPage == 0 {
			return nil, nil, importEvidenceFailure("execution_cv_workspace_mismatch")
		}
		if list.Pagination.NextPage <= page {
			return nil, nil, importEvidenceFailure("execution_cv_membership_unverified")
		}
		if page == 20 {
			return nil, nil, importEvidenceFailure("execution_cv_membership_limit")
		}
	}
	cv, err := c.ConfigurationVersions.Read(ctx, input.ConfigurationVersionID)
	if err != nil {
		return nil, nil, importReadError(err, 0)
	}
	if cv.ID != input.ConfigurationVersionID || !cv.Speculative || cv.Provisional || cv.AutoQueueRuns {
		return nil, nil, importEvidenceFailure("execution_cv_not_speculative")
	}
	return w, cv, nil
}

func createImportPlanRun(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	if input.BaselineCVID == "" && input.BaselineStateID == "" && input.BaselineSerial == 0 && input.SchemaCVID == "" && input.SchemaRunID == "" {
		return createImportBlankRun(ctx, c, input, logger)
	}
	if !validImportTargetAddress(input.TargetAddress) {
		return importPhaseInputFailure(result, "plan_input_invalid", "target_address_required_for_import_plan", "Provide a valid target_address for the reviewed resource/import block; only a blank-workspace provider-only schema probe omits it.")
	}
	if !input.ConfirmSpeculativeRun {
		return importPhaseInputFailure(result, "plan_input_invalid", "confirm_speculative_run_required", "After review, set confirm_speculative_run=true to authorize only a CV-bound plan-only Run.")
	}
	if input.ConfigurationVersionID == "" {
		return importPhaseInputFailure(result, "plan_input_invalid", "configuration_version_id_required", "Supply the uploaded speculative configuration_version_id returned by upload.")
	}
	if input.SchemaRunID == "" && (input.BaselineCVID == "" || input.BaselineStateID == "") {
		return importPhaseInputFailure(result, "plan_input_invalid", "baseline_ids_required", "Existing-workspace plan requires baseline_cv_id, baseline_state_id and baseline_state_serial from preparation.")
	}
	if !input.ConfirmSpeculativeRun || input.RunID != "" || !validImportTargetAddress(input.TargetAddress) || (input.SchemaRunID == "" && (!importInputName(input.BaselineCVID) || !importInputName(input.BaselineStateID))) || (input.SchemaRunID != "" && (input.BaselineCVID != "" || input.BaselineStateID != "" || input.BaselineSerial != 0)) {
		return importPhaseInputFailure(result, "plan_input_invalid", "plan_phase_fields_invalid", "Use an uploaded CV ID, target_address, and verified baseline fields OR blank-workspace bootstrap schema IDs; omit run_id and never mix baseline and schema IDs.")
	}
	prepared := prepareImportFromAPIs(ctx, c, input)
	if prepared.Status != "prepared" {
		return prepared
	}
	if prepared.Baseline.ConfigurationVersionID != input.BaselineCVID || prepared.Baseline.StateVersionID != input.BaselineStateID || (input.SchemaRunID == "" && (prepared.Baseline.StateSerial == nil || *prepared.Baseline.StateSerial != input.BaselineSerial)) {
		return importExecutionFailure(result, "baseline_changed")
	}
	// The baseline is supplied by the agent and verified against current Atlas
	// state, not against a record retained by this server.
	lookup := input
	lookup.BaselineCVID, lookup.BaselineStateID, lookup.BaselineSerial = "", "", 0
	lookup.QueryID, lookup.Selections = "", nil
	w, cv, err := readImportExecutionCV(ctx, c, lookup)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.WorkspaceID = w.ID
	result.QueryRunID = input.QueryID
	result.ManagedType = input.Selections[0].ManagedType
	result.Execution = &importExecutionResponse{Stage: "ready_for_plan", ConfigurationVersionID: cv.ID, TargetAddress: input.TargetAddress}
	if importCurrentConfigurationID(w) != input.BaselineCVID || w.ExecutionMode != "remote" || w.WorkingDirectory != "" || w.VCSRepo != nil {
		return importExecutionFailure(result, "baseline_changed")
	}
	if input.SchemaRunID != "" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return importExecutionFailure(result, importDiagnosticCode(err))
		}
	} else {
		sv, err := readImportCurrentState(ctx, c, w.ID)
		if err != nil {
			return importExecutionFailure(result, importDiagnosticCode(err))
		}
		if sv.ID != input.BaselineStateID || sv.Serial != input.BaselineSerial {
			return importExecutionFailure(result, "baseline_changed")
		}
	}
	if cv.Status != tfe.ConfigurationUploaded {
		return importExecutionFailure(result, "execution_cv_not_uploaded")
	}
	result.Execution.Stage = "run_create_outcome_unknown"
	mutationClient, err := client.NewTfeClientForImportMutation(ctx, logger)
	if err != nil {
		return importExecutionFailure(result, "backend_client_unavailable")
	}
	message := "Speculative Search import inspection"
	r, err := mutationClient.Runs.Create(ctx, tfe.RunCreateOptions{Workspace: w, ConfigurationVersion: cv, PlanOnly: tfe.Bool(true), AutoApply: tfe.Bool(false), AllowConfigGeneration: tfe.Bool(false), Message: &message})
	if err != nil {
		result.NextAction = "Run creation outcome is unknown. Inspect Atlas Runs for this CV before another create; do not retry blindly."
		return importExecutionFailure(result, "run_create_outcome_unknown")
	}
	result.Execution.RunID = r.ID
	result.Execution.Stage = "run_created"
	if r.ID == "" || !r.PlanOnly || r.Workspace == nil || r.Workspace.ID != w.ID || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != cv.ID {
		return importExecutionFailure(result, "run_association_unverified")
	}
	result.Status, result.Stage = "pending", "plan"
	result.NextAction = "Poll status with workspace, configuration_version_id, run_id and target_address. The speculative Run has not imported into persisted state."
	return result
}

func validImportTargetAddress(address string) bool {
	return strings.TrimSpace(address) == address && address != "" && len(address) <= 512 && !strings.ContainsAny(address, "\x00\r\n")
}

// Every status call derives current facts from Atlas and the caller's explicit
// workspace/CV/Run IDs. An upload URL cannot be reacquired from a CV read.
func readImportExecutionStatus(ctx context.Context, c *tfe.Client, input importPrepareInput) importPreparation {
	result := importExecutionResult(input)
	if input.ConfirmSpeculativeRun || (input.TargetAddress != "" && !validImportTargetAddress(input.TargetAddress)) || (input.RunID == "" && input.TargetAddress != "") || (input.RunID != "" && !importInputName(input.RunID)) {
		return importExecutionFailure(result, "execution_input_invalid")
	}
	w, cv, err := readImportExecutionCV(ctx, c, input)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.WorkspaceID = w.ID
	result.Execution = &importExecutionResponse{Stage: "status", ConfigurationVersionID: cv.ID, RunID: input.RunID, TargetAddress: input.TargetAddress}
	result.Execution.ConfigurationStatus = string(cv.Status)
	if input.RunID == "" {
		if cv.Status != tfe.ConfigurationUploaded {
			if cv.Status == tfe.ConfigurationErrored || cv.Status == tfe.ConfigurationArchived {
				return importExecutionFailure(result, "execution_cv_unavailable")
			}
			result.Status, result.Stage = "awaiting_agent_upload", "status"
			result.NextAction = "Use the original upload URL if still valid. If lost or expired, status cannot reacquire it: reconcile the pending CV in Atlas before considering another reviewed speculative CV; do not blindly create. Poll by configuration_version_id after agent upload. No Run ID was supplied; this does not establish that a Run does not exist."
			return result
		}
		result.Status, result.Stage = "ready_for_plan", "status"
		result.NextAction = "CV is uploaded. If no prior Run create had an uncertain outcome, call plan with this CV ID, query selection and confirmation. For a blank-workspace import, include the selected target_address and omit baseline/schema IDs. Otherwise reconcile existing Runs in Atlas first."
		return result
	}
	r, err := c.Runs.Read(ctx, input.RunID)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(importReadError(err, 0)))
	}
	result.Execution.RunStatus = string(r.Status)
	if r.ID != input.RunID || !r.PlanOnly || r.Workspace == nil || r.Workspace.ID != w.ID || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != cv.ID {
		return importExecutionFailure(result, "run_association_unverified")
	}
	if r.Plan == nil || r.Plan.ID == "" {
		result.Status, result.Stage = "pending", "status"
		result.NextAction = "Poll the same Run ID; no plan is available yet."
		return result
	}
	result.Execution.PlanID = r.Plan.ID
	p, err := c.Plans.Read(ctx, r.Plan.ID)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(importReadError(err, 0)))
	}
	result.Execution.PlanStatus = string(p.Status)
	if p.Status == tfe.PlanErrored || p.Status == tfe.PlanCanceled || p.Status == tfe.PlanUnreachable || r.Status == tfe.RunErrored || r.Status == tfe.RunCanceled || r.Status == tfe.RunDiscarded {
		result.Status, result.Stage = "failed", "status"
		result.Diagnostics = []string{"speculative_plan_failed"}
		result.AgentInstructions = []string{"Inspect the exact failed Run/Plan and bounded logs for diagnostic evidence. A partial change summary is not a completed import plan; do not apply or infer a replacement cause without plan evidence.", "Repair the agent-owned configuration, review changes, and create a new speculative CV/Run only after the prior outcome is known. Never blindly retry an uncertain create."}
		result.NextAction = "Use existing run/plan/log tools to inspect diagnostics; repair and review HCL locally before a new speculative CV/Run."
		return result
	}
	if p.Status != tfe.PlanFinished || r.Status != tfe.RunPlannedAndFinished {
		result.Status, result.Stage = "pending", "status"
		result.NextAction = "Poll this exact Run until the plan and required run stages finish."
		return result
	}
	if input.TargetAddress == "" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return importExecutionFailure(result, importDiagnosticCode(err))
		}
		if _, err := readImportBootstrapSchemaRun(ctx, c, w, cv.ID, r.ID); err != nil {
			return importExecutionFailure(result, importDiagnosticCode(err))
		}
		result.Status, result.Stage = "bootstrap_schema_ready", "status"
		result.NextAction = "Call prepare with the same query selection and schema_cv_id/schema_run_id. It will verify that the selected managed type exists in the completed bootstrap plan schema. No state was changed."
		return result
	}
	facts, err := readImportPlanFacts(ctx, c, r.Plan.ID, input.TargetAddress)
	if err != nil {
		if importDiagnosticCode(err) == "plan_drift_limit" || importDiagnosticCode(err) == "plan_drift_evidence_invalid" {
			result.NextAction = "Compact drift evidence is incomplete. Inspect the full finished plan JSON for this plan ID before assessing the import; do not infer that drift is harmless or retry automatically."
		}
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.Execution.PlanFacts = facts
	if facts.Truncated {
		result.Diagnostics = []string{"plan_facts_truncated"}
		result.NextAction = "Compact plan facts are truncated. Inspect the full finished plan JSON before assessing this import; do not infer that omitted actions are safe or automatically retry."
		return result
	}
	result.Status, result.Stage = "plan_available_for_agent_assessment", "status"
	result.AgentInstructions = []string{"Compare the selected address, provider source, import marker and every action with the selected query identity and user intent. The compact plan facts do not include the import ID value or attribute differences.", "For any replacement or update, inspect get_plan_json_output with this plan_id for replace_paths and changed attributes; do not apply. Repair agent-owned HCL, review it and use a new speculative CV/Run.", "Drift entries are refresh-only resource_drift facts, separate from planned resource_changes; inspect the full plan JSON for their attribute differences and all output/deferred changes. Even a no-op import marker is only a speculative plan, not proof of persisted state or exactly-once execution."}
	result.NextAction = "Agent: inspect import and action facts; for replacements or updates, read get_plan_json_output(plan_id) before another reviewed CV/Run. A speculative plan made no persisted state change."
	return result
}

func readImportPlanFacts(ctx context.Context, c *tfe.Client, planID, address string) (*importPlanFacts, error) {
	raw, status, err := readImportBackendJSON(ctx, c, "plans/"+url.PathEscape(planID)+"/json-output", maxImportSchemaBytes)
	if err != nil || status != 200 {
		return nil, importReadError(err, status)
	}
	var plan struct {
		FormatVersion   string `json:"format_version"`
		ResourceChanges []struct {
			Address      string `json:"address"`
			Mode         string `json:"mode"`
			Type         string `json:"type"`
			ProviderName string `json:"provider_name"`
			Change       struct {
				Actions   []string        `json:"actions"`
				Importing json.RawMessage `json:"importing"`
			} `json:"change"`
		} `json:"resource_changes"`
		OutputChanges map[string]struct{} `json:"output_changes"`
		ResourceDrift []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_drift"`
		DeferredChanges []struct{} `json:"deferred_changes"`
	}
	if err := decodeImportEvidenceJSONLimit(raw, &plan, maxImportSchemaBytes); err != nil {
		return nil, err
	}
	if !importCompatibleFormat(plan.FormatVersion) {
		return nil, importEvidenceFailure("plan_format_unsupported")
	}
	if len(plan.ResourceChanges) > 10000 {
		return nil, importEvidenceFailure("plan_resource_limit")
	}
	if len(plan.ResourceDrift) > 100 {
		return nil, importEvidenceFailure("plan_drift_limit")
	}
	f := &importPlanFacts{FormatVersion: plan.FormatVersion, SelectedAddress: address, ResourceChangeCount: len(plan.ResourceChanges), OtherManagedActions: []importPlannedResource{}, OtherImports: []importPlannedResource{}, DriftEntries: []importResourceDrift{}, OutputChangeCount: len(plan.OutputChanges), DriftCount: len(plan.ResourceDrift), DeferredCount: len(plan.DeferredChanges)}
	for _, drift := range plan.ResourceDrift {
		if drift.Address == "" || len(drift.Change.Actions) == 0 {
			return nil, importEvidenceFailure("plan_drift_evidence_invalid")
		}
		f.DriftEntries = append(f.DriftEntries, importResourceDrift{Address: drift.Address, Actions: drift.Change.Actions})
	}
	for _, entry := range plan.ResourceChanges {
		imported := len(entry.Change.Importing) > 0 && string(entry.Change.Importing) != "null"
		var importing struct {
			ID json.RawMessage `json:"id"`
		}
		if imported && json.Unmarshal(entry.Change.Importing, &importing) != nil {
			return nil, importEvidenceFailure("plan_import_evidence_invalid")
		}
		item := importPlannedResource{Address: entry.Address, Mode: entry.Mode, Type: entry.Type, ProviderSource: entry.ProviderName, Actions: entry.Change.Actions, ImportPresent: imported, ImportIDPresent: len(importing.ID) > 0 && string(importing.ID) != "null"}
		if entry.Address == address {
			f.SelectedEntries++
			if f.SelectedEntries == 1 {
				f.Selected = &item
			}
			continue
		}
		if imported {
			f.OtherImportCount++
			if len(f.OtherImports) < 100 {
				f.OtherImports = append(f.OtherImports, item)
			} else {
				f.Truncated = true
			}
		}
		if entry.Mode == "managed" && (len(entry.Change.Actions) != 1 || entry.Change.Actions[0] != "no-op") {
			f.OtherManagedActionCount++
			if len(f.OtherManagedActions) < 100 {
				f.OtherManagedActions = append(f.OtherManagedActions, item)
			} else {
				f.Truncated = true
			}
		}
	}
	return f, nil
}

// Absence must be established by the backend, not inferred from a missing CV
// relationship alone. An inaccessible state is not an empty workspace.
func checkImportBlankBaseline(ctx context.Context, c *tfe.Client, w *tfe.Workspace) error {
	current, err := c.Workspaces.ReadByID(ctx, w.ID)
	if err != nil {
		return importReadError(err, 0)
	}
	if current.ID != w.ID || current.Organization == nil || w.Organization == nil || !strings.EqualFold(current.Organization.Name, w.Organization.Name) || importCurrentConfigurationID(current) != "" || current.ExecutionMode != "remote" || current.WorkingDirectory != "" || current.VCSRepo != nil {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	_, err = c.StateVersions.ReadCurrent(ctx, w.ID)
	if errors.Is(err, tfe.ErrResourceNotFound) {
		return nil
	}
	if err != nil {
		return importReadError(err, 0)
	}
	return importEvidenceFailure("blank_workspace_has_state")
}

// The bootstrap must be a completed, plan-only speculative run on a CV in the
// intended workspace, with no changes whatsoever. The agent, not MCP, reviews
// the intended provider-only tree: an empty plan cannot prove HCL content.
// The speculative plan is the backend schema source.
func readImportBootstrapSchemaRun(ctx context.Context, c *tfe.Client, w *tfe.Workspace, cvID, runID string) (*tfe.Run, error) {
	if !importInputName(cvID) || !importInputName(runID) {
		return nil, importEvidenceFailure("bootstrap_schema_input_invalid")
	}
	_, cv, err := readImportExecutionCV(ctx, c, importPrepareInput{Organization: w.Organization.Name, Workspace: w.Name, ConfigurationVersionID: cvID})
	if err != nil {
		return nil, err
	}
	if cv.Status != tfe.ConfigurationUploaded {
		return nil, importEvidenceFailure("bootstrap_cv_not_uploaded")
	}
	r, err := c.Runs.Read(ctx, runID)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if r.ID != runID || !r.PlanOnly || r.Workspace == nil || r.Workspace.ID != w.ID || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != cv.ID || r.Plan == nil || r.Plan.ID == "" || r.Status != tfe.RunPlannedAndFinished {
		return nil, importEvidenceFailure("bootstrap_run_not_ready_or_unverified")
	}
	p, err := c.Plans.Read(ctx, r.Plan.ID)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if p.Status != tfe.PlanFinished {
		return nil, importEvidenceFailure("bootstrap_plan_not_finished")
	}
	facts, err := readImportPlanFacts(ctx, c, r.Plan.ID, "__bootstrap_must_be_empty__")
	if err != nil {
		return nil, err
	}
	if facts.ResourceChangeCount != 0 || facts.OutputChangeCount != 0 || facts.DriftCount != 0 || facts.DeferredCount != 0 {
		return nil, importEvidenceFailure("bootstrap_plan_not_empty")
	}
	return r, nil
}

func createImportBlankCV(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	if !input.ConfirmSpeculativeRun {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "confirm_speculative_run_required", "After review, set confirm_speculative_run=true to authorize only a speculative CV create.")
	}
	if input.TargetAddress != "" && !validImportTargetAddress(input.TargetAddress) {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "target_address_invalid", "Supply a valid target_address for a direct import, or omit it only for a provider-only schema probe.")
	}
	if !input.ConfirmSpeculativeRun || input.BaselineCVID != "" || input.BaselineStateID != "" || input.BaselineSerial != 0 || input.ConfigurationVersionID != "" || input.RunID != "" || input.SchemaCVID != "" || input.SchemaRunID != "" || (input.TargetAddress != "" && !validImportTargetAddress(input.TargetAddress)) {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "blank_upload_phase_fields_invalid", "For a verified blank workspace, upload omits baseline, schema, configuration_version_id and run_id; use target_address for direct import or omit it only for a provider-only schema probe.")
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(importReadError(err, 0)))
	}
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return importExecutionFailure(result, "workspace_ownership_unverified")
	}
	result.WorkspaceID = w.ID
	if err := checkImportBlankBaseline(ctx, c, w); err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	discovery, err := readImportDiscovery(ctx, c, input.QueryID)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	if discovery.WorkspaceID != w.ID {
		return importExecutionFailure(result, "query_workspace_mismatch")
	}
	if err := checkImportBlankSelection(discovery, input); err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	if err := checkImportBlankBaseline(ctx, c, w); err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.Execution = &importExecutionResponse{Stage: "cv_create_outcome_unknown", TargetAddress: input.TargetAddress}
	mutation, err := client.NewTfeClientForImportMutation(ctx, logger)
	if err != nil {
		return importExecutionFailure(result, "backend_client_unavailable")
	}
	no, yes := false, true
	cv, err := mutation.ConfigurationVersions.Create(ctx, w.ID, tfe.ConfigurationVersionCreateOptions{AutoQueueRuns: &no, Speculative: &yes})
	if err != nil {
		result.NextAction = "CV create outcome unknown; reconcile in Atlas before another create."
		return importExecutionFailure(result, "cv_create_outcome_unknown")
	}
	result.Execution.ConfigurationVersionID = cv.ID
	if cv.ID == "" || !cv.Speculative || cv.Provisional || cv.AutoQueueRuns || !validImportArtifactLocation(ctx, cv.UploadURL) {
		return importExecutionFailure(result, "cv_create_response_invalid")
	}
	result.Status, result.Stage, result.Execution.Stage = "awaiting_agent_upload", "upload", "awaiting_agent_upload"
	result.Execution.UploadURL = cv.UploadURL
	result.Execution.UploadInstructions = "Retain this one-use URL securely until the client-local PUT of the complete reviewed archive succeeds (Content-Type: application/octet-stream). MCP does not read its contents. Poll status with this CV ID. If the URL is lost, status cannot reacquire it: reconcile the pending CV before considering a new reviewed speculative CV; do not blindly create another."
	result.NextAction = "After direct agent upload and uploaded status, call plan with this CV ID and the same query selection and target_address, without baseline/schema IDs."
	if input.TargetAddress == "" {
		result.Execution.Stage = "bootstrap_awaiting_agent_upload"
		result.Execution.UploadInstructions = "Retain this one-use URL securely until the client-local PUT of the complete reviewed provider-only archive succeeds (Content-Type: application/octet-stream). Do not include resources, imports, outputs or secrets. Poll status with this CV ID. If the URL is lost, status cannot reacquire it: reconcile the pending CV before considering a new reviewed speculative CV; do not blindly create another."
		result.NextAction = "After direct agent upload and uploaded status, call plan using this CV ID and the same query selection, without baseline IDs, schema IDs or target_address."
	}
	return result
}

func checkImportBlankSelection(discovery *importDiscovery, input importPrepareInput) error {
	for _, candidate := range discovery.Candidates {
		if candidate.CandidateID == input.Selections[0].CandidateID {
			// Without a destination schema, only permit an exact list/managed
			// type name match. An inferred cross-type mapping needs more evidence.
			if input.TargetAddress != "" && candidate.ResourceType != input.Selections[0].ManagedType {
				return importEvidenceFailure("blank_managed_type_unverified")
			}
			return nil
		}
	}
	return importEvidenceFailure("selected_candidate_not_found")
}

func createImportBlankRun(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	if !input.ConfirmSpeculativeRun {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "confirm_speculative_run_required", "After review, set confirm_speculative_run=true to authorize only a plan-only Run.")
	}
	if input.TargetAddress != "" && !validImportTargetAddress(input.TargetAddress) {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "target_address_invalid", "Supply a valid target_address for a direct import plan, or omit it only for a provider-only schema probe.")
	}
	if input.ConfigurationVersionID == "" {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "configuration_version_id_required", "Supply the uploaded speculative configuration_version_id returned by upload.")
	}
	if !input.ConfirmSpeculativeRun || input.BaselineCVID != "" || input.BaselineStateID != "" || input.BaselineSerial != 0 || input.RunID != "" || input.SchemaCVID != "" || input.SchemaRunID != "" || (input.TargetAddress != "" && !validImportTargetAddress(input.TargetAddress)) {
		return importPhaseInputFailure(result, "blank_workspace_input_invalid", "blank_plan_phase_fields_invalid", "For a verified blank workspace, plan uses the uploaded CV ID and omits baseline, schema and run_id; use target_address for direct import or omit it only for a provider-only schema probe.")
	}
	lookup := importPrepareInput{Organization: input.Organization, Workspace: input.Workspace, ConfigurationVersionID: input.ConfigurationVersionID}
	w, cv, err := readImportExecutionCV(ctx, c, lookup)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.WorkspaceID = w.ID
	result.Execution = &importExecutionResponse{ConfigurationVersionID: cv.ID, Stage: "ready_for_plan", TargetAddress: input.TargetAddress}
	if cv.Status != tfe.ConfigurationUploaded {
		return importExecutionFailure(result, "execution_cv_not_uploaded")
	}
	if err := checkImportBlankBaseline(ctx, c, w); err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	discovery, err := readImportDiscovery(ctx, c, input.QueryID)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	if discovery.WorkspaceID != w.ID {
		return importExecutionFailure(result, "query_workspace_mismatch")
	}
	if err := checkImportBlankSelection(discovery, input); err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	if err := checkImportBlankBaseline(ctx, c, w); err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.Execution.Stage = "run_create_outcome_unknown"
	mutation, err := client.NewTfeClientForImportMutation(ctx, logger)
	if err != nil {
		return importExecutionFailure(result, "backend_client_unavailable")
	}
	message := "Speculative Search import inspection"
	if input.TargetAddress == "" {
		message = "Speculative Search provider-schema bootstrap"
	}
	r, err := mutation.Runs.Create(ctx, tfe.RunCreateOptions{Workspace: w, ConfigurationVersion: cv, PlanOnly: tfe.Bool(true), AutoApply: tfe.Bool(false), AllowConfigGeneration: tfe.Bool(false), Message: &message})
	if err != nil {
		result.NextAction = "Run create outcome unknown; reconcile Atlas before another create."
		return importExecutionFailure(result, "run_create_outcome_unknown")
	}
	result.Execution.RunID = r.ID
	if r.ID == "" || !r.PlanOnly || r.Workspace == nil || r.Workspace.ID != w.ID || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != cv.ID {
		return importExecutionFailure(result, "run_association_unverified")
	}
	result.Status, result.Stage, result.Execution.Stage = "pending", "plan", "run_created"
	result.NextAction = "Poll status with CV, Run and target_address; inspect import and change facts. The speculative Run did not change persisted state."
	if input.TargetAddress == "" {
		result.Execution.Stage = "bootstrap_run_created"
		result.NextAction = "Poll status with bootstrap CV and Run IDs (no target_address); then call prepare with schema_cv_id and schema_run_id. No state was applied."
	}
	return result
}
