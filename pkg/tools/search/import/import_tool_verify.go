// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

// Polling is bounded because MCP clients commonly time out near 60 seconds.
var (
	importVerifyWait     = 40 * time.Second
	importVerifyInterval = 2 * time.Second
)

const (
	maxVerifyAttention  = 100
	maxVerifyUnselected = 50
	maxVerifyDetail     = 20
	maxVerifyPaths      = 25
)

// change describes what the plan does to a selected item.
const (
	changeNone      = "none"
	changeUpdate    = "update"
	changeReplace   = "replace_or_destroy"
	changeCreate    = "create"
	changeMissing   = "import_missing"
	changeNotInPlan = "not_in_plan"
	changeType      = "type_mismatch"
)

// object_identity describes how the provider-returned identity compares with
// the carried Search identity.
const (
	identityMatched     = "matched"
	identityMismatched  = "mismatched"
	identityUnsupported = "unsupported"
	identityUnverified  = "unverified"
)

// Reasons an identity could not be checked.
const (
	reasonCLINoIdentity  = "terraform_cli_no_identity"
	reasonTypeNoIdentity = "resource_type_no_identity"
	reasonUndetermined   = "support_undetermined"
)

var verifyChangeKeys = []string{changeNone, changeUpdate, changeReplace, changeCreate, changeMissing, changeNotInPlan, changeType}
var verifyIdentityKeys = []string{identityMatched, identityMismatched, identityUnsupported, identityUnverified}

type importVerifyBinding struct {
	CandidateID   string `json:"candidate_id"`
	TargetAddress string `json:"target_address"`
}

type importAttention struct {
	CandidateID    string   `json:"candidate_id"`
	TargetAddress  string   `json:"target_address"`
	ManagedType    string   `json:"managed_type"`
	Change         string   `json:"change"`
	ObjectIdentity string   `json:"object_identity"`
	ChangedPaths   []string `json:"changed_paths,omitempty"`
	IdentityReason string   `json:"identity_reason,omitempty"`
}

type importUnsupportedGroup struct {
	ManagedType string `json:"managed_type"`
	Reason      string `json:"reason"`
	Count       int    `json:"count"`
}

type importUnselectedEntry struct {
	Address string `json:"address"`
	Kind    string `json:"kind"`
}

type importUnselected struct {
	ExtraImports       int                     `json:"extra_imports"`
	OtherManagedAction int                     `json:"other_managed_actions"`
	Drift              int                     `json:"drift"`
	Addresses          []importUnselectedEntry `json:"addresses"`
	Truncated          bool                    `json:"truncated"`
}

type importVerifiedPlan struct {
	TerraformVersion string `json:"terraform_version,omitempty"`
	FormatVersion    string `json:"format_version,omitempty"`
	ResourceChanges  int    `json:"resource_changes"`
	OutputChanges    int    `json:"output_changes"`
	DeferredChanges  int    `json:"deferred_changes"`
}

type importVerifyDetail struct {
	TargetAddress   string   `json:"target_address"`
	Actions         []string `json:"actions"`
	ImportIDPresent bool     `json:"import_id_present"`
	IdentityPresent bool     `json:"after_identity_present"`
	ChangedPaths    []string `json:"changed_paths,omitempty"`
	ReplacePaths    int      `json:"replace_path_count,omitempty"`
}

// importVerified is a factual description of what a finished plan showed.
// It never approves, certifies or recommends an apply, and it carries no
// attribute or import ID values.
type importVerified struct {
	ContractVersion     string                   `json:"contract_version"`
	Status              string                   `json:"status"`
	Overall             string                   `json:"overall,omitempty"`
	RunID               string                   `json:"run_id,omitempty"`
	RunStatus           string                   `json:"run_status,omitempty"`
	PlanStatus          string                   `json:"plan_status,omitempty"`
	Selected            int                      `json:"selected,omitempty"`
	Changes             map[string]int           `json:"changes,omitempty"`
	ObjectIdentity      map[string]int           `json:"object_identity,omitempty"`
	IdentityUnsupported []importUnsupportedGroup `json:"identity_unsupported,omitempty"`
	Attention           []importAttention        `json:"attention,omitempty"`
	AttentionTruncated  bool                     `json:"attention_truncated,omitempty"`
	Unselected          *importUnselected        `json:"unselected,omitempty"`
	Plan                *importVerifiedPlan      `json:"plan,omitempty"`
	Detail              []importVerifyDetail     `json:"detail,omitempty"`
	Diagnostics         []string                 `json:"diagnostics"`
	NextAction          string                   `json:"next_action"`
}

func (v importVerified) isBlocked() bool { return v.Status == "blocked" || v.Status == "failed" }

type importVerifyArgs struct {
	Organization string                `json:"organization_name"`
	Workspace    string                `json:"workspace_name"`
	RunID        string                `json:"run_id"`
	Carry        *importCarryBlock     `json:"carry"`
	Bindings     []importVerifyBinding `json:"bindings"`
	Detail       []string              `json:"detail_addresses"`
}

func verifyBlocked(runID, code, next string) importVerified {
	return importVerified{ContractVersion: importToolContractVersion, Status: "blocked", RunID: runID, Diagnostics: []string{code}, NextAction: next}
}

// validateImportCarry checks the carried values are consistent with the
// carried selection. This catches transcription errors; it is not QueryRun
// attestation and not authorization.
func validateImportCarry(carry *importCarryBlock, bindings []importVerifyBinding) (map[string]string, error) {
	if carry == nil || carry.QueryRunID == "" || len(carry.Candidates) < 1 || len(carry.Candidates) > maxImportSelections || carry.Destination.IdentitySupport == nil {
		return nil, importEvidenceFailure("carry_invalid")
	}
	if importCarryDigest(*carry) != carry.SelectionDigest {
		return nil, importEvidenceFailure("carry_digest_mismatch")
	}
	known := make(map[string]importCarryCandidate, len(carry.Candidates))
	for _, c := range carry.Candidates {
		p, ok := carry.Providers[c.ListType]
		if !ok || !importInputName(c.ManagedType) || len(c.Identity) == 0 {
			return nil, importEvidenceFailure("carry_invalid")
		}
		if importCandidateID(carry.QueryRunID, workspaceProvider{Source: p.Source, Version: p.Version}, c.ListType, c.Identity) != c.CandidateID {
			return nil, importEvidenceFailure("carry_candidate_id_mismatch")
		}
		if _, dup := known[c.CandidateID]; dup {
			return nil, importEvidenceFailure("carry_invalid")
		}
		known[c.CandidateID] = c
	}
	if len(bindings) != len(carry.Candidates) {
		return nil, importEvidenceFailure("bindings_incomplete")
	}
	addresses, byCandidate := map[string]bool{}, map[string]string{}
	for _, b := range bindings {
		if _, ok := known[b.CandidateID]; !ok || !validImportTargetAddress(b.TargetAddress) || addresses[b.TargetAddress] {
			return nil, importEvidenceFailure("bindings_invalid")
		}
		if _, dup := byCandidate[b.CandidateID]; dup {
			return nil, importEvidenceFailure("bindings_invalid")
		}
		addresses[b.TargetAddress] = true
		byCandidate[b.CandidateID] = b.TargetAddress
	}
	return byCandidate, nil
}

// waitForImportPlan polls the exact run for a bounded time. A timeout returns
// pending, never a verdict.
func waitForImportPlan(ctx context.Context, c *tfe.Client, w *tfe.Workspace, runID string) (*tfe.Run, *tfe.Plan, string, error) {
	deadline := time.Now().Add(importVerifyWait)
	for {
		r, err := c.Runs.Read(ctx, runID)
		if err != nil {
			return nil, nil, "", importReadError(err, 0)
		}
		if r.ID != runID || !r.PlanOnly || r.Workspace == nil || r.Workspace.ID != w.ID || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID == "" {
			return nil, nil, "", importEvidenceFailure("run_association_unverified")
		}
		if r.AutoApply {
			return nil, nil, "", importEvidenceFailure("run_not_plan_only")
		}
		state := "pending"
		var p *tfe.Plan
		if r.Plan != nil && r.Plan.ID != "" {
			if p, err = c.Plans.Read(ctx, r.Plan.ID); err != nil {
				return nil, nil, "", importReadError(err, 0)
			}
			switch {
			case p.Status == tfe.PlanErrored || p.Status == tfe.PlanCanceled || p.Status == tfe.PlanUnreachable || r.Status == tfe.RunErrored || r.Status == tfe.RunCanceled || r.Status == tfe.RunDiscarded:
				state = "failed"
			case p.Status == tfe.PlanFinished && r.Status == tfe.RunPlannedAndFinished:
				state = "finished"
			}
		}
		if state != "pending" || !time.Now().Add(importVerifyInterval).Before(deadline) {
			return r, p, state, nil
		}
		select {
		case <-ctx.Done():
			return r, p, "pending", nil
		case <-time.After(importVerifyInterval):
		}
	}
}

type importVerifyPlanJSON struct {
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
			Before        json.RawMessage   `json:"before"`
			After         json.RawMessage   `json:"after"`
			AfterUnknown  json.RawMessage   `json:"after_unknown"`
			ReplacePaths  []json.RawMessage `json:"replace_paths"`
		} `json:"change"`
	} `json:"resource_changes"`
	OutputChanges   map[string]json.RawMessage `json:"output_changes"`
	DeferredChanges []json.RawMessage          `json:"deferred_changes"`
	ResourceDrift   []struct {
		Address string `json:"address"`
		Change  struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"resource_drift"`
}

func isNoop(actions []string) bool { return len(actions) == 1 && actions[0] == "no-op" }

func classifyChange(actions []string) string {
	has := func(a string) bool {
		for _, x := range actions {
			if x == a {
				return true
			}
		}
		return false
	}
	switch {
	case isNoop(actions):
		return changeNone
	case has("delete"):
		return changeReplace
	case has("create"):
		return changeCreate
	default:
		return changeUpdate
	}
}

// changedPaths returns top-level attribute names whose planned value differs.
// Values are never returned.
func changedPaths(before, after, unknown json.RawMessage) []string {
	var b, a, u map[string]json.RawMessage
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)
	_ = json.Unmarshal(unknown, &u)
	set := map[string]bool{}
	for k, av := range a {
		if bv, ok := b[k]; !ok || !bytes.Equal(bytes.TrimSpace(av), bytes.TrimSpace(bv)) {
			set[k] = true
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			set[k] = true
		}
	}
	for k, v := range u {
		if !bytes.Equal(bytes.TrimSpace(v), []byte("false")) && !bytes.Equal(bytes.TrimSpace(v), []byte("{}")) && !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > maxVerifyPaths {
		out = out[:maxVerifyPaths]
	}
	return out
}

// classifyIdentity compares the provider-returned identity with the carried
// Search identity. Support comes from the plan's own Terraform version and
// per-resource after_identity; the carried destination only names the reason.
func classifyIdentity(planVersion string, carried importCarryCandidate, support string, afterIdentity json.RawMessage) (string, string) {
	hasAfter := len(afterIdentity) > 0 && !bytes.Equal(bytes.TrimSpace(afterIdentity), []byte("null"))
	if hasAfter {
		status, reason := importCompareProviderIdentity(carried.Identity, afterIdentity)
		return status, reason
	}
	if atLeast, known := terraformVersionAtLeast(planVersion, 1, 12); known && !atLeast {
		return identityUnsupported, reasonCLINoIdentity
	}
	switch support {
	case importIdentityNone:
		return identityUnsupported, reasonTypeNoIdentity
	case importIdentitySupported:
		return identityUnverified, "provider_returned_identity_unavailable"
	}
	return identityUnsupported, reasonUndetermined
}

// verifyImportPlanFacts reads the finished plan once and classifies every
// selected item on two independent axes.
func verifyImportPlanFacts(raw []byte, carry *importCarryBlock, byCandidate map[string]string, detailAddrs []string) (importVerified, error) {
	var plan importVerifyPlanJSON
	if err := decodeImportEvidenceJSONLimit(raw, &plan, maxImportSchemaBytes); err != nil {
		return importVerified{}, err
	}
	if !importCompatibleFormat(plan.FormatVersion) || plan.Complete == nil || !*plan.Complete || plan.Errored {
		return importVerified{}, importEvidenceFailure("plan_evidence_incomplete")
	}
	if len(plan.ResourceChanges) > 10000 || len(plan.ResourceDrift) > 100 {
		return importVerified{}, importEvidenceFailure("plan_resource_limit")
	}

	out := importVerified{ContractVersion: importToolContractVersion, Status: "plan_available", Selected: len(carry.Candidates), Diagnostics: []string{},
		Changes: map[string]int{}, ObjectIdentity: map[string]int{},
		Plan: &importVerifiedPlan{TerraformVersion: plan.TerraformVersion, FormatVersion: plan.FormatVersion, ResourceChanges: len(plan.ResourceChanges), OutputChanges: len(plan.OutputChanges), DeferredChanges: len(plan.DeferredChanges)}}
	for _, k := range verifyChangeKeys {
		out.Changes[k] = 0
	}
	for _, k := range verifyIdentityKeys {
		out.ObjectIdentity[k] = 0
	}

	byAddress := map[string]int{}
	index := map[string]int{}
	for i, rc := range plan.ResourceChanges {
		byAddress[rc.Address]++
		index[rc.Address] = i
	}
	selectedAddr := map[string]bool{}
	unsupported := map[[2]string]int{}
	detailWanted := map[string]bool{}
	for _, a := range detailAddrs {
		detailWanted[a] = true
	}

	for _, cand := range carry.Candidates {
		addr := byCandidate[cand.CandidateID]
		selectedAddr[addr] = true
		item := importAttention{CandidateID: cand.CandidateID, TargetAddress: addr, ManagedType: cand.ManagedType}
		var afterIdentity json.RawMessage
		i, found := index[addr]
		switch {
		case byAddress[addr] > 1:
			return importVerified{}, importEvidenceFailure("plan_address_duplicate")
		case !found:
			item.Change, item.ObjectIdentity, item.IdentityReason = changeNotInPlan, identityUnverified, "no_matching_plan_entry"
		default:
			rc := plan.ResourceChanges[i]
			p := carry.Providers[cand.ListType]
			imported := len(rc.Change.Importing) > 0 && !bytes.Equal(bytes.TrimSpace(rc.Change.Importing), []byte("null"))
			var marker struct {
				ID       json.RawMessage `json:"id"`
				Identity json.RawMessage `json:"identity"`
				Unknown  bool            `json:"unknown"`
			}
			if imported && decodeImportEvidenceJSONLimit(rc.Change.Importing, &marker, maxImportSchemaBytes) != nil {
				return importVerified{}, importEvidenceFailure("plan_import_evidence_invalid")
			}
			idPresent := len(marker.ID) > 0 && !bytes.Equal(bytes.TrimSpace(marker.ID), []byte("null"))
			identityPresent := len(marker.Identity) > 0 && !bytes.Equal(bytes.TrimSpace(marker.Identity), []byte("null"))
			afterIdentity = rc.Change.AfterIdentity
			switch {
			case rc.Mode != "managed" || rc.Type != cand.ManagedType || rc.ProviderName != p.Source:
				item.Change, item.ObjectIdentity, item.IdentityReason = changeType, identityUnverified, "plan_type_or_provider_differs_from_selection"
			case !imported || marker.Unknown || idPresent == identityPresent:
				item.Change, item.ObjectIdentity, item.IdentityReason = changeMissing, identityUnverified, "no_usable_import_marker"
			default:
				item.Change = classifyChange(rc.Change.Actions)
				if item.Change != changeNone {
					item.ChangedPaths = changedPaths(rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown)
				}
				item.ObjectIdentity, item.IdentityReason = classifyIdentity(plan.TerraformVersion, cand, carry.Destination.IdentitySupport[cand.ManagedType], afterIdentity)
			}
			if detailWanted[addr] && len(out.Detail) < maxVerifyDetail {
				out.Detail = append(out.Detail, importVerifyDetail{TargetAddress: addr, Actions: rc.Change.Actions, ImportIDPresent: idPresent, IdentityPresent: len(afterIdentity) > 0 && !bytes.Equal(bytes.TrimSpace(afterIdentity), []byte("null")), ChangedPaths: changedPaths(rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown), ReplacePaths: len(rc.Change.ReplacePaths)})
			}
		}
		out.Changes[item.Change]++
		out.ObjectIdentity[item.ObjectIdentity]++
		if item.ObjectIdentity == identityUnsupported {
			unsupported[[2]string{cand.ManagedType, item.IdentityReason}]++
		}
		// Unsupported identity is counted, not attention. Everything else
		// that is not a clean match with no change is.
		if item.Change != changeNone || item.ObjectIdentity == identityMismatched || item.ObjectIdentity == identityUnverified {
			if len(out.Attention) < maxVerifyAttention {
				out.Attention = append(out.Attention, item)
			} else {
				out.AttentionTruncated = true
			}
		}
	}

	groups := make([]importUnsupportedGroup, 0, len(unsupported))
	for k, n := range unsupported {
		groups = append(groups, importUnsupportedGroup{ManagedType: k[0], Reason: k[1], Count: n})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ManagedType != groups[j].ManagedType {
			return groups[i].ManagedType < groups[j].ManagedType
		}
		return groups[i].Reason < groups[j].Reason
	})
	out.IdentityUnsupported = groups

	un := &importUnselected{Addresses: []importUnselectedEntry{}}
	add := func(address, kind string) {
		if len(un.Addresses) < maxVerifyUnselected {
			un.Addresses = append(un.Addresses, importUnselectedEntry{Address: address, Kind: kind})
		} else {
			un.Truncated = true
		}
	}
	for _, rc := range plan.ResourceChanges {
		if selectedAddr[rc.Address] {
			continue
		}
		if len(rc.Change.Importing) > 0 && !bytes.Equal(bytes.TrimSpace(rc.Change.Importing), []byte("null")) {
			un.ExtraImports++
			add(rc.Address, "extra_import")
		}
		if rc.Mode == "managed" && !isNoop(rc.Change.Actions) {
			un.OtherManagedAction++
			add(rc.Address, "other_managed_action")
		}
	}
	for _, d := range plan.ResourceDrift {
		if d.Address == "" || len(d.Change.Actions) == 0 {
			return importVerified{}, importEvidenceFailure("plan_drift_evidence_invalid")
		}
		un.Drift++
		add(d.Address, "drift")
	}
	out.Unselected = un

	out.Overall = verifyOverall(out)
	out.NextAction = verifyNextAction(out)
	return out, nil
}

func verifyOverall(v importVerified) string {
	if v.Changes[changeCreate]+v.Changes[changeMissing]+v.Changes[changeNotInPlan]+v.Changes[changeType] > 0 || v.Plan.DeferredChanges > 0 {
		return "blocked"
	}
	if v.Changes[changeUpdate]+v.Changes[changeReplace] > 0 || v.ObjectIdentity[identityMismatched]+v.ObjectIdentity[identityUnverified] > 0 ||
		v.Unselected.ExtraImports+v.Unselected.OtherManagedAction+v.Unselected.Drift > 0 || v.Plan.OutputChanges > 0 {
		return "needs_iteration"
	}
	return "no_unintended_changes"
}

// verifyNextAction describes what the plan showed with counts. It never says
// safe, verified or approved, and never suggests an apply (ADR 0005).
func verifyNextAction(v importVerified) string {
	var parts []string
	n := v.Selected
	switch v.Overall {
	case "blocked":
		parts = append(parts, fmt.Sprintf("The plan shows %d of %d selected imports not taking effect (create %d, import_missing %d, not_in_plan %d, type_mismatch %d)", v.Changes[changeCreate]+v.Changes[changeMissing]+v.Changes[changeNotInPlan]+v.Changes[changeType], n, v.Changes[changeCreate], v.Changes[changeMissing], v.Changes[changeNotInPlan], v.Changes[changeType]))
		if v.Plan.DeferredChanges > 0 {
			parts = append(parts, fmt.Sprintf("and %d deferred changes", v.Plan.DeferredChanges))
		}
		parts[len(parts)-1] += ". Fix the configuration listed in attention, then create a new configuration version and run."
	case "needs_iteration":
		if v.ObjectIdentity[identityMismatched] > 0 {
			parts = append(parts, fmt.Sprintf("%d selected items have an identity that differs from the carried selection. Stop and review this selection with the user.", v.ObjectIdentity[identityMismatched]))
		}
		parts = append(parts, fmt.Sprintf("The plan shows %d selected items with updates or replacements, %d with unverified identity, and %d other actions outside the selection (extra imports %d, other changes %d, drift %d, output changes %d). Review attention and unselected, adjust the configuration, then create a new configuration version and run.",
			v.Changes[changeUpdate]+v.Changes[changeReplace], v.ObjectIdentity[identityUnverified], v.Unselected.ExtraImports+v.Unselected.OtherManagedAction+v.Unselected.Drift+v.Plan.OutputChanges, v.Unselected.ExtraImports, v.Unselected.OtherManagedAction, v.Unselected.Drift, v.Plan.OutputChanges))
	default:
		parts = append(parts, fmt.Sprintf("The plan shows %d selected imports with no changes and no other actions. %d identities matched the carried selection. Review the plan.", n, v.ObjectIdentity[identityMatched]))
	}
	if u := v.ObjectIdentity[identityUnsupported]; u > 0 {
		undetermined := 0
		for _, g := range v.IdentityUnsupported {
			if g.Reason == reasonUndetermined {
				undetermined += g.Count
			}
		}
		s := fmt.Sprintf("Identity could not be checked for %d items (see identity_unsupported). Confirm each import ID against the destination provider version's documentation.", u)
		if undetermined > 0 {
			s += fmt.Sprintf(" Identity support could not be determined for %d of them; it was not read from HCP (blank workspace, or a schema the agent obtained locally).", undetermined)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func verifyImportPlan(ctx context.Context, c *tfe.Client, a importVerifyArgs) importVerified {
	w, err := c.Workspaces.Read(ctx, a.Organization, a.Workspace)
	if err != nil {
		return verifyBlocked(a.RunID, importDiagnosticCode(importReadError(err, 0)), "Resolve the reported diagnostic and call verify_import_plan again.")
	}
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, a.Organization) {
		return verifyBlocked(a.RunID, "workspace_ownership_unverified", "Use the organization and workspace the run was created in.")
	}
	r, p, state, err := waitForImportPlan(ctx, c, w, a.RunID)
	if err != nil {
		return verifyBlocked(a.RunID, importDiagnosticCode(err), "Resolve the reported diagnostic. Use the run_id returned by create_import_run.")
	}
	out := importVerified{ContractVersion: importToolContractVersion, RunID: a.RunID, RunStatus: string(r.Status), Diagnostics: []string{}}
	if p != nil {
		out.PlanStatus = string(p.Status)
	}
	switch state {
	case "pending":
		out.Status = "pending"
		out.NextAction = "The plan has not finished. Call verify_import_plan again with the same run_id."
		return out
	case "failed":
		out.Status = "failed"
		out.Diagnostics = []string{"speculative_plan_failed"}
		out.NextAction = "The plan did not finish successfully. Read get_plan_logs for this plan, repair the configuration, and create a new configuration version and run. Do not retry a create blindly."
		return out
	}
	if a.Carry == nil || len(a.Bindings) == 0 {
		out.Status = "carry_required"
		out.NextAction = "The plan finished. Call verify_import_plan again with the carry block from prepare_import and bindings (candidate_id and target_address for every selection)."
		return out
	}
	byCandidate, err := validateImportCarry(a.Carry, a.Bindings)
	if err != nil {
		return verifyBlocked(a.RunID, importDiagnosticCode(err), "The carried selection is not consistent. Pass the carry block exactly as prepare_import returned it, with one binding per candidate. Nothing about the plan was read.")
	}
	raw, status, err := readImportBackendJSON(ctx, c, "plans/"+url.PathEscape(p.ID)+"/json-output", maxImportSchemaBytes)
	if err != nil || status != 200 {
		return verifyBlocked(a.RunID, importDiagnosticCode(importReadError(err, status)), "The plan JSON could not be read. Call verify_import_plan again, or read get_plan_json_output.")
	}
	facts, err := verifyImportPlanFacts(raw, a.Carry, byCandidate, a.Detail)
	if err != nil {
		return verifyBlocked(a.RunID, importDiagnosticCode(err), "The plan could not be summarized completely. Read get_plan_json_output for this plan; do not treat a partial summary as complete.")
	}
	facts.RunID, facts.RunStatus, facts.PlanStatus = a.RunID, string(r.Status), string(p.Status)
	return facts
}

// VerifyImportPlanDefinition describes verify_import_plan.
func VerifyImportPlanDefinition() mcp.Tool {
	return mcp.NewTool("verify_import_plan",
		mcp.WithDescription(`Wait up to about 40 seconds for a plan-only import Run, then describe what the finished plan showed. Read-only.

Poll with organization_name, workspace_name and run_id from create_import_run; while the plan runs the result is a short status, so call again with the same run_id. When the plan has finished, call once more with the carry block from prepare_import (unchanged) and bindings: one candidate_id and target_address for every selected candidate.

The result gives, for each selected item, a change (none, update, replace_or_destroy, create, import_missing, not_in_plan, type_mismatch) and an object_identity (matched, mismatched, unsupported, unverified), as counts plus an attention list of items that need a look. unsupported means the identity could not be compared (Terraform below 1.12 or a type with no identity); confirm those import IDs against the destination provider version's documentation. unselected lists extra imports, other actions and drift. No attribute or import ID values are returned; use get_plan_json_output for detail. The summary describes the plan; it is not an approval and a plan-only Run never imports into state.`),
		mcp.WithTitleAnnotation("Verify import plan"),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true), mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("organization_name", mcp.Required(), mcp.Description("HCP Terraform organization.")),
		mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Destination workspace name.")),
		mcp.WithString("run_id", mcp.Required(), mcp.Description("Run ID returned by create_import_run.")),
		mcp.WithObject("carry", mcp.Description("The carry block from prepare_import, unchanged. Required once the plan has finished.")),
		mcp.WithArray("bindings", mcp.Description("One entry per selected candidate. Required once the plan has finished."), mcp.MaxItems(maxImportSelections), mcp.Items(map[string]any{
			"type": "object", "required": []string{"candidate_id", "target_address"},
			"properties":           map[string]any{"candidate_id": map[string]any{"type": "string"}, "target_address": map[string]any{"type": "string"}},
			"additionalProperties": false,
		})),
		mcp.WithArray("detail_addresses", mcp.Description("Up to 20 target addresses to return action and changed-attribute names for. Values are never returned."), mcp.MaxItems(maxVerifyDetail), mcp.WithStringItems()),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithOutputSchema[importVerified]())
}

// HandleVerifyImportPlan serves verify_import_plan.
func HandleVerifyImportPlan(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	var a importVerifyArgs
	if err := decodeImportToolArguments(request.GetArguments(), &a, 512*1024); err != nil || !importInputName(a.Organization) || !importInputName(a.Workspace) || !importInputName(a.RunID) || len(a.Bindings) > maxImportSelections || len(a.Detail) > maxVerifyDetail {
		return importToolResult(verifyBlocked("", "import_input_invalid", "Supply organization_name, workspace_name and run_id; once the plan has finished also the carry block and bindings."))
	}
	if err := client.AuthorizeOrganization(ctx, a.Organization); err != nil {
		return importToolResult(verifyBlocked(a.RunID, "organization_not_allowed", "Use an organization allowed by this server."))
	}
	ctx, cancel := context.WithTimeout(ctx, importVerifyWait+20*time.Second)
	defer cancel()
	c, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return importToolResult(verifyBlocked(a.RunID, "backend_client_unavailable", "Supply current backend credentials and retry."))
	}
	return importToolResult(verifyImportPlan(ctx, c, a))
}
