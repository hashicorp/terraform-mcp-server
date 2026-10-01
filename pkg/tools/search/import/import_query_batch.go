// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
)

// A QueryRun candidate ID is not a Terraform import ID. Re-read the complete,
// finished query for every batch operation; caller-carried selections do not
// authenticate an uploaded archive or a previous tool call.
func readImportBatchCandidates(ctx context.Context, c *tfe.Client, input importPrepareInput, workspaceID string) ([]importDiscoveryCandidate, error) {
	discovery, err := readImportDiscovery(ctx, c, input.QueryID)
	if err != nil {
		return nil, err
	}
	if discovery.WorkspaceID != workspaceID {
		return nil, importEvidenceFailure("query_workspace_mismatch")
	}
	byID := make(map[string]importDiscoveryCandidate, len(discovery.Candidates))
	for _, candidate := range discovery.Candidates {
		byID[candidate.CandidateID] = candidate
	}
	selected := make([]importDiscoveryCandidate, 0, len(input.Selections))
	seenIdentity := make(map[string]bool, len(input.Selections))
	for _, selection := range input.Selections {
		candidate, ok := byID[selection.CandidateID]
		if !ok {
			return nil, importEvidenceFailure("selected_candidate_not_found")
		}
		identity, err := json.Marshal(candidate.Identity)
		if err != nil {
			return nil, importEvidenceFailure("query_identity_invalid")
		}
		identityKey := candidate.Provider.Source + "/" + candidate.Provider.Version + "/" + string(identity)
		if seenIdentity[identityKey] {
			return nil, importEvidenceFailure("selected_identity_overlap")
		}
		seenIdentity[identityKey] = true
		selected = append(selected, candidate)
	}
	return selected, nil
}

func firstImportSelection(input importPrepareInput) importPrepareInput {
	first := input
	first.Selections = []importSelection{{CandidateID: input.Selections[0].CandidateID, ManagedType: input.Selections[0].ManagedType}}
	first.TargetAddress = input.Selections[0].TargetAddress
	return first
}

func prepareImportBatchFromAPIs(ctx context.Context, c *tfe.Client, input importPrepareInput) importPreparation {
	first := firstImportSelection(input)
	first.TargetAddress = ""
	result := prepareImportFromAPIs(ctx, c, first)
	if result.Status != "prepared" && result.Status != "ready_for_authoring" {
		return result
	}
	candidates, err := readImportBatchCandidates(ctx, c, input, result.WorkspaceID)
	if err != nil {
		result.Status = "blocked"
		result.NextAction = "Resolve the selected QueryRun/candidate evidence diagnostic before a speculative create. No CV or Run was created by this request."
		return importExecutionFailure(result, importDiagnosticCode(err))
	}
	// Read each distinct selected managed type against the same state-associated
	// plan artifact. Return its full schema via a focused one-selection prepare;
	// repeating it 100 times in this packet would exceed the response budget.
	if result.SchemaSource != nil {
		checked := map[string]bool{result.SchemaSource.ProviderSource + "/" + first.Selections[0].ManagedType: true}
		for index, candidate := range candidates {
			key := candidate.Provider.Source + "/" + input.Selections[index].ManagedType
			if checked[key] {
				continue
			}
			if _, _, _, err := readImportManagedSchema(ctx, c, result.SchemaSource.RunID, candidate.Provider.Source, input.Selections[index].ManagedType); err != nil {
				result.Status = "blocked"
				result.NextAction = "Resolve the destination managed-type schema diagnostic for the selected candidate before a speculative create. No CV or Run was created by this request."
				return importExecutionFailure(result, importDiagnosticCode(err))
			}
			checked[key] = true
		}
	}
	result.SelectedCandidates = candidates
	result.Selection, result.ManagedSchema, result.IdentitySchema = nil, nil, nil
	result.ManagedType, result.ManagedTypeSupport = "", ""
	result.Notes = append(result.Notes, "batch_selected_types_checked_where_destination_schema_available; call prepare for one candidate when its complete managed schema is needed; no HCL or archive was inspected")
	result.AgentInstructions = append(result.AgentInstructions, "For each selected candidate, review its exact observed identity and author one resource instance and one individual import block. Record candidate_id -> target_address -> documented destination import ID/identity and provider scope. Keep this complete binding list for plan and post-Run status; a candidate ID never appears in Terraform plan JSON.")
	result.NextAction = "Review all selected candidates and their destination managed schemas as needed. Author one individual resource/import binding per candidate in the complete preserved tree; obtain review before speculative upload. Up to 100 explicit selections are supported, not the first N results."
	if raw, err := json.Marshal(result); err != nil || len(raw) > maxImportPreparationBytes {
		result.Status = "blocked"
		result.Diagnostics = append(result.Diagnostics, "evidence_response_limit")
		result.SelectedCandidates = nil
		result.NextAction = "The selected candidate evidence exceeds the bounded response. Narrow the reviewed batch; no CV or Run was created. Do not treat a partial batch as prepared."
	}
	return result
}

func createImportBatchCV(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	if !input.ConfirmSpeculativeRun {
		return importPhaseInputFailure(result, "speculative_upload_input_invalid", "confirm_speculative_run_required", "Confirm only a reviewed speculative CV create.")
	}
	if input.ConfigurationVersionID != "" || input.RunID != "" {
		return importPhaseInputFailure(result, "speculative_upload_input_invalid", "upload_phase_fields_invalid", "Batch upload omits CV and Run IDs.")
	}
	prepared := prepareImportBatchFromAPIs(ctx, c, input)
	if prepared.Status != "prepared" && prepared.Status != "ready_for_authoring" {
		return prepared
	}
	first := firstImportSelection(input)
	if input.BaselineCVID != "" || input.SchemaCVID != "" {
		first.TargetAddress = "" // existing/bootstrap upload has no scalar target
	}
	result = createImportSpeculativeCV(ctx, c, first, logger)
	if result.Status == "awaiting_agent_upload" {
		result.NextAction += " Preserve all reviewed candidate/address bindings for the later plan and status calls; MCP has not inspected the archive."
	}
	return result
}

func createImportBatchRun(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importExecutionResult(input)
	if !input.ConfirmSpeculativeRun || input.ConfigurationVersionID == "" || input.RunID != "" {
		return importPhaseInputFailure(result, "plan_input_invalid", "plan_phase_fields_invalid", "Batch plan needs the uploaded CV ID, every reviewed candidate/address binding and explicit speculative confirmation; omit run_id.")
	}
	prepared := prepareImportBatchFromAPIs(ctx, c, input)
	if prepared.Status != "prepared" && prepared.Status != "ready_for_authoring" {
		return prepared
	}
	result = createImportPlanRun(ctx, c, firstImportSelection(input), logger)
	if result.Execution != nil {
		result.Execution.TargetAddress = "" // a batch does not have one representative target
	}
	if result.Status == "pending" {
		result.NextAction = "Poll status with this CV/Run pair, query_run_id and the complete reviewed selections including each target_address. These caller-carried bindings do not authenticate the uploaded HCL; no import has been applied."
	}
	return result
}

func addImportBatchContinuation(input importPrepareInput, response *importPreparation) {
	refs := &importWorkflowContext{Organization: input.Organization, Workspace: input.Workspace, WorkspaceID: response.WorkspaceID, QueryRunID: input.QueryID, Selections: append([]importSelection(nil), input.Selections...)}
	if response.Baseline != nil && input.Phase == "prepare" {
		refs.BaselineCVID, refs.BaselineStateID = response.Baseline.ConfigurationVersionID, response.Baseline.StateVersionID
		if response.Baseline.StateSerial != nil {
			refs.BaselineSerial = *response.Baseline.StateSerial
		}
	} else {
		refs.BaselineCVID, refs.BaselineStateID, refs.BaselineSerial = input.BaselineCVID, input.BaselineStateID, input.BaselineSerial
	}
	refs.SchemaCVID, refs.SchemaRunID = input.SchemaCVID, input.SchemaRunID
	if response.Execution != nil {
		refs.ConfigurationVersionID, refs.RunID, refs.PlanID = response.Execution.ConfigurationVersionID, response.Execution.RunID, response.Execution.PlanID
	}
	response.WorkflowContext = refs
	args := map[string]any{"organization_name": refs.Organization, "workspace_name": refs.Workspace}
	switch {
	case input.Phase == "prepare" && (response.Status == "prepared" || response.Status == "ready_for_authoring"):
		args["phase"], args["query_run_id"], args["selections"] = "upload", refs.QueryRunID, refs.Selections
		if refs.BaselineCVID != "" {
			args["baseline_cv_id"], args["baseline_state_id"], args["baseline_state_serial"] = refs.BaselineCVID, refs.BaselineStateID, refs.BaselineSerial
		}
		if refs.SchemaCVID != "" {
			args["schema_cv_id"], args["schema_run_id"] = refs.SchemaCVID, refs.SchemaRunID
		}
		response.Continuation = &importContinuation{NextPhase: "upload", Arguments: args, RequiredInputs: []string{"confirm_speculative_run", "selections[].target_address"}, Precondition: "Before upload, review one individual resource/import block and exact target_address for each selected candidate, plus the entire archive and lock. Confirm only a speculative CV create. For an existing workspace retain the verified baseline; do not substitute the Search provider release."}
	case input.Phase == "upload" && response.Status == "awaiting_agent_upload":
		args["phase"], args["configuration_version_id"] = "status", refs.ConfigurationVersionID
		response.Continuation = &importContinuation{NextPhase: "status", Arguments: args, Precondition: "Retain the one-use upload URL securely until the client PUT is confirmed. Status cannot reacquire it. Keep the reviewed query/selection/address bindings for the later plan; status by CV alone cannot reconstruct them."}
	case input.Phase == "plan" && response.Status == "pending" || input.Phase == "status" && response.Status == "pending":
		args["phase"], args["configuration_version_id"], args["run_id"] = "status", refs.ConfigurationVersionID, refs.RunID
		args["query_run_id"], args["selections"] = refs.QueryRunID, refs.Selections
		response.Continuation = &importContinuation{NextPhase: "status", Arguments: args, Precondition: "Poll the exact plan-only CV/Run pair with the same caller-carried batch bindings. No import was applied; do not create another Run merely because this one is pending."}
	}
}
