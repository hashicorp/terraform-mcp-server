// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
)

const verifyImportWait = 40 * time.Second
const verifyImportInterval = 2 * time.Second

var importRunPattern = regexp.MustCompile(`^run-[a-zA-Z0-9-]{1,124}$`)
var importAddressPattern = regexp.MustCompile(`^[a-zA-Z0-9_\[\]".:-]+$`)

type ImportBinding struct {
	QueryResultMarker string `json:"query_result_marker"`
	TargetAddress     string `json:"target_address"`
}

type VerifyImportInput struct {
	OrganizationName string          `json:"organization_name"`
	WorkspaceName    string          `json:"workspace_name"`
	RunID            string          `json:"run_id"`
	Carry            *ImportCarry    `json:"carry,omitempty"`
	Bindings         []ImportBinding `json:"bindings,omitempty"`
	DetailAddresses  []string        `json:"detail_addresses,omitempty"`
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

func validateImportCarry(c *ImportCarry, bindings []ImportBinding, target *tfe.Workspace) (map[string]ImportCarrySelection, map[string]string, error) {
	if c == nil || !validQueryRunID(c.QueryRunID) || c.Target.WorkspaceID != target.ID || target.Organization == nil || !strings.EqualFold(c.Target.OrganizationName, target.Organization.Name) || len(c.SelectedResults) < 1 || len(c.SelectedResults) > maxImportSelections || len(bindings) != len(c.SelectedResults) || len(c.LockedProviders) == 0 {
		return nil, nil, queryEvidenceError("carry_invalid")
	}
	digest, err := importDigest(*c)
	if err != nil || digest != c.SelectionDigest {
		return nil, nil, queryEvidenceError("carry_digest_mismatch")
	}
	selected, addresses, seen := map[string]ImportCarrySelection{}, map[string]string{}, map[string]bool{}
	for _, s := range c.SelectedResults {
		provider, ok := c.Providers[s.ListType]
		if !ok || !importTypePattern.MatchString(s.ManagedType) || s.SearchIdentityVersion < 0 || len(s.Identity) == 0 || !exactLockedVersion.MatchString(c.LockedProviders[provider.Source]) {
			return nil, nil, queryEvidenceError("carry_invalid")
		}
		marker, err := queryResultMarker(c.QueryRunID, provider, s.ListType, s.Identity)
		if err != nil || marker != s.QueryResultMarker || seen[marker] {
			return nil, nil, queryEvidenceError("carry_marker_invalid")
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
	if !validImportWorkspaceName(input.OrganizationName) || !validImportWorkspaceName(input.WorkspaceName) || !importRunPattern.MatchString(input.RunID) || len(input.DetailAddresses) > 20 {
		return nil, queryEvidenceError("verify_input_invalid")
	}
	details := map[string]bool{}
	for _, address := range input.DetailAddresses {
		if address == "" || len(address) > 512 || !importAddressPattern.MatchString(address) || details[address] {
			return nil, queryEvidenceError("verify_input_invalid")
		}
		details[address] = true
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, verifyImportWait)
	defer cancel()
	w, err := c.Workspaces.Read(deadlineCtx, input.OrganizationName, input.WorkspaceName)
	if err != nil || w == nil || !validImportWorkspaceName(w.ID) || w.Name != input.WorkspaceName || w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.OrganizationName) || !allowedImportOrganization(w.Organization.Name) {
		return nil, queryEvidenceError("target_read_unavailable")
	}
	out := &VerifyImportResult{Status: "blocked", RequestedRunID: input.RunID, OrganizationName: w.Organization.Name, WorkspaceName: w.Name, WorkspaceID: w.ID, Diagnostics: []string{}}
	stop := func(status, code, guidance string) (*VerifyImportResult, error) {
		out.Status, out.Diagnostics, out.NextAction = status, []string{code}, guidance
		return out, checkImportResultBudget(out)
	}
	var observed bool
	for {
		raw, err := readQueryBackendJSON(deadlineCtx, c, "runs/"+url.PathEscape(input.RunID))
		if err != nil {
			// Caller cancellation or failed backend reads are not pending.
			if ctx.Err() != nil || !observed || deadlineCtx.Err() == nil {
				return nil, queryEvidenceError("run_read_unavailable")
			}
			break
		}
		var run importRunEvidence
		if decodeQueryEvidence(raw, &run) != nil || run.Data.ID != input.RunID {
			return nil, queryEvidenceError("run_evidence_invalid")
		}
		observed = true
		out.RunID, out.RunStatus, out.PlanOnly, out.AutoApply = run.Data.ID, run.Data.Attributes.Status, run.Data.Attributes.PlanOnly, run.Data.Attributes.AutoApply
		out.PlanID, out.PlanStatus, out.ConfigurationVersionID = "", "", ""
		ws := run.Data.Relationships.Workspace.Data
		if ws == nil || ws.Type != "workspaces" || ws.ID != w.ID {
			return stop("blocked", "run_workspace_mismatch", "Stop: the exact Run is not associated with the confirmed target. Do not inspect its Plan as this import attempt.")
		}
		if out.PlanOnly == nil || !*out.PlanOnly || out.AutoApply == nil || *out.AutoApply {
			return stop("blocked", "run_not_plan_only", "Stop: the backend has not confirmed both plan-only=true and auto-apply=false. Do not classify this Run.")
		}
		if input.Carry != nil && input.Carry.Target.WorkspaceID != w.ID {
			return stop("blocked", "carry_target_mismatch", "The carried target differs from the Run's workspace. Confirm target and repeat preparation before any new plan.")
		}
		cv := run.Data.Relationships.ConfigurationVersion.Data
		if cv != nil && cv.Type == "configuration-versions" {
			out.ConfigurationVersionID = cv.ID
		}
		planRel := run.Data.Relationships.Plan.Data
		if planRel != nil && planRel.Type == "plans" {
			out.PlanID = planRel.ID
		}
		switch out.RunStatus {
		case "errored", "discarded", "canceled", "force_canceled", "policy_soft_failed":
			return stop("failed", "run_failed", "This plan-only Run failed or was canceled. Review the Run in HCP Terraform; no import was applied.")
		case "planned_and_finished":
			if cv == nil || cv.ID == "" || cv.Type != "configuration-versions" || planRel == nil || planRel.ID == "" || planRel.Type != "plans" {
				return stop("blocked", "run_plan_unassociated", "The terminal Run lacks an observed Plan or configuration-version association; do not guess an ID or classify a Plan.")
			}
			plan, err := c.Plans.Read(deadlineCtx, planRel.ID)
			if err != nil {
				return nil, queryEvidenceError("plan_read_unavailable")
			}
			if plan == nil || plan.ID != planRel.ID {
				return stop("blocked", "plan_association_unverified", "The associated Plan ID could not be verified.")
			}
			out.PlanStatus = string(plan.Status)
			if plan.Status == tfe.PlanErrored || plan.Status == tfe.PlanCanceled || plan.Status == tfe.PlanUnreachable {
				return stop("failed", "plan_failed", "The associated Plan failed. Review it in HCP Terraform; do not treat it as an import result.")
			}
			if plan.Status != tfe.PlanFinished {
				return stop("blocked", "plan_unavailable", "The terminal Run has no finished associated Plan; do not keep polling it as a successful plan.")
			}
			if input.Carry == nil || len(input.Bindings) == 0 {
				return stop("carry_required", "carry_required", "Supply the unchanged prepare_import carry and one query_result_marker-to-target_address binding per selection; no Plan JSON was read.")
			}
			selected, addresses, err := validateImportCarry(input.Carry, input.Bindings, w)
			if err != nil {
				return stop("blocked", err.Error(), "The carry or bindings are inconsistent. Do not classify this Plan; recheck the reviewed selection and addresses.")
			}
			for requested := range details {
				found := false
				for _, address := range addresses {
					if requested == address {
						found = true
						break
					}
				}
				if !found {
					return stop("blocked", "detail_address_not_selected", "Detail requests must name selected target addresses; review bindings before retrying.")
				}
			}
			return classifyImportPlan(deadlineCtx, c, out, plan.ID, input.Carry, selected, addresses, details)
		case "pending", "fetching", "fetching_completed", "pre_plan_running", "pre_plan_completed", "queuing", "plan_queued", "planning", "planned", "cost_estimating", "cost_estimated", "policy_checking", "policy_checked", "post_plan_running", "post_plan_completed":
			// Carry is optional while the Run is still in progress.
			if planRel != nil {
				if planRel.Type != "plans" || planRel.ID == "" {
					return stop("blocked", "run_plan_unassociated", "The Run has an invalid Plan relationship. Inspect it in HCP Terraform.")
				}
				plan, err := c.Plans.Read(deadlineCtx, planRel.ID)
				if err != nil {
					return nil, queryEvidenceError("plan_read_unavailable")
				}
				if plan == nil || plan.ID != planRel.ID {
					return stop("blocked", "plan_association_unverified", "The associated Plan ID could not be verified.")
				}
				out.PlanStatus = string(plan.Status)
				switch plan.Status {
				case tfe.PlanErrored, tfe.PlanCanceled, tfe.PlanUnreachable:
					return stop("failed", "plan_failed", "The Plan failed, even though the Run status has not caught up. Review it in HCP Terraform.")
				case tfe.PlanFinished, tfe.PlanCreated, tfe.PlanMFAWaiting, tfe.PlanPending, tfe.PlanQueued, tfe.PlanRunning:
				default:
					return stop("blocked", "plan_status_unexpected", "Unexpected Plan status; inspect the exact Plan before retrying.")
				}
			}
		default:
			return stop("blocked", "run_status_unexpected", "Unexpected Run state; inspect the exact Run in HCP Terraform instead of polling indefinitely.")
		}
		if deadlineCtx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, queryEvidenceError("verify_canceled")
		case <-deadlineCtx.Done():
			break
		case <-time.After(verifyImportInterval):
			continue
		}
		break
	}
	if ctx.Err() != nil || !observed {
		return nil, queryEvidenceError("verify_canceled")
	}
	out.Status, out.Diagnostics, out.NextAction, out.RetryAfterSeconds = "pending", []string{"run_pending"}, "Poll verify_import_plan with the same Run ID; carry/bindings can wait until the associated Plan is finished. Do not start another Run.", 5
	return out, checkImportResultBudget(out)
}

func classifyImportPlan(ctx context.Context, c *tfe.Client, out *VerifyImportResult, planID string, carry *ImportCarry, selected map[string]ImportCarrySelection, addresses map[string]string, detailWanted map[string]bool) (*VerifyImportResult, error) {
	req, err := c.NewRequest(http.MethodGet, "plans/"+url.PathEscape(planID)+"/json-output", nil)
	if err != nil {
		return nil, queryEvidenceError("plan_read_unavailable")
	}
	buf := &queryBoundedWriter{limit: 32 << 20}
	if err := req.Do(ctx, buf); err != nil {
		return nil, queryEvidenceError("plan_read_unavailable")
	}
	var plan struct {
		FormatVersion    string `json:"format_version"`
		TerraformVersion string `json:"terraform_version"`
		Complete         *bool  `json:"complete"`
		Errored          bool   `json:"errored"`
		ResourceChanges  []struct {
			Address      string `json:"address"`
			Mode         string `json:"mode"`
			Type         string `json:"type"`
			ProviderName string `json:"provider_name"`
			Change       struct {
				Actions       []string          `json:"actions"`
				Importing     json.RawMessage   `json:"importing"`
				AfterIdentity json.RawMessage   `json:"after_identity"`
				ReplacePaths  []json.RawMessage `json:"replace_paths"`
			} `json:"change"`
		} `json:"resource_changes"`
		PlannedValues struct {
			RootModule importPlanModule `json:"root_module"`
		} `json:"planned_values"`
		OutputChanges   map[string]json.RawMessage `json:"output_changes"`
		DeferredChanges []json.RawMessage          `json:"deferred_changes"`
		ResourceDrift   []struct {
			Address string `json:"address"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_drift"`
	}
	if len(buf.buffer.Bytes()) == 0 || decodeQueryEvidenceLimit(buf.buffer.Bytes(), &plan, 32<<20, 64) != nil || !regexp.MustCompile(`^1\.[0-9]+$`).MatchString(plan.FormatVersion) || plan.Complete == nil || !*plan.Complete || plan.Errored {
		return nil, queryEvidenceError("plan_incomplete_or_unsupported")
	}
	if old, known := terraformImportSyntaxVersion(plan.TerraformVersion); !known || old || len(plan.ResourceChanges) > 10000 || len(plan.ResourceDrift) > 100 || len(plan.DeferredChanges) > 10000 {
		return nil, queryEvidenceError("plan_incomplete_or_unsupported")
	}
	out.Plan = &ImportPlanFacts{TerraformVersion: plan.TerraformVersion, FormatVersion: plan.FormatVersion, ResourceChanges: len(plan.ResourceChanges), DeferredChanges: len(plan.DeferredChanges), ResourceDrift: len(plan.ResourceDrift)}
	out.Unselected = &ImportUnselected{Addresses: []ImportUnselectedAddress{}}
	addUnselected := func(address, kind string) {
		if len(out.Unselected.Addresses) < 50 {
			out.Unselected.Addresses = append(out.Unselected.Addresses, ImportUnselectedAddress{Address: address, Kind: kind})
		} else {
			out.Unselected.Truncated = true
		}
	}
	for _, drift := range plan.ResourceDrift {
		if drift.Address == "" || len(drift.Address) > 512 || !importAddressPattern.MatchString(drift.Address) || len(drift.Change.Actions) == 0 {
			return nil, queryEvidenceError("plan_drift_evidence_invalid")
		}
		addUnselected(drift.Address, "drift")
	}
	out.Unselected.Drift = len(plan.ResourceDrift)
	for _, raw := range plan.OutputChanges {
		var change struct {
			Actions []string `json:"actions"`
			Change  *struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		}
		if decodeQueryEvidence(raw, &change) != nil {
			return nil, queryEvidenceError("plan_evidence_invalid")
		}
		actions := change.Actions
		if len(actions) == 0 && change.Change != nil {
			actions = change.Change.Actions
		}
		if len(actions) == 0 {
			return nil, queryEvidenceError("plan_evidence_invalid")
		}
		if len(actions) != 1 || actions[0] != "no-op" {
			out.Plan.OutputChanges++
		}
	}
	out.Changes = map[string]int{}
	out.ObjectIdentity = map[string]int{}
	byAddress := map[string]int{}
	selectedAddresses := map[string]bool{}
	for _, addr := range addresses {
		selectedAddresses[addr] = true
	}
	for i, rc := range plan.ResourceChanges {
		if rc.Address == "" || len(rc.Address) > 512 || byAddress[rc.Address] != 0 || !importAddressPattern.MatchString(rc.Address) || len(rc.Change.Actions) == 0 || rc.Mode != "managed" && rc.Mode != "data" {
			return nil, queryEvidenceError("plan_evidence_invalid")
		}
		for _, action := range rc.Change.Actions {
			switch action {
			case "no-op", "create", "read", "update", "delete":
			default:
				return nil, queryEvidenceError("plan_actions_unsupported")
			}
		}
		if jsonPresent(rc.Change.AfterIdentity) {
			var after map[string]any
			if decodeQueryEvidence(rc.Change.AfterIdentity, &after) != nil || len(after) == 0 {
				return nil, queryEvidenceError("plan_identity_evidence_invalid")
			}
		}
		byAddress[rc.Address] = i + 1
		if !selectedAddresses[rc.Address] {
			if jsonPresent(rc.Change.Importing) {
				out.Plan.ExtraImports++
				addUnselected(rc.Address, "extra_import")
			}
			if rc.Mode == "managed" && (len(rc.Change.Actions) != 1 || rc.Change.Actions[0] != "no-op") {
				out.Plan.OtherManagedActions++
				addUnselected(rc.Address, "other_managed_action")
			}
		}
	}
	versions := map[string]uint64{}
	if err := collectImportPlanVersions(plan.PlannedValues.RootModule, versions, 0); err != nil {
		return nil, err
	}
	markers := make([]string, 0, len(selected))
	for marker := range selected {
		markers = append(markers, marker)
	}
	sort.Strings(markers)
	for _, marker := range markers {
		entry := selected[marker]
		addr := addresses[marker]
		item := ImportPlanAttention{QueryResultMarker: marker, TargetAddress: addr, Change: "not_in_plan", ObjectIdentity: "unverified", IdentityReason: "no_matching_plan_entry", SearchIdentityVersion: entry.SearchIdentityVersion}
		detail := ImportPlanDetail{TargetAddress: addr}
		if v, ok := versions[addr]; ok {
			item.PlanIdentityVersion = &v
		}
		if index := byAddress[addr]; index > 0 {
			rc := plan.ResourceChanges[index-1]
			detail.Actions, detail.AfterIdentityPresent, detail.ReplacePathCount = rc.Change.Actions, jsonPresent(rc.Change.AfterIdentity), len(rc.Change.ReplacePaths)
			if rc.Mode != "managed" || rc.Type != entry.ManagedType || rc.ProviderName != carry.Providers[entry.ListType].Source {
				item.Change, item.IdentityReason = "type_mismatch", "plan_type_or_provider_differs"
			} else if !jsonPresent(rc.Change.Importing) {
				item.Change, item.IdentityReason = "import_missing", "no_import_marker"
			} else {
				var importing struct {
					ID       json.RawMessage `json:"id"`
					Identity json.RawMessage `json:"identity"`
					Unknown  bool            `json:"unknown"`
				}
				if decodeQueryEvidence(rc.Change.Importing, &importing) != nil {
					return nil, queryEvidenceError("plan_import_evidence_invalid")
				}
				detail.ImportIDPresent = jsonPresent(importing.ID)
				if importing.Unknown || jsonPresent(importing.ID) == jsonPresent(importing.Identity) {
					item.Change, item.IdentityReason = "import_missing", "no_usable_import_marker"
				} else {
					switch {
					case len(rc.Change.Actions) == 1 && rc.Change.Actions[0] == "no-op":
						item.Change = "none"
					case len(rc.Change.Actions) == 1 && rc.Change.Actions[0] == "update":
						item.Change = "update"
					case len(rc.Change.Actions) == 1 && rc.Change.Actions[0] == "create":
						item.Change = "create"
					case len(rc.Change.Actions) == 2 && (rc.Change.Actions[0] == "delete" || rc.Change.Actions[1] == "delete"):
						item.Change = "replace_or_destroy"
					default:
						return nil, queryEvidenceError("plan_actions_unsupported")
					}
					item.ObjectIdentity, item.IdentityReason = compareImportPlanIdentity(entry, versions, addr, rc.Change.AfterIdentity, plan.TerraformVersion, carry.Target.IdentitySupport[entry.ManagedType])
				}
			}
		}
		if detailWanted[addr] {
			out.Detail = append(out.Detail, detail)
		}
		out.Changes[item.Change]++
		out.ObjectIdentity[item.ObjectIdentity]++
		out.Attention = append(out.Attention, item)
	}
	out.Status, out.Diagnostics = "plan_available", []string{}
	out.NextAction = "Report identity uncertainty and mismatches separately from Plan actions. Review the full HCP Plan, other managed actions, outputs, drift and provider scope with the user. This bounded result does not attest the uploaded bytes or authorize apply."
	if out.ObjectIdentity["unverified"]+out.ObjectIdentity["mismatched"]+out.ObjectIdentity["unsupported"] > 0 {
		out.Diagnostics = append(out.Diagnostics, "identity_requires_review")
		out.NextAction = "Identity is unverified, mismatched or unsupported for some selections; report this before plan-action findings. Confirm documented import form, provider scope and full HCP Plan with the user. A clean import action does not prove the selected cloud object; no apply was authorized."
	}
	if err := checkImportResultBudget(out); err != nil {
		return nil, err
	}
	return out, nil
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

func compareImportPlanIdentity(selection ImportCarrySelection, versions map[string]uint64, address string, raw json.RawMessage, terraformVersion, support string) (string, string) {
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
