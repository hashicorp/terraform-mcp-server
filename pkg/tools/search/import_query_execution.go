// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
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

type importPlanFacts struct {
	FormatVersion           string                  `json:"format_version"`
	Selected                *importPlannedResource  `json:"selected,omitempty"`
	SelectedAddress         string                  `json:"selected_address"`
	SelectedEntries         int                     `json:"selected_entries"`
	OtherManagedActions     []importPlannedResource `json:"other_managed_actions"`
	OtherImports            []importPlannedResource `json:"other_imports"`
	OtherManagedActionCount int                     `json:"other_managed_action_count"`
	OtherImportCount        int                     `json:"other_import_count"`
	OutputChangeCount       int                     `json:"output_change_count"`
	DriftCount              int                     `json:"drift_count"`
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

func createImportSpeculativeCV(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	if !input.ConfirmSpeculativeRun || !importInputName(input.BaselineCVID) || !importInputName(input.BaselineStateID) || input.ConfigurationVersionID != "" || input.RunID != "" || input.TargetAddress != "" {
		return importExecutionFailure(result, "speculative_upload_input_invalid")
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
	if prepared.Baseline.ConfigurationVersionID != input.BaselineCVID || prepared.Baseline.StateVersionID != input.BaselineStateID || prepared.Baseline.StateSerial != input.BaselineSerial {
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
	result.Execution.UploadInstructions = "PUT the complete agent-owned .tar.gz to upload_url with Content-Type: application/octet-stream; keep the URL secret. Poll status using the workspace and configuration_version_id. Never upload individual .tf files or use this CV for apply."
	result.NextAction = "Agent uploads the complete archive directly; MCP has not read configuration or created a run."
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
	if !input.ConfirmSpeculativeRun || input.RunID != "" || !validImportTargetAddress(input.TargetAddress) || !importInputName(input.BaselineCVID) || !importInputName(input.BaselineStateID) {
		return importExecutionFailure(result, "plan_input_invalid")
	}
	prepared := prepareImportFromAPIs(ctx, c, input)
	if prepared.Status != "prepared" {
		return prepared
	}
	if prepared.Baseline.ConfigurationVersionID != input.BaselineCVID || prepared.Baseline.StateVersionID != input.BaselineStateID || prepared.Baseline.StateSerial != input.BaselineSerial {
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
	sv, err := readImportCurrentState(ctx, c, w.ID)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	if sv.ID != input.BaselineStateID || sv.Serial != input.BaselineSerial {
		return importExecutionFailure(result, "baseline_changed")
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
	if input.ConfirmSpeculativeRun || (input.RunID != "" && !validImportTargetAddress(input.TargetAddress)) || (input.RunID == "" && input.TargetAddress != "") || (input.RunID != "" && !importInputName(input.RunID)) {
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
			result.NextAction = "Use the original upload URL if still valid; otherwise reconcile the CV in Atlas. Poll by configuration_version_id after agent upload. No Run ID was supplied; this does not establish that a Run does not exist."
			return result
		}
		result.Status, result.Stage = "ready_for_plan", "status"
		result.NextAction = "CV is uploaded. If no prior Run create had an uncertain outcome, call plan with this CV ID, query selection, baseline markers, address and confirmation. Otherwise reconcile existing Runs in Atlas first."
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
		result.NextAction = "Use existing run/plan/log tools to inspect sanitized diagnostics; repair locally and create a new speculative attempt."
		return result
	}
	if p.Status != tfe.PlanFinished || r.Status != tfe.RunPlannedAndFinished {
		result.Status, result.Stage = "pending", "status"
		result.NextAction = "Poll this exact Run until the plan and required run stages finish."
		return result
	}
	facts, err := readImportPlanFacts(ctx, c, r.Plan.ID, input.TargetAddress)
	if err != nil {
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	result.Execution.PlanFacts = facts
	result.Status, result.Stage = "plan_available_for_agent_assessment", "status"
	result.NextAction = "Agent: compare selected address/import evidence and all other actions with query identity, provider scope and user intent. This factual projection is not an import-verification verdict; the speculative run made no state change."
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
		OutputChanges   map[string]struct{} `json:"output_changes"`
		ResourceDrift   []struct{}          `json:"resource_drift"`
		DeferredChanges []struct{}          `json:"deferred_changes"`
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
	f := &importPlanFacts{FormatVersion: plan.FormatVersion, SelectedAddress: address, OtherManagedActions: []importPlannedResource{}, OtherImports: []importPlannedResource{}, OutputChangeCount: len(plan.OutputChanges), DriftCount: len(plan.ResourceDrift), DeferredCount: len(plan.DeferredChanges)}
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
