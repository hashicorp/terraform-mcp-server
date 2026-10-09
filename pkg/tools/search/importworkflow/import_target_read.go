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

const maxImportPreparationBytes = 256 * 1024
const maxImportSelections = 100

// A candidate ID is "candidate-" followed by a SHA-256 digest in hex.
const (
	importCandidateIDPrefix = "candidate-"
	importCandidateIDLength = len(importCandidateIDPrefix) + 64
)

// Bounds on tool arguments, so an oversized call is refused before decoding.
const (
	maxPrepareArgumentBytes  = 64 * 1024
	maxDownloadArgumentBytes = 4 * 1024
	maxCreateArgumentBytes   = 8 * 1024
	maxVerifyArgumentBytes   = 512 * 1024
)

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
	PreparedTargetID       string            `json:"-"`
}

// importTargetRead is what the shared target read learns about the target
// workspace: its baseline, the run that produced the current schema, and notes.
// Status is blocked, prepared or ready_for_authoring.
type importTargetRead struct {
	Status        string
	Stage         string
	WorkspaceID   string
	ExecutionMode string
	Baseline      *importAPIBaseline
	SchemaSource  *importAPISchemaSource
	Notes         []string
	Diagnostics   []string
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
		if !strings.HasPrefix(s.CandidateID, importCandidateIDPrefix) || len(s.CandidateID) != importCandidateIDLength || !importInputName(s.ManagedType) || seen[s.CandidateID] {
			return false
		}
		seen[s.CandidateID] = true
	}
	return true
}

// readImportTarget reads the target workspace's baseline and schema source for
// an already parsed QueryRun log, so the log is parsed once per request. It
// resolves the baseline and schema source only; prepareImportTool downloads the
// schema artifact once for all selected types (ADR 0007). With agentSchema set
// (the schema artifact is missing, ADR 0008) it only re-checks the baseline.
func readImportTarget(ctx context.Context, c *tfe.Client, input importPrepareInput, discovery *importDiscovery, targetID string, agentSchema bool) importTargetRead {
	result := importTargetRead{Status: "blocked", Stage: "workspace", Diagnostics: []string{}}
	fail := func(err error) importTargetRead {
		result.Diagnostics = append(result.Diagnostics, importDiagnosticCode(err))
		return result
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	result.WorkspaceID = w.ID
	if w.ID != targetID {
		return fail(importEvidenceFailure("prepared_target_workspace_mismatch"))
	}
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
	selected, found := findImportCandidate(discovery, input.Selections[0].CandidateID)
	if !found {
		return fail(importEvidenceFailure("selected_candidate_not_found"))
	}
	result.Stage = "current_state"
	if w.ExecutionMode != "remote" || w.VCSRepo != nil || w.WorkingDirectory != "" {
		return fail(importEvidenceFailure("execution_source_not_supported"))
	}
	sv, stateErr := readImportCurrentState(ctx, c, w.ID)
	if result.Baseline.ConfigurationVersionID == "" && importDiagnosticCode(stateErr) == "current_state_unavailable_or_inaccessible" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return fail(err)
		}
		result.Status, result.Stage = "ready_for_authoring", "blank_workspace"
		result.Notes = append(result.Notes, "No target provider schema exists in this empty workspace. The agent must validate its proposed provider configuration, resource and import blocks locally; the speculative plan establishes runtime facts.")
		return result
	}
	if stateErr != nil {
		return fail(stateErr)
	}
	result.Baseline.StateVersionID = sv.ID
	result.Baseline.StateSerial = &sv.Serial
	if agentSchema {
		// The agent obtains the schema (ADR 0008); only re-check the baseline.
		if err := recheckImportBaseline(ctx, c, input, w, sv, ""); err != nil {
			return fail(err)
		}
		result.Status, result.Stage = "ready_for_authoring", "agent_schema"
		return result
	}
	result.Stage = "schema_source"
	r, err := readImportSchemaRun(ctx, c, w.ID, sv)
	if err != nil {
		return fail(err)
	}
	result.SchemaSource = &importAPISchemaSource{StateVersionID: sv.ID, RunID: r.ID, PlanID: r.Plan.ID, ProviderSource: selected.Provider.Source, TerraformVersion: r.TerraformVersion, ConfigurationBaselineRelation: "unknown"}
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
	result.Stage = "baseline_recheck"
	if err := recheckImportBaseline(ctx, c, input, w, sv, r.ID); err != nil {
		return fail(err)
	}
	result.Status = "prepared"
	result.Stage = "ready_for_authoring"
	return result
}

func importCurrentConfigurationID(w *tfe.Workspace) string {
	if w.CurrentConfigurationVersion != nil {
		return w.CurrentConfigurationVersion.ID
	}
	return ""
}

func findImportCandidate(d *importDiscovery, id string) (importDiscoveryCandidate, bool) {
	for _, candidate := range d.Candidates {
		if candidate.CandidateID == id {
			return candidate, true
		}
	}
	return importDiscoveryCandidate{}, false
}

// recheckImportBaseline fails with baseline_changed when the workspace, its
// current configuration, its working directory or its current state changed
// since the first read. A non-empty schemaRunID must still be the run that
// produced the current state.
func recheckImportBaseline(ctx context.Context, c *tfe.Client, input importPrepareInput, w *tfe.Workspace, sv *tfe.StateVersion, schemaRunID string) error {
	current, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return importReadError(err, 0)
	}
	currentState, err := readImportCurrentState(ctx, c, w.ID)
	if err != nil {
		return err
	}
	changed := current.ID != w.ID ||
		current.Organization == nil || !strings.EqualFold(current.Organization.Name, input.Organization) ||
		current.ExecutionMode != "remote" || current.VCSRepo != nil ||
		importCurrentConfigurationID(current) != importCurrentConfigurationID(w) ||
		current.WorkingDirectory != w.WorkingDirectory ||
		currentState.ID != sv.ID || currentState.Serial != sv.Serial
	if schemaRunID != "" {
		changed = changed || currentState.Run == nil || currentState.Run.ID != schemaRunID
	}
	if changed {
		return importEvidenceFailure("baseline_changed")
	}
	return nil
}
