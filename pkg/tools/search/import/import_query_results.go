// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
)

type workspaceProvider struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

const importPreparationContractVersion = importToolContractVersion
const maxImportPreparationBytes = 256 * 1024
const maxImportSelections = 100

type importSelection struct {
	CandidateID string `json:"candidate_id"`
	ManagedType string `json:"managed_type"`
}

// importPrepareInput carries the shared preparation inputs. The configuration
// version ID is internal to the CV membership check and is never decoded from
// a prepare_import call.
type importPrepareInput struct {
	Organization           string            `json:"organization_name"`
	Workspace              string            `json:"workspace_name"`
	QueryID                string            `json:"query_run_id"`
	Selections             []importSelection `json:"selections"`
	ConfigurationVersionID string            `json:"-"`

	// skipSchemaDownload resolves the baseline and schema source only. The
	// caller then downloads the schema artifact once for all selected types
	// (ADR 0007).
	skipSchemaDownload bool
}

type importPreparation struct {
	ContractVersion        string                      `json:"contract_version"`
	Status                 string                      `json:"status"`
	Stage                  string                      `json:"stage"`
	Organization           string                      `json:"organization_name,omitempty"`
	WorkspaceID            string                      `json:"workspace_id,omitempty"`
	QueryRunID             string                      `json:"query_run_id,omitempty"`
	ExecutionMode          string                      `json:"execution_mode,omitempty"`
	Baseline               *importAPIBaseline          `json:"baseline,omitempty"`
	Selection              *importDiscoveryCandidate   `json:"selection,omitempty"`
	SelectedCandidates     []importDiscoveryCandidate  `json:"selected_candidates,omitempty"`
	SchemaSource           *importAPISchemaSource      `json:"schema_source,omitempty"`
	ManagedType            string                      `json:"managed_type,omitempty"`
	ManagedTypeSupport     string                      `json:"managed_type_support,omitempty"`
	ManagedSchema          map[string]any              `json:"managed_schema,omitempty"`
	IdentitySchema         map[string]any              `json:"identity_schema,omitempty"`
	EvidenceStatus         string                      `json:"evidence_status,omitempty"`
	SourceSchemaComparison string                      `json:"source_schema_comparison,omitempty"`
	ValidationStatus       string                      `json:"validation_status,omitempty"`
	Notes                  []string                    `json:"notes,omitempty"`
	AgentInstructions      []string                    `json:"agent_instructions,omitempty"`
	Diagnostics            []string                    `json:"diagnostics"`
	NextAction             string                      `json:"next_action"`
	Context                *importConfigurationHandoff `json:"configuration_context,omitempty"`
}

type importAPIBaseline struct {
	ConfigurationVersionID string `json:"configuration_version_id,omitempty"`
	StateVersionID         string `json:"state_version_relationship_id,omitempty"`
	StateSerial            *int64 `json:"state_serial,omitempty"`
	WorkingDirectory       string `json:"working_directory"`
}

type importAPISchemaSource struct {
	ConfigurationVersionID        string `json:"configuration_version_id,omitempty"`
	ConfigurationBaselineRelation string `json:"configuration_baseline_relation"`
	StateVersionID                string `json:"state_version_id"`
	RunID                         string `json:"run_id"`
	PlanID                        string `json:"plan_id"`
	ProviderSource                string `json:"provider_source"`
	TerraformVersion              string `json:"terraform_version,omitempty"`
	ArtifactDigest                string `json:"artifact_digest,omitempty"`
}

func importInputName(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\\x00\r\n") && value != "." && value != ".."
}

func validImportSelections(input importPrepareInput) bool {
	seen := map[string]bool{}
	for _, s := range input.Selections {
		if !strings.HasPrefix(s.CandidateID, "candidate-") || len(s.CandidateID) != 74 || !importInputName(s.ManagedType) || seen[s.CandidateID] {
			return false
		}
		seen[s.CandidateID] = true
	}
	return true
}

// prepareImportWithDiscovery lets a caller that already read the QueryRun log
// pass that discovery in, so the log is parsed once per request.
func prepareImportWithDiscovery(ctx context.Context, c *tfe.Client, input importPrepareInput, preRead *importDiscovery) importPreparation {
	result := importPreparation{ContractVersion: importPreparationContractVersion, Status: "blocked", Stage: "workspace", Organization: input.Organization, QueryRunID: input.QueryID, SourceSchemaComparison: "not_requested", Diagnostics: []string{}, NextAction: "Resolve the reported evidence gap, then repeat preparation."}
	fail := func(err error) importPreparation {
		result.Diagnostics = append(result.Diagnostics, importDiagnosticCode(err))
		return result
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	result.WorkspaceID = w.ID
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return fail(importEvidenceFailure("workspace_ownership_unverified"))
	}
	if err := client.AuthorizeOrganization(ctx, w.Organization.Name); err != nil {
		return fail(importEvidenceFailure("organization_not_allowed"))
	}
	result.ExecutionMode = w.ExecutionMode
	result.Baseline = &importAPIBaseline{ConfigurationVersionID: importCurrentConfigurationID(w), WorkingDirectory: w.WorkingDirectory}
	if result.Baseline.ConfigurationVersionID == "" {
		result.Notes = append(result.Notes, "configuration_baseline_unavailable")
	}
	result.Stage = "query_selection"
	discovery := preRead
	if discovery == nil {
		var err error
		if discovery, err = readImportDiscovery(ctx, c, input.QueryID); err != nil {
			return fail(err)
		}
	}
	if discovery.WorkspaceID != w.ID {
		return fail(importEvidenceFailure("query_workspace_mismatch"))
	}
	for _, candidate := range discovery.Candidates {
		if candidate.CandidateID == input.Selections[0].CandidateID {
			selected := candidate
			result.Selection = &selected
		}
	}
	if result.Selection == nil {
		return fail(importEvidenceFailure("selected_candidate_not_found"))
	}
	result.ManagedType = input.Selections[0].ManagedType
	result.ManagedTypeSupport = "unknown"
	result.Stage = "current_state"
	sv, stateErr := readImportCurrentState(ctx, c, w.ID)
	if result.Baseline.ConfigurationVersionID == "" && importDiagnosticCode(stateErr) == "current_state_unavailable_or_inaccessible" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return fail(err)
		}
		result.Status, result.Stage = "ready_for_authoring", "blank_workspace"
		result.ValidationStatus = "plan_validation_pending"
		result.EvidenceStatus = "selected_query_candidate_only; managed_schema_not_verified; configuration_not_validated"
		result.Notes = append(result.Notes, "No destination provider schema exists in this empty workspace. The agent must validate its proposed provider configuration, resource and import blocks locally; the speculative plan establishes runtime facts.")
		result.NextAction = "Author and review the complete resource/import configuration and lock locally, then call create_import_cv."
		return result
	}
	if stateErr != nil {
		return fail(stateErr)
	}
	result.Baseline.StateVersionID = sv.ID
	result.Baseline.StateSerial = &sv.Serial
	result.Stage = "schema_source"
	r, err := readImportSchemaRun(ctx, c, w.ID, sv)
	if err != nil {
		return fail(err)
	}
	result.SchemaSource = &importAPISchemaSource{StateVersionID: sv.ID, RunID: r.ID, PlanID: r.Plan.ID, ProviderSource: result.Selection.Provider.Source, TerraformVersion: r.TerraformVersion, ConfigurationBaselineRelation: "unknown"}
	if r.ConfigurationVersion != nil {
		result.SchemaSource.ConfigurationVersionID = r.ConfigurationVersion.ID
		if r.ConfigurationVersion.ID != "" && result.Baseline.ConfigurationVersionID != "" {
			result.SchemaSource.ConfigurationBaselineRelation = "same_configuration_version"
			if r.ConfigurationVersion.ID != result.Baseline.ConfigurationVersionID {
				result.SchemaSource.ConfigurationBaselineRelation = "different_configuration_version"
				result.Notes = append(result.Notes, "schema_source_uses_different_configuration_version")
			}
		}
	}
	if !input.skipSchemaDownload {
		result.Stage = "managed_schema"
		managed, identity, digest, err := readImportManagedSchema(ctx, c, r.ID, result.SchemaSource.ProviderSource, result.ManagedType)
		if err != nil {
			if importDiagnosticCode(err) == "managed_schema_type_missing" {
				result.ManagedTypeSupport = "unsupported"
			}
			return fail(err)
		}
		if err := decodeImportEvidenceJSONLimit(managed, &result.ManagedSchema, maxImportSchemaBytes); err != nil {
			return fail(err)
		}
		if len(identity) > 0 {
			if err := decodeImportEvidenceJSON(identity, &result.IdentitySchema); err != nil {
				return fail(err)
			}
		}
		result.SchemaSource.ArtifactDigest = digest
		result.ManagedTypeSupport = "supported"
		result.EvidenceStatus = "destination_schema_acquired; resource_type_checked; configuration_not_validated"
	}
	result.Stage = "baseline_recheck"
	current, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	currentState, err := readImportCurrentState(ctx, c, w.ID)
	if err != nil {
		return fail(err)
	}
	if current.ID != w.ID || importCurrentConfigurationID(current) != result.Baseline.ConfigurationVersionID || current.WorkingDirectory != w.WorkingDirectory || currentState.ID != sv.ID || currentState.Serial != sv.Serial || currentState.Run == nil || currentState.Run.ID != r.ID {
		return fail(importEvidenceFailure("baseline_changed"))
	}
	result.Status = "prepared"
	result.Stage = "ready_for_authoring"
	result.ValidationStatus = "plan_validation_pending"
	result.NextAction = "Before requesting the short-lived phase=context URL, ask the user to choose an isolated client-local temporary directory or a user-approved directory in the agent's workspace. Then preserve the current full tree/lock, author or adapt resource/import HCL locally using selected schema and query evidence, resolve missing inputs and provider wiring, review changes with the user, and request a speculative CV through upload."
	return result
}

func importCurrentConfigurationID(w *tfe.Workspace) string {
	if w.CurrentConfigurationVersion != nil {
		return w.CurrentConfigurationVersion.ID
	}
	return ""
}
