// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
)

// Keep enough headroom below the mark3labs HTTP server's 30-second
// WriteTimeout to serialize and write a pending response.
const verifyImportWait = 20 * time.Second
const verifyImportInterval = 2 * time.Second

type verifyImportPollConfig struct {
	wait     time.Duration
	interval time.Duration
}

func defaultVerifyImportPollConfig() verifyImportPollConfig {
	return verifyImportPollConfig{wait: verifyImportWait, interval: verifyImportInterval}
}

var importAddressPattern = regexp.MustCompile(`^[a-zA-Z0-9_\[\]".:-]+$`)

type ImportBinding struct {
	QueryResultMarker string `json:"query_result_marker" jsonschema:"Selected QueryRun result marker"`
	TargetAddress     string `json:"target_address" jsonschema:"Exact Terraform target resource address"`
}

type VerifyImportInput struct {
	OrganizationName string          `json:"organization_name" jsonschema:"Confirmed target organization name"`
	WorkspaceName    string          `json:"workspace_name" jsonschema:"Confirmed target workspace name"`
	RunID            string          `json:"run_id" jsonschema:"Exact CLI-originated plan-only HCP Terraform Run ID"`
	ImportContext    *ImportContext  `json:"import_context,omitempty" jsonschema:"Unchanged context returned by prepare_import; required for finished Plan classification"`
	Bindings         []ImportBinding `json:"bindings,omitempty" jsonschema:"One query result marker to exact target address binding per selection"`
	DetailAddresses  []string        `json:"detail_addresses,omitempty" jsonschema:"Optional selected target addresses to inspect without values"`
}

type ImportPlanFacts struct {
	TerraformVersion    string `json:"terraform_version"`
	FormatVersion       string `json:"format_version"`
	ResourceChanges     int    `json:"resource_changes"`
	OutputChanges       int    `json:"output_changes"`
	DeferredChanges     int    `json:"deferred_changes"`
	ResourceDrift       int    `json:"resource_drift"`
	ExtraImports        int    `json:"extra_imports"`
	OtherManagedActions int    `json:"other_managed_actions"`
}

type ImportPlanAttention struct {
	QueryResultMarker     string  `json:"query_result_marker"`
	TargetAddress         string  `json:"target_address"`
	Change                string  `json:"change"`
	ObjectIdentity        string  `json:"object_identity"`
	IdentityReason        string  `json:"identity_reason,omitempty"`
	SearchIdentityVersion int     `json:"search_identity_version"`
	PlanIdentityVersion   *uint64 `json:"plan_identity_version,omitempty"`
}

type ImportPlanDetail struct {
	TargetAddress        string   `json:"target_address"`
	Actions              []string `json:"actions"`
	ImportIDPresent      bool     `json:"import_id_present"`
	AfterIdentityPresent bool     `json:"after_identity_present"`
	ReplacePathCount     int      `json:"replace_path_count"`
}

type ImportUnselected struct {
	ExtraImports        int                       `json:"extra_imports"`
	OtherManagedActions int                       `json:"other_managed_actions"`
	Drift               int                       `json:"drift"`
	Addresses           []ImportUnselectedAddress `json:"addresses"`
	Truncated           bool                      `json:"truncated"`
}

type ImportUnselectedAddress struct {
	Address string `json:"address"`
	Kind    string `json:"kind"`
}

type VerifyImportResult struct {
	Status                 string                `json:"status"`
	RequestedRunID         string                `json:"requested_run_id"`
	OrganizationName       string                `json:"organization_name,omitempty"`
	WorkspaceName          string                `json:"workspace_name,omitempty"`
	WorkspaceID            string                `json:"workspace_id,omitempty"`
	RunID                  string                `json:"run_id,omitempty"`
	RunUIURL               string                `json:"run_ui_url,omitempty"`
	PlanID                 string                `json:"plan_id,omitempty"`
	ConfigurationVersionID string                `json:"configuration_version_id,omitempty"`
	RunStatus              string                `json:"run_status,omitempty"`
	PlanStatus             string                `json:"plan_status,omitempty"`
	PlanOnly               *bool                 `json:"plan_only,omitempty"`
	AutoApply              *bool                 `json:"auto_apply,omitempty"`
	RetryAfterSeconds      int                   `json:"retry_after_seconds,omitempty"`
	Diagnostics            []string              `json:"diagnostics"`
	NextAction             string                `json:"next_action"`
	Plan                   *ImportPlanFacts      `json:"plan,omitempty"`
	Changes                map[string]int        `json:"changes,omitempty"`
	ObjectIdentity         map[string]int        `json:"object_identity,omitempty"`
	Attention              []ImportPlanAttention `json:"attention,omitempty"`
	Unselected             *ImportUnselected     `json:"unselected,omitempty"`
	Detail                 []ImportPlanDetail    `json:"detail,omitempty"`
}

type importRunEvidence struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			Status    string `json:"status"`
			PlanOnly  *bool  `json:"plan-only"`
			AutoApply *bool  `json:"auto-apply"`
		} `json:"attributes"`
		Relationships struct {
			Workspace            queryWireRelation `json:"workspace"`
			Plan                 queryWireRelation `json:"plan"`
			ConfigurationVersion queryWireRelation `json:"configuration-version"`
		} `json:"relationships"`
	} `json:"data"`
}

// Only construct a browser navigation link for known HCP hosts. API addresses
// for private/Enterprise deployments are not necessarily browser addresses.
// Never reflect an API URL's userinfo, query, fragment, or arbitrary host.
func importRunUIURL(base url.URL, org, workspace, runID string) string {
	if base.Scheme != "https" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Port() != "" ||
		(base.Hostname() != "app.terraform.io" && base.Hostname() != "app.staging.terraform.io") ||
		strings.TrimSpace(org) == "" || strings.TrimSpace(workspace) == "" || strings.TrimSpace(runID) == "" {
		return ""
	}
	path := "/app/" + org + "/workspaces/" + workspace + "/runs/" + runID
	rawPath := "/app/" + url.PathEscape(org) + "/workspaces/" + url.PathEscape(workspace) + "/runs/" + url.PathEscape(runID)
	return (&url.URL{Scheme: "https", Host: base.Hostname(), Path: path, RawPath: rawPath}).String()
}

func validateImportContext(c *ImportContext, bindings []ImportBinding, target *tfe.Workspace) (map[string]ImportContextSelection, map[string]string, error) {
	if c == nil || strings.TrimSpace(c.QueryRunID) == "" || c.Target.WorkspaceID != target.ID || target.Organization == nil || !strings.EqualFold(c.Target.OrganizationName, target.Organization.Name) || len(c.SelectedResults) < 1 || len(bindings) != len(c.SelectedResults) || len(c.LockedProviders) == 0 {
		return nil, nil, queryEvidenceError("import_context_invalid")
	}
	digest, err := importDigest(*c)
	if err != nil || digest != c.SelectionDigest {
		return nil, nil, queryEvidenceError("import_context_digest_mismatch")
	}
	selected, addresses, seen := map[string]ImportContextSelection{}, map[string]string{}, map[string]bool{}
	for _, s := range c.SelectedResults {
		provider, ok := c.Providers[s.ListType]
		if !ok || !importTypePattern.MatchString(s.ManagedType) || s.SearchIdentityVersion < 0 || len(s.Identity) == 0 || !exactLockedVersion.MatchString(c.LockedProviders[provider.Source]) {
			return nil, nil, queryEvidenceError("import_context_invalid")
		}
		marker, err := queryResultMarker(c.QueryRunID, provider, s.ListType, s.Identity)
		if err != nil || marker != s.QueryResultMarker || seen[marker] {
			return nil, nil, queryEvidenceError("import_context_marker_invalid")
		}
		seen[marker] = true
		selected[marker] = s
	}
	seen = map[string]bool{}
	for _, binding := range bindings {
		if _, exists := selected[binding.QueryResultMarker]; !exists || seen[binding.QueryResultMarker] || len(binding.TargetAddress) > 512 || !importAddressPattern.MatchString(binding.TargetAddress) || seen["address:"+binding.TargetAddress] {
			return nil, nil, queryEvidenceError("binding_invalid")
		}
		seen[binding.QueryResultMarker] = true
		seen["address:"+binding.TargetAddress] = true
		addresses[binding.QueryResultMarker] = binding.TargetAddress
	}
	return selected, addresses, nil
}

// The verifier never reads QueryRun evidence, local files or caller-supplied
// Plan/CV IDs. Every returned association comes from the exact backend Run.
func ReadVerifyImportPlan(ctx context.Context, c *tfe.Client, input VerifyImportInput) (*VerifyImportResult, error) {
	return readVerifyImportPlanWithConfig(ctx, c, input, defaultVerifyImportPollConfig())
}

// validateNormalizedVerifyImportRequest validates caller input after its opaque
// HCP identifiers have been trimmed and indexes requested Terraform addresses.
func validateNormalizedVerifyImportRequest(input VerifyImportInput, config verifyImportPollConfig) (map[string]bool, error) {
	if input.OrganizationName == "" || input.WorkspaceName == "" || input.RunID == "" {
		return nil, queryEvidenceError("verify_input_invalid")
	}
	if config.wait <= 0 || config.interval <= 0 {
		return nil, queryEvidenceError("verify_config_invalid")
	}
	details := map[string]bool{}
	for _, address := range input.DetailAddresses {
		if address == "" || len(address) > 512 || !importAddressPattern.MatchString(address) || details[address] {
			return nil, queryEvidenceError("verify_input_invalid")
		}
		details[address] = true
	}
	return details, nil
}

// readVerifyImportTarget resolves and validates the user-confirmed target workspace.
func readVerifyImportTarget(ctx context.Context, c *tfe.Client, input VerifyImportInput) (*tfe.Workspace, error) {
	w, err := c.Workspaces.Read(ctx, input.OrganizationName, input.WorkspaceName)
	if err != nil || w == nil || w.ID == "" || w.Name != input.WorkspaceName || w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.OrganizationName) {
		return nil, queryEvidenceError("target_read_unavailable")
	}
	return w, nil
}

// stopVerifyImport builds a non-error result that stops the verification workflow.
func stopVerifyImport(out *VerifyImportResult, status, code, guidance string) (*VerifyImportResult, error) {
	out.Status, out.Diagnostics, out.NextAction = status, []string{code}, guidance
	return out, nil
}

// observeImportRun reads the exact Run and verifies its target and plan-only properties.
func observeImportRun(ctx context.Context, c *tfe.Client, input VerifyImportInput, target *tfe.Workspace, out *VerifyImportResult) (*importRunEvidence, *VerifyImportResult, error) {
	// Keep the raw projection until go-tfe can preserve attribute presence and
	// JSON:API relationship types; tfe.Run currently collapses missing booleans
	// to false and does not expose relationship resource types.
	raw, err := readQueryBackendJSON(ctx, c, "runs/"+url.PathEscape(input.RunID))
	if err != nil {
		return nil, nil, err
	}
	var run importRunEvidence
	if decodeQueryEvidence(raw, &run) != nil || run.Data.ID != input.RunID {
		return nil, nil, queryEvidenceError(runEvidenceInvalidCode)
	}

	out.RunID, out.RunStatus, out.PlanOnly, out.AutoApply = run.Data.ID, run.Data.Attributes.Status, run.Data.Attributes.PlanOnly, run.Data.Attributes.AutoApply
	out.PlanID, out.PlanStatus, out.ConfigurationVersionID = "", "", ""
	workspace := run.Data.Relationships.Workspace.Data
	if workspace == nil || workspace.Type != "workspaces" || workspace.ID != target.ID {
		result, err := stopVerifyImport(out, "blocked", "run_workspace_mismatch", "Stop: the exact Run is not associated with the confirmed target. Do not inspect its Plan as this import attempt.")
		return nil, result, err
	}
	if out.PlanOnly == nil || !*out.PlanOnly || out.AutoApply == nil || *out.AutoApply {
		result, err := stopVerifyImport(out, "blocked", "run_not_plan_only", "Stop: the backend has not confirmed both plan-only=true and auto-apply=false. Do not classify this Run.")
		return nil, result, err
	}
	if input.ImportContext != nil && input.ImportContext.Target.WorkspaceID != target.ID {
		result, err := stopVerifyImport(out, "blocked", "import_context_target_mismatch", "The carried target differs from the Run's workspace. Confirm target and repeat preparation before any new plan.")
		return nil, result, err
	}

	out.RunUIURL = importRunUIURL(c.BaseURL(), target.Organization.Name, target.Name, run.Data.ID)
	if cv := run.Data.Relationships.ConfigurationVersion.Data; cv != nil && cv.Type == "configuration-versions" {
		out.ConfigurationVersionID = cv.ID
	}
	if plan := run.Data.Relationships.Plan.Data; plan != nil && plan.Type == "plans" {
		out.PlanID = plan.ID
	}
	return &run, nil, nil
}

// readAssociatedImportPlan reads the backend-associated Plan and verifies its ID.
func readAssociatedImportPlan(ctx context.Context, c *tfe.Client, out *VerifyImportResult, planID string) (*tfe.Plan, *VerifyImportResult, error) {
	plan, err := c.Plans.Read(ctx, planID)
	if err != nil {
		return nil, nil, queryEvidenceError("plan_read_unavailable")
	}
	if plan == nil || plan.ID != planID {
		result, err := stopVerifyImport(out, "blocked", "plan_association_unverified", "The associated Plan ID could not be verified.")
		return nil, result, err
	}
	out.PlanStatus = string(plan.Status)
	return plan, nil, nil
}

// verifyFinishedImportRun validates and classifies a terminal Run's finished Plan.
func verifyFinishedImportRun(ctx context.Context, c *tfe.Client, input VerifyImportInput, target *tfe.Workspace, out *VerifyImportResult, run *importRunEvidence, details map[string]bool) (*VerifyImportResult, error) {
	cv := run.Data.Relationships.ConfigurationVersion.Data
	planRelation := run.Data.Relationships.Plan.Data
	if cv == nil || strings.TrimSpace(cv.ID) == "" || cv.Type != "configuration-versions" || planRelation == nil || strings.TrimSpace(planRelation.ID) == "" || planRelation.Type != "plans" {
		return stopVerifyImport(out, "blocked", "run_plan_unassociated", "The terminal Run lacks an observed Plan or configuration-version association; do not guess an ID or classify a Plan.")
	}
	plan, result, err := readAssociatedImportPlan(ctx, c, out, planRelation.ID)
	if result != nil || err != nil {
		return result, err
	}
	if plan.Status == tfe.PlanErrored || plan.Status == tfe.PlanCanceled || plan.Status == tfe.PlanUnreachable {
		return stopVerifyImport(out, "failed", "plan_failed", "The associated Plan failed. Review it in HCP Terraform; do not treat it as an import result.")
	}
	if plan.Status != tfe.PlanFinished {
		return stopVerifyImport(out, "blocked", "plan_unavailable", "The terminal Run has no finished associated Plan; do not keep polling it as a successful plan.")
	}
	if input.ImportContext == nil || len(input.Bindings) == 0 {
		return stopVerifyImport(out, "import_context_required", "import_context_required", "Supply the unchanged prepare_import import_context and one query_result_marker-to-target_address binding per selection; no Plan JSON was read.")
	}
	selected, addresses, err := validateImportContext(input.ImportContext, input.Bindings, target)
	if err != nil {
		return stopVerifyImport(out, "blocked", err.Error(), "The import context or bindings are inconsistent. Do not classify this Plan; recheck the reviewed selection and addresses.")
	}
	selectedAddresses := make(map[string]bool, len(addresses))
	for _, address := range addresses {
		selectedAddresses[address] = true
	}
	for requested := range details {
		if !selectedAddresses[requested] {
			return stopVerifyImport(out, "blocked", "detail_address_not_selected", "Detail requests must name selected target addresses; review bindings before retrying.")
		}
	}
	return classifyImportPlan(ctx, c, out, plan.ID, input.ImportContext, selected, addresses, details)
}

// inspectPendingImportPlan checks an in-progress Run's associated Plan for terminal failure.
func inspectPendingImportPlan(ctx context.Context, c *tfe.Client, out *VerifyImportResult, run *importRunEvidence) (*VerifyImportResult, error) {
	planRelation := run.Data.Relationships.Plan.Data
	if planRelation == nil {
		return nil, nil
	}
	if planRelation.Type != "plans" || strings.TrimSpace(planRelation.ID) == "" {
		return stopVerifyImport(out, "blocked", "run_plan_unassociated", "The Run has an invalid Plan relationship. Inspect it in HCP Terraform.")
	}
	plan, result, err := readAssociatedImportPlan(ctx, c, out, planRelation.ID)
	if result != nil || err != nil {
		return result, err
	}
	switch plan.Status {
	case tfe.PlanErrored, tfe.PlanCanceled, tfe.PlanUnreachable:
		return stopVerifyImport(out, "failed", "plan_failed", "The Plan failed, even though the Run status has not caught up. Review it in HCP Terraform.")
	case tfe.PlanFinished, tfe.PlanCreated, tfe.PlanMFAWaiting, tfe.PlanPending, tfe.PlanQueued, tfe.PlanRunning:
		return nil, nil
	default:
		return stopVerifyImport(out, "blocked", "plan_status_unexpected", "Unexpected Plan status; inspect the exact Plan before retrying.")
	}
}

// readVerifyImportPlanWithConfig polls the exact Run and dispatches status-specific verification.
func readVerifyImportPlanWithConfig(ctx context.Context, c *tfe.Client, input VerifyImportInput, config verifyImportPollConfig) (*VerifyImportResult, error) {
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.WorkspaceName = strings.TrimSpace(input.WorkspaceName)
	input.RunID = strings.TrimSpace(input.RunID)
	details, err := validateNormalizedVerifyImportRequest(input, config)
	if err != nil {
		return nil, err
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, config.wait)
	defer cancel()
	target, err := readVerifyImportTarget(deadlineCtx, c, input)
	if err != nil {
		return nil, err
	}
	out := &VerifyImportResult{Status: "blocked", RequestedRunID: input.RunID, OrganizationName: target.Organization.Name, WorkspaceName: target.Name, WorkspaceID: target.ID, Diagnostics: []string{}}
	var observed bool
	for {
		run, result, err := observeImportRun(deadlineCtx, c, input, target, out)
		if err != nil {
			if code, ok := queryEvidenceErrorCode(err); ok && code == runEvidenceInvalidCode {
				return nil, err
			}
			// Caller cancellation or failed backend reads are not pending.
			if ctx.Err() != nil || !observed || deadlineCtx.Err() == nil {
				return nil, queryEvidenceError("run_read_unavailable")
			}
			break
		}
		if result != nil {
			return result, nil
		}
		observed = true
		switch out.RunStatus {
		case "errored", "discarded", "canceled", "force_canceled", "policy_soft_failed":
			return stopVerifyImport(out, "failed", "run_failed", "This plan-only Run failed or was canceled. Review the Run in HCP Terraform; no import was applied.")
		case "planned_and_finished":
			return verifyFinishedImportRun(deadlineCtx, c, input, target, out, run, details)
		case "pending", "fetching", "fetching_completed", "pre_plan_running", "pre_plan_completed", "queuing", "plan_queued", "planning", "planned", "cost_estimating", "cost_estimated", "policy_checking", "policy_checked", "post_plan_running", "post_plan_completed":
			// Import context is optional while the Run is still in progress.
			result, err := inspectPendingImportPlan(deadlineCtx, c, out, run)
			if result != nil || err != nil {
				return result, err
			}
		default:
			return stopVerifyImport(out, "blocked", "run_status_unexpected", "Unexpected Run state; inspect the exact Run in HCP Terraform instead of polling indefinitely.")
		}
		if deadlineCtx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, queryEvidenceError("verify_canceled")
		case <-deadlineCtx.Done():
			break
		case <-time.After(config.interval):
			continue
		}
		break
	}
	if ctx.Err() != nil || !observed {
		return nil, queryEvidenceError("verify_canceled")
	}
	out.Status, out.Diagnostics, out.NextAction, out.RetryAfterSeconds = "pending", []string{"run_pending"}, "Poll verify_import_plan with the same Run ID; import_context/bindings can wait until the associated Plan is finished. Do not start another Run.", 5
	return out, nil
}

type importPlanJSON struct {
	FormatVersion    string                     `json:"format_version"`
	TerraformVersion string                     `json:"terraform_version"`
	Complete         *bool                      `json:"complete"`
	Errored          bool                       `json:"errored"`
	ResourceChanges  []importPlanResourceChange `json:"resource_changes"`
	PlannedValues    struct {
		RootModule importPlanModule `json:"root_module"`
	} `json:"planned_values"`
	OutputChanges   map[string]json.RawMessage `json:"output_changes"`
	DeferredChanges []json.RawMessage          `json:"deferred_changes"`
	ResourceDrift   []importPlanResourceDrift  `json:"resource_drift"`
}

type importPlanResourceChange struct {
	Address      string           `json:"address"`
	Mode         string           `json:"mode"`
	Type         string           `json:"type"`
	ProviderName string           `json:"provider_name"`
	Change       importPlanChange `json:"change"`
}

type importPlanChange struct {
	Actions       []string          `json:"actions"`
	Importing     json.RawMessage   `json:"importing"`
	AfterIdentity json.RawMessage   `json:"after_identity"`
	ReplacePaths  []json.RawMessage `json:"replace_paths"`
}

type importPlanResourceDrift struct {
	Address string `json:"address"`
	Change  struct {
		Actions []string `json:"actions"`
	} `json:"change"`
}

type importPlanOutputChange struct {
	Actions []string `json:"actions"`
	Change  *struct {
		Actions []string `json:"actions"`
	} `json:"change"`
}

type importPlanImporting struct {
	ID       json.RawMessage `json:"id"`
	Identity json.RawMessage `json:"identity"`
	Unknown  bool            `json:"unknown"`
}

// unsupportedImportPlan returns a value-free blocked result for unsafe Plan JSON.
func unsupportedImportPlan(out *VerifyImportResult, code, guidance string) *VerifyImportResult {
	out.Status, out.Diagnostics, out.NextAction = "blocked", []string{code}, guidance+" Review the exact Run and full Plan in HCP Terraform; do not apply or start another Run to diagnose this response."
	return out
}

// readImportPlanJSON reads and validates the Plan JSON envelope before classification.
func readImportPlanJSON(ctx context.Context, c *tfe.Client, out *VerifyImportResult, planID string) (*importPlanJSON, *VerifyImportResult, error) {
	raw, err := c.Plans.ReadJSONOutput(ctx, planID)
	if err != nil {
		return nil, nil, queryEvidenceError("plan_read_unavailable")
	}
	if len(raw) == 0 {
		return nil, unsupportedImportPlan(out, "plan_json_empty", "The associated Plan JSON response was empty."), nil
	}
	var plan importPlanJSON
	if err := decodeQueryEvidence(raw, &plan); err != nil {
		return nil, unsupportedImportPlan(out, "plan_json_invalid", "The associated Plan JSON could not be safely decoded."), nil
	}
	if !terraformPlanFormatVersionPattern.MatchString(plan.FormatVersion) {
		return nil, unsupportedImportPlan(out, "plan_format_unsupported", "The Plan JSON format is missing or is not a supported 1.x format."), nil
	}
	if plan.Complete == nil || !*plan.Complete {
		return nil, unsupportedImportPlan(out, "plan_json_incomplete", "The Plan JSON does not report complete=true, even if the Run and Plan statuses are finished."), nil
	}
	if plan.Errored {
		return nil, unsupportedImportPlan(out, "plan_json_errored", "The Plan JSON reports errored=true despite the finished Plan status."), nil
	}
	if old, known := terraformImportSyntaxVersion(plan.TerraformVersion); !known {
		return nil, unsupportedImportPlan(out, "plan_terraform_version_unavailable", "The Plan JSON lacks a recognized Terraform release or prerelease version; import-block support cannot be checked from this evidence."), nil
	} else if old {
		return nil, unsupportedImportPlan(out, "plan_terraform_version_unsupported", "The Plan JSON reports a Terraform release before 1.5, which cannot support import blocks."), nil
	}
	if len(plan.ResourceChanges) > 10000 || len(plan.ResourceDrift) > 100 || len(plan.DeferredChanges) > 10000 {
		return nil, unsupportedImportPlan(out, "plan_change_limit", "The Plan exceeds verification's resource, drift or deferred-change limits."), nil
	}
	return &plan, nil, nil
}

// addUnselectedImportAddress records bounded, value-free context for unselected changes.
func addUnselectedImportAddress(out *VerifyImportResult, address, kind string) {
	if len(out.Unselected.Addresses) < 50 {
		out.Unselected.Addresses = append(out.Unselected.Addresses, ImportUnselectedAddress{Address: address, Kind: kind})
	} else {
		out.Unselected.Truncated = true
	}
}

// analyzeImportPlan validates aggregate evidence and indexes resource changes and identities.
func analyzeImportPlan(plan *importPlanJSON, out *VerifyImportResult, selectedAddresses map[string]bool) (map[string]int, map[string]uint64, error) {
	out.Plan = &ImportPlanFacts{TerraformVersion: plan.TerraformVersion, FormatVersion: plan.FormatVersion, ResourceChanges: len(plan.ResourceChanges), DeferredChanges: len(plan.DeferredChanges), ResourceDrift: len(plan.ResourceDrift)}
	out.Unselected = &ImportUnselected{Addresses: []ImportUnselectedAddress{}}
	for _, drift := range plan.ResourceDrift {
		if drift.Address == "" || len(drift.Address) > 512 || !importAddressPattern.MatchString(drift.Address) || len(drift.Change.Actions) == 0 {
			return nil, nil, queryEvidenceError("plan_drift_evidence_invalid")
		}
		addUnselectedImportAddress(out, drift.Address, "drift")
	}
	out.Unselected.Drift = len(plan.ResourceDrift)

	for _, raw := range plan.OutputChanges {
		var change importPlanOutputChange
		if decodeQueryEvidence(raw, &change) != nil {
			return nil, nil, queryEvidenceError("plan_evidence_invalid")
		}
		actions := change.Actions
		if len(actions) == 0 && change.Change != nil {
			actions = change.Change.Actions
		}
		if len(actions) == 0 {
			return nil, nil, queryEvidenceError("plan_evidence_invalid")
		}
		if len(actions) != 1 || actions[0] != "no-op" {
			out.Plan.OutputChanges++
		}
	}

	byAddress := map[string]int{}
	for i, resource := range plan.ResourceChanges {
		if resource.Address == "" || len(resource.Address) > 512 || byAddress[resource.Address] != 0 || !importAddressPattern.MatchString(resource.Address) || len(resource.Change.Actions) == 0 || resource.Mode != "managed" && resource.Mode != "data" {
			return nil, nil, queryEvidenceError("plan_evidence_invalid")
		}
		for _, action := range resource.Change.Actions {
			switch action {
			case "no-op", "create", "read", "update", "delete":
			default:
				return nil, nil, queryEvidenceError("plan_actions_unsupported")
			}
		}
		if jsonPresent(resource.Change.AfterIdentity) {
			var after map[string]any
			if decodeQueryEvidence(resource.Change.AfterIdentity, &after) != nil || len(after) == 0 {
				return nil, nil, queryEvidenceError("plan_identity_evidence_invalid")
			}
		}
		byAddress[resource.Address] = i + 1
		if !selectedAddresses[resource.Address] {
			if jsonPresent(resource.Change.Importing) {
				out.Plan.ExtraImports++
				addUnselectedImportAddress(out, resource.Address, "extra_import")
			}
			if resource.Mode == "managed" && (len(resource.Change.Actions) != 1 || resource.Change.Actions[0] != "no-op") {
				out.Plan.OtherManagedActions++
				addUnselectedImportAddress(out, resource.Address, "other_managed_action")
			}
		}
	}

	versions := map[string]uint64{}
	if err := collectImportPlanVersions(plan.PlannedValues.RootModule, versions, 0); err != nil {
		return nil, nil, err
	}
	return byAddress, versions, nil
}

// classifyImportActions maps a supported Terraform action sequence to its result category.
func classifyImportActions(actions []string) (string, error) {
	switch {
	case len(actions) == 1 && actions[0] == "no-op":
		return "none", nil
	case len(actions) == 1 && actions[0] == "update":
		return "update", nil
	case len(actions) == 1 && actions[0] == "create":
		return "create", nil
	case len(actions) == 2 && (actions[0] == "delete" || actions[1] == "delete"):
		return "replace_or_destroy", nil
	default:
		return "", queryEvidenceError("plan_actions_unsupported")
	}
}

// classifySelectedImport compares one selected result with its exact Plan entry.
func classifySelectedImport(plan *importPlanJSON, importContext *ImportContext, marker string, selection ImportContextSelection, address string, byAddress map[string]int, versions map[string]uint64) (ImportPlanAttention, ImportPlanDetail, error) {
	item := ImportPlanAttention{QueryResultMarker: marker, TargetAddress: address, Change: "not_in_plan", ObjectIdentity: "unverified", IdentityReason: "no_matching_plan_entry", SearchIdentityVersion: selection.SearchIdentityVersion}
	detail := ImportPlanDetail{TargetAddress: address}
	if version, ok := versions[address]; ok {
		item.PlanIdentityVersion = &version
	}
	index := byAddress[address]
	if index == 0 {
		return item, detail, nil
	}

	resource := plan.ResourceChanges[index-1]
	detail.Actions, detail.AfterIdentityPresent, detail.ReplacePathCount = resource.Change.Actions, jsonPresent(resource.Change.AfterIdentity), len(resource.Change.ReplacePaths)
	if resource.Mode != "managed" || resource.Type != selection.ManagedType || resource.ProviderName != importContext.Providers[selection.ListType].Source {
		item.Change, item.IdentityReason = "type_mismatch", "plan_type_or_provider_differs"
		return item, detail, nil
	}
	if !jsonPresent(resource.Change.Importing) {
		item.Change, item.IdentityReason = "import_missing", "no_import_marker"
		return item, detail, nil
	}

	var importing importPlanImporting
	if decodeQueryEvidence(resource.Change.Importing, &importing) != nil {
		return item, detail, queryEvidenceError("plan_import_evidence_invalid")
	}
	detail.ImportIDPresent = jsonPresent(importing.ID)
	if importing.Unknown || jsonPresent(importing.ID) == jsonPresent(importing.Identity) {
		item.Change, item.IdentityReason = "import_missing", "no_usable_import_marker"
		return item, detail, nil
	}
	change, err := classifyImportActions(resource.Change.Actions)
	if err != nil {
		return item, detail, err
	}
	item.Change = change
	item.ObjectIdentity, item.IdentityReason = compareImportPlanIdentity(selection, versions, address, resource.Change.AfterIdentity, plan.TerraformVersion, importContext.Target.IdentitySupport[selection.ManagedType])
	return item, detail, nil
}

// finalizeImportPlanResult adds diagnostics and review guidance after classification.
func finalizeImportPlanResult(out *VerifyImportResult, selected int) {
	out.Status, out.Diagnostics = "plan_available", []string{}
	out.NextAction = "Attention lists only selected results with a non-no-op change or identity needing review; an empty list does not clear other Plan actions, outputs or drift. Report identity uncertainty and mismatches separately from Plan actions. Ask the user whether to review the full Plan outside this session via run_ui_url (or the confirmed CLI browser Run URL if absent), or to preview it here. Only if the user chooses in-session review and get_plan_json_output is available, call it with this result's plan_id; raw Plan JSON may contain sensitive values visible to the agent, so summarize actions without reproducing secrets or sensitive attributes. Review other managed actions, outputs, drift and provider scope; this bounded result does not attest the uploaded bytes or authorize apply."
	if out.ObjectIdentity["unverified"]+out.ObjectIdentity["mismatched"]+out.ObjectIdentity["unsupported"] > 0 {
		out.Diagnostics = append(out.Diagnostics, "identity_requires_review")
		out.NextAction = importIdentityReviewAction(out, selected)
	}
	if out.Changes["update"]+out.Changes["create"]+out.Changes["replace_or_destroy"]+out.Plan.ExtraImports+out.Plan.OtherManagedActions+out.Plan.OutputChanges+out.Plan.DeferredChanges+out.Plan.ResourceDrift > 0 {
		out.Diagnostics = append(out.Diagnostics, "plan_actions_require_review")
		out.NextAction += " The Plan contains changes beyond a no-op import (or other changes/drift). Review the full attribute-level diff and reconcile the existing local configuration with the user; this Plan is not safe to apply as an import-only change. If configuration is revised, a separately approved new speculative plan must be verified under its own Run ID."
	}
	out.NextAction += " An import-only Plan or N-to-import summary is not identity proof; review separate identity findings, other actions, module instances, outputs and drift. If the full Plan is satisfactory and the user wants a refinement, adjust only the proposed imported resources and necessary editable local-module interfaces to follow the existing workspace's variables, references and coding practices. Check that changes preserve intended identities, addresses and provider scope; review the diff and obtain separate approval for a new speculative plan of the revised HCL, then verify its own Run ID. A satisfactory first Plan requires no second Plan; identity uncertainty alone does not require changing HCL or starting another Run. After full-Plan review, report the separate action and identity findings and hand off the proposed HCL. Do not run terraform apply, terraform import or apply_run as a continuation of this Search-to-import workflow; any eventual apply belongs to a separate, independently reviewed and authorized workflow."
	out.NextAction += " In the user-facing report, first summarize selected imports with both change=none and object_identity=matched; group them if numerous. Then report selected Plan changes by action, identifying affected addresses as useful. Separately group identities that are not matched by status and identity_reason, explaining in plain language what the evidence means and what remains uncertain; identify affected addresses as useful. A selection can appear in both exception sections. Use attention and the selected bindings for this breakdown, not an inferred overlap between the independent changes and object_identity count maps. Unverified is neither a match nor proof of a mismatch; mismatched needs user review; unsupported means this identity could not be checked with available evidence. Matched is only a bounded comparison, not proof of provider scope or the selected cloud object. Keep unselected actions, outputs and drift separate."
}

// classifyImportPlan projects a finished Plan into value-free import findings.
func classifyImportPlan(ctx context.Context, c *tfe.Client, out *VerifyImportResult, planID string, importContext *ImportContext, selected map[string]ImportContextSelection, addresses map[string]string, detailWanted map[string]bool) (*VerifyImportResult, error) {
	plan, blocked, err := readImportPlanJSON(ctx, c, out, planID)
	if blocked != nil || err != nil {
		return blocked, err
	}
	selectedAddresses := make(map[string]bool, len(addresses))
	for _, address := range addresses {
		selectedAddresses[address] = true
	}
	byAddress, versions, err := analyzeImportPlan(plan, out, selectedAddresses)
	if err != nil {
		return nil, err
	}
	out.Changes = map[string]int{}
	out.ObjectIdentity = map[string]int{}
	markers := make([]string, 0, len(selected))
	for marker := range selected {
		markers = append(markers, marker)
	}
	sort.Strings(markers)
	for _, marker := range markers {
		selection, address := selected[marker], addresses[marker]
		item, detail, err := classifySelectedImport(plan, importContext, marker, selection, address, byAddress, versions)
		if err != nil {
			return nil, err
		}
		if detailWanted[address] {
			out.Detail = append(out.Detail, detail)
		}
		out.Changes[item.Change]++
		out.ObjectIdentity[item.ObjectIdentity]++
		if item.Change != "none" || item.ObjectIdentity != "matched" {
			out.Attention = append(out.Attention, item)
		}
	}
	finalizeImportPlanResult(out, len(selected))
	return out, nil
}

// Keep the two axes distinct and describe only bounded, value-free evidence.
// Attention contains every non-matched selection, even when its Plan action is
// a no-op; version pairs come only from these already-classified entries.
func importIdentityReviewAction(out *VerifyImportResult, selected int) string {
	var versionDiffers, versionMissing int
	var searchVersion int
	var planVersion uint64
	onePair := true
	for _, item := range out.Attention {
		switch item.IdentityReason {
		case "identity_schema_version_differs":
			versionDiffers++
			if item.PlanIdentityVersion == nil {
				onePair = false
			} else if versionDiffers == 1 {
				searchVersion, planVersion = item.SearchIdentityVersion, *item.PlanIdentityVersion
			} else if searchVersion != item.SearchIdentityVersion || planVersion != *item.PlanIdentityVersion {
				onePair = false
			}
		case "identity_schema_version_not_compared":
			versionMissing++
		}
	}
	var identity []string
	if versionDiffers > 0 {
		reason := fmt.Sprintf("%d selected object identities are unverified because Search and Plan identity schema versions differ", versionDiffers)
		if onePair {
			reason += fmt.Sprintf(" (Search %d, Plan %d)", searchVersion, planVersion)
		} else {
			reason += " (see attention for each version pair)"
		}
		identity = append(identity, reason+"; equal-looking values or no-op imports do not establish an identity match.")
	}
	if versionMissing > 0 {
		identity = append(identity, fmt.Sprintf("%d selected object identities are unverified because the Plan identity schema version was not reported for their addresses; values cannot be confirmed comparable.", versionMissing))
	}
	if other := out.ObjectIdentity["unverified"] - versionDiffers - versionMissing; other > 0 {
		identity = append(identity, fmt.Sprintf("%d other selected object identities are unverified (see identity_reason in attention).", other))
	}
	if count := out.ObjectIdentity["mismatched"]; count > 0 {
		identity = append(identity, fmt.Sprintf("Identity mismatches: %d; stop and review the affected selections with the user.", count))
	}
	if count := out.ObjectIdentity["unsupported"]; count > 0 {
		identity = append(identity, fmt.Sprintf("%d selected object identities could not be checked with the available target evidence.", count))
	}
	parts := []string{"Identity needs review: " + strings.Join(identity, " ")}
	if out.Changes["none"] == selected {
		parts = append(parts, fmt.Sprintf("Plan actions, separately: all %d selected imports have no-op actions; this does not prove the selected cloud objects.", selected))
	} else {
		parts = append(parts, fmt.Sprintf("Plan actions, separately: %d of %d selected imports have no-op actions; inspect changes and attention for the others.", out.Changes["none"], selected))
	}
	parts = append(parts, "Review unselected actions, outputs, deferred changes, drift and provider scope independently; this bounded result does not attest uploaded bytes or authorize apply. Ask whether the user prefers full Plan review via run_ui_url (or the confirmed CLI browser Run URL if absent), or an in-session preview. Only with the user's choice and an available get_plan_json_output tool, use this result's plan_id; raw Plan JSON may contain sensitive values visible to the agent, so summarize without reproducing secrets or sensitive attributes.")
	if versionDiffers+versionMissing > 0 {
		parts = append(parts, "If the user asks to investigate identity uncertainty, consult the target's locked provider-release documentation and effective provider configuration, including aliases and module mappings. With the user's chosen Plan review route, compare documented identity and scope meanings with provider-returned Plan identity, relevant resource attributes and available Search observations. Report corroboration, unknowns and conflicts without reproducing sensitive values; stop for user review on a conflict. Keep object_identity unverified even if evidence increases confidence; identity uncertainty alone does not require changing configuration or starting another Run.")
	}
	return strings.Join(parts, " ")
}

type importPlanModule struct {
	Resources []struct {
		Address               string  `json:"address"`
		IdentitySchemaVersion *uint64 `json:"identity_schema_version"`
	} `json:"resources"`
	ChildModules []importPlanModule `json:"child_modules"`
}

func collectImportPlanVersions(module importPlanModule, versions map[string]uint64, depth int) error {
	if depth > 32 {
		return queryEvidenceError("plan_module_depth_limit")
	}
	for _, resource := range module.Resources {
		if resource.IdentitySchemaVersion != nil {
			if _, exists := versions[resource.Address]; exists {
				return queryEvidenceError("plan_identity_version_duplicate")
			}
			versions[resource.Address] = *resource.IdentitySchemaVersion
		}
	}
	for _, child := range module.ChildModules {
		if err := collectImportPlanVersions(child, versions, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func jsonPresent(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func compareImportPlanIdentity(selection ImportContextSelection, versions map[string]uint64, address string, raw json.RawMessage, terraformVersion, support string) (string, string) {
	if !jsonPresent(raw) {
		if old, known := terraformImportSyntaxVersion(terraformVersion); known && old {
			return "unsupported", "terraform_cli_no_identity"
		}
		if support == "none_historically" {
			return "unverified", "historical_type_no_identity_not_current_proof"
		}
		return "unverified", "after_identity_unavailable"
	}
	var after map[string]any
	if decodeQueryEvidence(raw, &after) != nil || len(after) == 0 {
		return "unverified", "after_identity_invalid"
	}
	planVersion, known := versions[address]
	if !known {
		return "unverified", "identity_schema_version_not_compared"
	}
	if planVersion != uint64(selection.SearchIdentityVersion) {
		return "unverified", "identity_schema_version_differs"
	}
	// The object returned independently by the provider is the comparison
	// evidence, never importing.id or importing.identity. Equal JSON primitives
	// are bounded evidence only; provider alias/account scope is not attested.
	if len(after) != len(selection.Identity) {
		return "unverified", "identity_keys_differ"
	}
	for key, want := range selection.Identity {
		got, ok := after[key]
		if !ok {
			return "unverified", "identity_keys_differ"
		}
		// Never claim a complete comparison for nested, unknown or null
		// provider values. Do not treat type-coerced JSON as equal.
		if want == nil || got == nil {
			return "unverified", "identity_value_unavailable"
		}
		switch want.(type) {
		case string, bool, json.Number:
		default:
			return "unverified", "identity_shape_unsupported"
		}
		switch got.(type) {
		case string, bool, json.Number:
		default:
			return "unverified", "identity_shape_unsupported"
		}
		a, _ := json.Marshal(want)
		b, _ := json.Marshal(got)
		if !bytes.Equal(a, b) {
			return "mismatched", "after_identity_differs"
		}
	}
	return "matched", "bounded_identity_comparison"
}
