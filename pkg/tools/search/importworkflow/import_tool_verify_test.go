// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const verifyProvider = "registry.terraform.io/hashicorp/aws"

var forbiddenNextActionWords = regexp.MustCompile(`(?i)\b(safe|safely|verified|approved|approve|apply)\b`)

type verifyFixture struct {
	carry    *importCarryBlock
	bindings []importVerifyBinding
	byCand   map[string]string
}

func newVerifyFixture(t *testing.T, n int, support, destVersion string) verifyFixture {
	t.Helper()
	carry := &importCarryBlock{
		QueryRunID: "qry-verify",
		Providers:  map[string]importCarryProvider{"aws_iam_role": {Source: verifyProvider, Version: "6.62.0"}},
		Target:     importCarryTarget{WorkspaceID: "ws-fixture", TerraformVersion: destVersion, IdentitySupport: map[string]string{"aws_iam_role": support}},
	}
	f := verifyFixture{carry: carry, byCand: map[string]string{}}
	for i := 0; i < n; i++ {
		identity := map[string]any{"account_id": "123456789012", "name": fmt.Sprintf("role-%d", i)}
		id := importCandidateID("qry-verify", workspaceProvider{Source: verifyProvider, Version: "6.62.0"}, "aws_iam_role", identity)
		carry.Candidates = append(carry.Candidates, importCarryCandidate{CandidateID: id, ListType: "aws_iam_role", ManagedType: "aws_iam_role", Identity: identity, SearchIdentityVersion: intPtr(0)})
		addr := fmt.Sprintf("aws_iam_role.r%d", i)
		f.bindings = append(f.bindings, importVerifyBinding{CandidateID: id, TargetAddress: addr})
		f.byCand[id] = addr
	}
	carry.SelectionDigest = importCarryDigest(*carry)
	return f
}

type planEntry struct {
	address, typ, provider string
	actions                []string
	importing              string
	afterIdentity          string
	before, after          string
	// identityVersion is the plan's planned_values identity_schema_version for
	// the address. nil means the plan reports none.
	identityVersion *int
}

func (e planEntry) json() string {
	if e.typ == "" {
		e.typ = "aws_iam_role"
	}
	if e.provider == "" {
		e.provider = verifyProvider
	}
	if e.actions == nil {
		e.actions = []string{"no-op"}
	}
	if e.importing == "" {
		e.importing = `{"id":"MUST-NOT-LEAK"}`
	}
	if e.before == "" {
		e.before = `{"name":"x","path":"/"}`
	}
	if e.after == "" {
		e.after = e.before
	}
	acts, _ := json.Marshal(e.actions)
	s := fmt.Sprintf(`{"address":%q,"mode":"managed","type":%q,"provider_name":%q,"change":{"actions":%s,"importing":%s,"before":%s,"after":%s,"after_unknown":{}`, e.address, e.typ, e.provider, acts, e.importing, e.before, e.after)
	if e.afterIdentity != "" {
		s += `,"after_identity":` + e.afterIdentity
	}
	return s + "}}"
}

func (f verifyFixture) cleanEntries(withIdentity bool) []planEntry {
	var entries []planEntry
	for i, c := range f.carry.Candidates {
		e := planEntry{address: f.bindings[i].TargetAddress, identityVersion: intPtr(0)}
		if withIdentity {
			encoded, _ := json.Marshal(c.Identity)
			e.afterIdentity = string(encoded)
		}
		entries = append(entries, e)
	}
	return entries
}

// planJSON builds a finished plan. Entries with an identityVersion get a
// planned_values record carrying it, unless extra supplies its own
// planned_values (a plan that reports no versions, or nested modules).
func planJSON(version string, entries []planEntry, extra string) []byte {
	parts := make([]string, len(entries))
	var versions []string
	for i, e := range entries {
		parts[i] = e.json()
		if e.identityVersion != nil {
			versions = append(versions, fmt.Sprintf(`{"address":%q,"identity_schema_version":%d}`, e.address, *e.identityVersion))
		}
	}
	if len(versions) > 0 && !strings.Contains(extra, `"planned_values"`) {
		extra += fmt.Sprintf(`,"planned_values":{"root_module":{"resources":[%s]}}`, strings.Join(versions, ","))
	}
	tv := ""
	if version != "" {
		tv = fmt.Sprintf(`"terraform_version":%q,`, version)
	}
	return []byte(fmt.Sprintf(`{"format_version":"1.2",%s"complete":true,"errored":false,"resource_changes":[%s],"output_changes":{}%s}`, tv, strings.Join(parts, ","), extra))
}

func verifyFacts(t *testing.T, f verifyFixture, plan []byte, detail ...string) importVerified {
	t.Helper()
	out, err := verifyImportPlanFacts(plan, f.carry, f.byCand, detail)
	require.NoError(t, err)
	assert.False(t, forbiddenNextActionWords.MatchString(out.NextAction), out.NextAction)
	encoded, _ := json.Marshal(out)
	assert.NotContains(t, string(encoded), "MUST-NOT-LEAK", "import ID values must never be returned")
	assert.NotContains(t, string(encoded), `"overall"`, "there is no combined verdict")
	assert.Equal(t, "plan_available", out.Status, "status is availability only")
	return out
}

func TestVerifyAllNoChangeMatchedIdentity(t *testing.T) {
	f := newVerifyFixture(t, 3, importIdentitySupported, "1.16.1")
	out := verifyFacts(t, f, planJSON("1.16.1", f.cleanEntries(true), ""))
	assert.Equal(t, 3, out.Changes[changeNone])
	assert.Equal(t, 3, out.ObjectIdentity[identityMatched])
	assert.Empty(t, out.Attention)
	assert.Empty(t, out.IdentityUnsupported)
	assert.Contains(t, out.NextAction, "3 selected imports with no changes")
	assert.Contains(t, out.NextAction, "3 identities matched")
	assert.Contains(t, out.NextAction, "not a general correctness guarantee")
}

func TestVerifyIdentityUnsupportedReasons(t *testing.T) {
	for _, tt := range []struct {
		name, support, planVersion, reason string
		undetermined                       bool
	}{
		{"terraform below 1.12", importIdentityUnknown, "1.11.4", reasonCLINoIdentity, false},
		{"below 1.12 even if a type is carried as supported", importIdentitySupported, "1.5.7", reasonCLINoIdentity, false},
		{"type has no identity", importIdentityNone, "1.16.1", reasonTypeNoIdentity, false},
		{"support undetermined", importIdentityUnknown, "1.16.1", reasonUndetermined, true},
		{"plan version missing and support undetermined", importIdentityUnknown, "", reasonUndetermined, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newVerifyFixture(t, 2, tt.support, "1.16.1")
			out := verifyFacts(t, f, planJSON(tt.planVersion, f.cleanEntries(false), ""))
			assert.Equal(t, 2, out.ObjectIdentity[identityUnsupported])
			assert.Empty(t, out.Attention, "unsupported identity is counted, not attention")
			require.Len(t, out.IdentityUnsupported, 1)
			assert.Equal(t, importUnsupportedGroup{ManagedType: "aws_iam_role", Reason: tt.reason, Count: 2}, out.IdentityUnsupported[0])
			assert.True(t, strings.HasPrefix(out.NextAction, "Identity could not be checked for 2 items"), "identity uncertainty leads: %s", out.NextAction)
			assert.Contains(t, out.NextAction, importConfidenceReportRule)
			assert.Equal(t, tt.undetermined, strings.Contains(out.NextAction, "could not be determined"))
		})
	}
}

func TestVerifyExpectedIdentityMissingIsUnverifiedAttention(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	out := verifyFacts(t, f, planJSON("1.16.1", f.cleanEntries(false), ""))
	assert.Equal(t, 1, out.ObjectIdentity[identityUnverified])
	require.Len(t, out.Attention, 1)
	assert.Equal(t, changeNone, out.Attention[0].Change)
	assert.Equal(t, identityUnverified, out.Attention[0].ObjectIdentity)
	assert.Equal(t, "provider_returned_identity_unavailable", out.Attention[0].IdentityReason)
	assert.True(t, strings.HasPrefix(out.NextAction, "1 selected identities are unverified for another reason"), out.NextAction)
}

func TestVerifyIdentityMismatchStops(t *testing.T) {
	f := newVerifyFixture(t, 2, importIdentitySupported, "1.16.1")
	entries := f.cleanEntries(true)
	entries[1].afterIdentity = `{"account_id":"123456789012","name":"someone-else"}`
	out := verifyFacts(t, f, planJSON("1.16.1", entries, ""))
	assert.Equal(t, 1, out.ObjectIdentity[identityMismatched])
	assert.Equal(t, 1, out.ObjectIdentity[identityMatched])
	require.Len(t, out.Attention, 1)
	assert.Equal(t, f.bindings[1].TargetAddress, out.Attention[0].TargetAddress)
	assert.Contains(t, out.NextAction, "Stop and review this selection with the user")
	assert.True(t, strings.HasPrefix(out.NextAction, "1 selected items have an identity that differs"), out.NextAction)
	assert.NotContains(t, out.NextAction, importClosingRule, "a conflicting identity is not closed out")
}

func TestVerifyChangeClassification(t *testing.T) {
	f := newVerifyFixture(t, 6, importIdentitySupported, "1.16.1")
	e := f.cleanEntries(true)
	e[0].actions, e[0].after = []string{"update"}, `{"name":"x","path":"/changed/"}`
	e[1].actions = []string{"delete", "create"}
	e[2].actions = []string{"create"}
	e[3].importing, e[3].afterIdentity = "null", ""
	e = e[:5]
	e[4].typ = "aws_iam_policy"
	// e[5] (not in plan) is dropped
	out := verifyFacts(t, f, planJSON("1.16.1", e, ""))
	assert.Equal(t, map[string]int{changeNone: 0, changeUpdate: 1, changeReplace: 1, changeCreate: 1, changeMissing: 1, changeNotInPlan: 1, changeType: 1}, out.Changes)
	require.NotEmpty(t, out.Attention)
	assert.Equal(t, []string{"path"}, out.Attention[0].ChangedPaths, "names only")
	for _, a := range out.Attention {
		assert.NotEmpty(t, a.Change)
		assert.NotEmpty(t, a.ObjectIdentity, "every attention entry carries both axes")
	}
	assert.Contains(t, out.NextAction, "not taking effect")
	assert.Contains(t, out.NextAction, "2 selected items with updates or replacements")
}

func TestVerifyUnselectedBoundedAndCounted(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	e := f.cleanEntries(true)
	for i := 0; i < 60; i++ {
		e = append(e, planEntry{address: fmt.Sprintf("aws_iam_role.extra%d", i)})
	}
	e = append(e, planEntry{address: "aws_s3_bucket.other", typ: "aws_s3_bucket", actions: []string{"update"}, importing: "null"})
	drift := `,"resource_drift":[{"address":"aws_iam_role.drifted","change":{"actions":["update"]}}]`
	out := verifyFacts(t, f, planJSON("1.16.1", e, drift))
	require.NotNil(t, out.Unselected)
	assert.Equal(t, 60, out.Unselected.ExtraImports)
	assert.Equal(t, 1, out.Unselected.OtherManagedAction)
	assert.Equal(t, 1, out.Unselected.Drift)
	assert.Len(t, out.Unselected.Addresses, maxVerifyUnselected)
	assert.True(t, out.Unselected.Truncated)
	assert.Contains(t, out.NextAction, "extra imports 60, other managed actions 1 outside the selection (counts may overlap)")
	assert.Contains(t, out.NextAction, "refresh drift 1 (may include selected addresses)")
}

func TestVerifyDeferredAndOutputChanges(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	out := verifyFacts(t, f, planJSON("1.16.1", f.cleanEntries(true), `,"deferred_changes":[{}]`))
	assert.Equal(t, 1, out.Plan.DeferredChanges)
	assert.Contains(t, out.NextAction, "1 deferred changes")
	assert.NotContains(t, out.NextAction, "no changes and no other actions")
	plan := strings.Replace(string(planJSON("1.16.1", f.cleanEntries(true), "")), `"output_changes":{}`, `"output_changes":{"x":{}}`, 1)
	assert.Contains(t, verifyFacts(t, f, []byte(plan)).NextAction, "1 output changes")
}

func TestVerifyOutputChangesCountOnlyActionable(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	base := string(planJSON("1.16.1", f.cleanEntries(true), ""))
	with := func(outputs string) importVerified {
		return verifyFacts(t, f, []byte(strings.Replace(base, `"output_changes":{}`, `"output_changes":`+outputs, 1)))
	}
	noop := with(`{"a":{"actions":["no-op"]},"b":{"actions":["no-op"]}}`)
	assert.Equal(t, 0, noop.Plan.OutputChanges)
	assert.Contains(t, noop.NextAction, "no changes and no other actions")

	mixed := with(`{"a":{"actions":["no-op"]},"b":{"actions":["update"]}}`)
	assert.Equal(t, 1, mixed.Plan.OutputChanges)
	assert.Contains(t, mixed.NextAction, "1 output changes")

	// Shape captured from `terraform show -json` on Terraform 1.17.0-beta1
	// (format_version 1.2): every output is listed, actions are top level.
	real := with(`{"changes":{"actions":["update"],"before":"one","after":"two","after_unknown":false,"before_sensitive":false,"after_sensitive":false},"stable":{"actions":["no-op"],"before":"same","after":"same","after_unknown":false,"before_sensitive":false,"after_sensitive":false}}`)
	assert.Equal(t, 1, real.Plan.OutputChanges)
	assert.Contains(t, real.NextAction, "1 output changes")
	realNoop := with(`{"stable":{"actions":["no-op"],"before":"same","after":"same","after_unknown":false,"before_sensitive":false,"after_sensitive":false}}`)
	assert.Equal(t, 0, realNoop.Plan.OutputChanges)
	assert.Contains(t, realNoop.NextAction, "no changes and no other actions")

	// The public format page draws the actions under "change".
	assert.Equal(t, 0, with(`{"a":{"change":{"actions":["no-op"]}}}`).Plan.OutputChanges)
	assert.Equal(t, 1, with(`{"a":{"change":{"actions":["create"]}}}`).Plan.OutputChanges)

	for name, outputs := range map[string]string{"empty entry": `{"x":{}}`, "no actions": `{"x":{"actions":[]}}`, "not an object": `{"x":"y"}`} {
		got := with(outputs)
		assert.Equal(t, 1, got.Plan.OutputChanges, name)
		assert.Contains(t, got.NextAction, "1 output changes", name)
	}
}

func TestVerifyDetailReturnsNamesNotValues(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	e := f.cleanEntries(true)
	e[0].actions, e[0].after = []string{"update"}, `{"name":"x","path":"/SECRET-VALUE/"}`
	out := verifyFacts(t, f, planJSON("1.16.1", e, ""), f.bindings[0].TargetAddress)
	require.Len(t, out.Detail, 1)
	assert.Equal(t, []string{"path"}, out.Detail[0].ChangedPaths)
	assert.True(t, out.Detail[0].ImportIDPresent)
	encoded, _ := json.Marshal(out)
	assert.NotContains(t, string(encoded), "SECRET-VALUE")
}

func TestVerifyIncompletePlanFailsClosed(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	for name, plan := range map[string]string{
		"not complete": strings.Replace(string(planJSON("1.16.1", f.cleanEntries(true), "")), `"complete":true`, `"complete":false`, 1),
		"errored":      strings.Replace(string(planJSON("1.16.1", f.cleanEntries(true), "")), `"errored":false`, `"errored":true`, 1),
		"bad format":   strings.Replace(string(planJSON("1.16.1", f.cleanEntries(true), "")), `"1.2"`, `"2.0"`, 1),
		"duplicate":    string(planJSON("1.16.1", append(f.cleanEntries(true), f.cleanEntries(true)...), "")),
	} {
		_, err := verifyImportPlanFacts([]byte(plan), f.carry, f.byCand, nil)
		assert.Error(t, err, name)
	}
}

func TestValidateImportCarry(t *testing.T) {
	f := newVerifyFixture(t, 2, importIdentitySupported, "1.16.1")
	_, err := validateImportCarry(f.carry, f.bindings)
	require.NoError(t, err)

	clone := func() *importCarryBlock {
		raw, _ := json.Marshal(f.carry)
		var c importCarryBlock
		require.NoError(t, json.Unmarshal(raw, &c))
		return &c
	}
	t.Run("changed identity breaks digest", func(t *testing.T) {
		c := clone()
		c.Candidates[0].Identity["name"] = "tampered"
		_, err := validateImportCarry(c, f.bindings)
		assert.ErrorContains(t, err, "carry_digest_mismatch")
	})
	t.Run("changed destination breaks digest", func(t *testing.T) {
		c := clone()
		c.Target.IdentitySupport["aws_iam_role"] = importIdentityNone
		_, err := validateImportCarry(c, f.bindings)
		assert.ErrorContains(t, err, "carry_digest_mismatch")
	})
	t.Run("candidate id must match its facts even with a fresh digest", func(t *testing.T) {
		c := clone()
		c.Candidates[0].Identity["name"] = "other"
		c.SelectionDigest = importCarryDigest(*c)
		_, err := validateImportCarry(c, f.bindings)
		assert.ErrorContains(t, err, "carry_candidate_id_mismatch")
	})
	t.Run("bindings", func(t *testing.T) {
		_, err := validateImportCarry(f.carry, f.bindings[:1])
		assert.ErrorContains(t, err, "bindings_incomplete")
		dup := []importVerifyBinding{f.bindings[0], {CandidateID: f.bindings[1].CandidateID, TargetAddress: f.bindings[0].TargetAddress}}
		_, err = validateImportCarry(f.carry, dup)
		assert.ErrorContains(t, err, "bindings_invalid")
		unknown := []importVerifyBinding{f.bindings[0], {CandidateID: "candidate-x", TargetAddress: "aws_iam_role.z"}}
		_, err = validateImportCarry(f.carry, unknown)
		assert.ErrorContains(t, err, "bindings_invalid")
	})
	t.Run("missing carry", func(t *testing.T) {
		_, err := validateImportCarry(nil, f.bindings)
		assert.ErrorContains(t, err, "carry_invalid")
	})
	t.Run("a versionless carry is rejected even with a recomputed digest", func(t *testing.T) {
		c := clone()
		c.Candidates[1].SearchIdentityVersion = nil
		c.SelectionDigest = importCarryDigest(*c)
		_, err := validateImportCarry(c, f.bindings)
		assert.ErrorContains(t, err, "carry_identity_version_invalid")
	})
	t.Run("a negative carried version is rejected even with a recomputed digest", func(t *testing.T) {
		c := clone()
		c.Candidates[0].SearchIdentityVersion = intPtr(-1)
		c.SelectionDigest = importCarryDigest(*c)
		_, err := validateImportCarry(c, f.bindings)
		assert.ErrorContains(t, err, "carry_identity_version_invalid")
	})
	t.Run("version zero is valid and the digest covers the version", func(t *testing.T) {
		c := clone()
		require.NotNil(t, c.Candidates[0].SearchIdentityVersion)
		assert.Equal(t, 0, *c.Candidates[0].SearchIdentityVersion)
		_, err := validateImportCarry(c, f.bindings)
		assert.NoError(t, err)
		c.Candidates[0].SearchIdentityVersion = intPtr(1)
		_, err = validateImportCarry(c, f.bindings)
		assert.ErrorContains(t, err, "carry_digest_mismatch", "a changed version without a fresh digest is tampering")
		c.SelectionDigest = importCarryDigest(*c)
		_, err = validateImportCarry(c, f.bindings)
		assert.NoError(t, err, "candidate_id does not include the version")
	})
}

func TestVerifyResponseSizesAtScale(t *testing.T) {
	for _, n := range []int{10, 100} {
		f := newVerifyFixture(t, n, importIdentitySupported, "1.16.1")
		clean, _ := json.Marshal(verifyFacts(t, f, planJSON("1.16.1", f.cleanEntries(true), "")))
		e := f.cleanEntries(true)
		for i := range e {
			if i%2 == 0 {
				e[i].actions, e[i].after = []string{"update"}, `{"name":"x","path":"/y/"}`
			}
		}
		mixed, _ := json.Marshal(verifyFacts(t, f, planJSON("1.16.1", e, "")))
		t.Logf("verify_import_plan, %d selected: all clean %d bytes, half need changes %d bytes", n, len(clean), len(mixed))
		assert.Less(t, len(clean), 4*1024, "a clean result stays small regardless of size")
		assert.Less(t, len(mixed), 64*1024)
	}
}

func TestVerifyImportPlanHandlerPollsThenSummarizes(t *testing.T) {
	oldWait, oldInterval := importVerifyWait, importVerifyInterval
	importVerifyWait, importVerifyInterval = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { importVerifyWait, importVerifyInterval = oldWait, oldInterval })

	f, _, finished := importExecutionFixture(t)
	prepared := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", prepared.Status, prepared.Diagnostics)
	carry := prepared.Carry
	f.mu.Lock()
	logReads, queryReads := f.requests["GET /logs"], f.requests["GET /api/v2/queries/qry-fixture"]
	f.mu.Unlock()

	inner := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import/json-output" {
			encoded, _ := json.Marshal(carry.Candidates[0].Identity)
			_, _ = io.WriteString(w, string(planJSON("1.16.1", []planEntry{{address: "aws_iam_role.selected", afterIdentity: string(encoded), identityVersion: intPtr(0)}}, "")))
			return true
		}
		return inner(w, r)
	}

	call := func(extra map[string]any) importVerified {
		args := map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "run_id": "run-import", "configuration_version_id": "cv-import"}
		for k, v := range extra {
			args[k] = v
		}
		res, err := HandleVerifyImportPlan(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}, silentLogger())
		require.NoError(t, err)
		out, ok := res.StructuredContent.(importVerified)
		require.True(t, ok)
		return out
	}

	pending := call(nil)
	assert.Equal(t, "pending", pending.Status)
	assert.Contains(t, pending.NextAction, "same run_id")
	assert.Equal(t, "ws-fixture", pending.WorkspaceID)
	assert.Equal(t, "cv-import", pending.ConfigurationVersionID)
	assert.Equal(t, "plan-import", pending.PlanID)

	*finished = true
	needCarry := call(nil)
	assert.Equal(t, "carry_required", needCarry.Status)
	assert.Equal(t, "plan-import", needCarry.PlanID)
	assert.Contains(t, needCarry.NextAction, "carry block you kept from prepare_import")
	assert.NotContains(t, strings.ToLower(needCarry.NextAction), "prepare_import again")

	rawCarry, _ := json.Marshal(carry)
	var carryArg map[string]any
	require.NoError(t, json.Unmarshal(rawCarry, &carryArg))
	done := call(map[string]any{"carry": carryArg, "bindings": []any{map[string]any{"candidate_id": carry.Candidates[0].CandidateID, "target_address": "aws_iam_role.selected"}}})
	require.Equal(t, "plan_available", done.Status, done.Diagnostics)
	assert.Equal(t, "plan-import", done.PlanID)
	assert.Equal(t, "cv-import", done.ConfigurationVersionID)
	assert.Equal(t, 1, done.ObjectIdentity[identityMatched])

	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, logReads, f.requests["GET /logs"], "verify never reads the QueryRun log")
	assert.Equal(t, queryReads, f.requests["GET /api/v2/queries/qry-fixture"])
}

func TestVerifyExpectedCVAndTerminalNoPlanResponse(t *testing.T) {
	oldWait, oldInterval := importVerifyWait, importVerifyInterval
	importVerifyWait, importVerifyInterval = 5*time.Millisecond, time.Millisecond
	t.Cleanup(func() { importVerifyWait, importVerifyInterval = oldWait, oldInterval })
	for _, tc := range []struct {
		name, runStatus, expectedCV, planID, wantStatus, wantCode string
	}{
		{"wrong CV in same target", "planned_and_finished", "cv-other", "plan-import", "blocked", "run_configuration_version_mismatch"},
		{"run failed without plan", "errored", "cv-import", "", "failed", "speculative_plan_failed"},
		{"run canceled without plan", "canceled", "cv-import", "", "failed", "speculative_plan_failed"},
		{"terminal without plan evidence", "planned_and_finished", "cv-import", "", "blocked", "plan_evidence_unavailable"},
		{"pending without plan", "pending", "cv-import", "", "pending", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _ := importExecutionFixture(t)
			inner := f.mutationHandler
			f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v2/runs/run-import" {
					plan := "null"
					if tc.planID != "" {
						plan = `{"id":"plan-import","type":"plans"}`
					}
					_, _ = fmt.Fprintf(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":%q,"plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":%s}}}}`, tc.runStatus, plan)
					return true
				}
				return inner(w, r)
			}
			args := map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "run_id": "run-import", "configuration_version_id": tc.expectedCV}
			res, err := HandleVerifyImportPlan(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}, silentLogger())
			require.NoError(t, err)
			out := res.StructuredContent.(importVerified)
			assert.Equal(t, tc.wantStatus, out.Status, out)
			assert.Equal(t, tc.wantStatus == "blocked" || tc.wantStatus == "failed", res.IsError)
			if tc.wantCode != "" {
				assert.Contains(t, out.Diagnostics, tc.wantCode)
			}
			assert.Equal(t, "ws-fixture", out.WorkspaceID)
			assert.Equal(t, "cv-import", out.ConfigurationVersionID, "observed CV, not caller expectation")
			assert.Equal(t, "run-import", out.RunID)
			assert.Equal(t, tc.planID, out.PlanID)
			text, ok := mcp.AsTextContent(res.Content[0])
			require.True(t, ok)
			assert.JSONEq(t, stringMustMarshal(t, res.StructuredContent), text.Text)
			f.mu.Lock()
			if tc.wantStatus != "pending" {
				assert.Zero(t, f.requests["GET /api/v2/plans/plan-import/json-output"])
			}
			if tc.wantCode == "run_configuration_version_mismatch" {
				assert.Zero(t, f.requests["GET /api/v2/plans/plan-import"], "wrong CV blocks before plan metadata")
			}
			f.mu.Unlock()
		})
	}
}

func TestVerifyRejectsPlanReadWithDifferentIDBeforeJSON(t *testing.T) {
	f, _, finished := importExecutionFixture(t)
	*finished = true
	inner := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v2/plans/plan-import" {
			_, _ = io.WriteString(w, `{"data":{"id":"plan-other","type":"plans","attributes":{"status":"finished"}}}`)
			return true
		}
		return inner(w, r)
	}
	out := verifyImportPlan(context.Background(), f.client, importVerifyArgs{Organization: "fixture-org", Workspace: "import-root", RunID: "run-import", ConfigurationVersionID: "cv-import"})
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "plan_association_unverified")
	assert.Equal(t, "plan-import", out.PlanID, "only the Run relationship is handed off")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["GET /api/v2/plans/plan-import/json-output"])
}

func TestVerifyImportPlanHandlerRejectsTamperedCarryBeforeReadingPlan(t *testing.T) {
	f, _, finished := importExecutionFixture(t)
	*finished = true
	prepared := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", prepared.Status)
	prepared.Carry.Candidates[0].Identity["name"] = "tampered"
	rawCarry, _ := json.Marshal(prepared.Carry)
	var carryArg map[string]any
	require.NoError(t, json.Unmarshal(rawCarry, &carryArg))
	res, err := HandleVerifyImportPlan(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"organization_name": "fixture-org", "workspace_name": "import-root", "run_id": "run-import", "configuration_version_id": "cv-import", "carry": carryArg,
		"bindings": []any{map[string]any{"candidate_id": prepared.Carry.Candidates[0].CandidateID, "target_address": "aws_iam_role.selected"}},
	}}}, silentLogger())
	require.NoError(t, err)
	assert.True(t, res.IsError)
	out := res.StructuredContent.(importVerified)
	assert.Contains(t, out.Diagnostics, "carry_digest_mismatch")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["GET /api/v2/plans/plan-import/json-output"], "the plan is not read when the carry is inconsistent")
}
