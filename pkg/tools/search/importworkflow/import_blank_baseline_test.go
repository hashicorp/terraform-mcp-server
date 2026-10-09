// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mutateBlankWorkspace edits the workspace document served on the
// organization/name path and the ID path, or on the ID path only.
func mutateBlankWorkspace(t *testing.T, f *importBackendTest, edit func(attrs, rels map[string]any), idOnly bool) {
	t.Helper()
	paths := []string{"/api/v2/organizations/fixture-org/workspaces/import-root", "/api/v2/workspaces/ws-fixture"}
	if idOnly {
		paths = paths[1:]
	}
	for _, path := range paths {
		var document map[string]any
		require.NoError(t, json.Unmarshal(f.responses[path], &document))
		data := document["data"].(map[string]any)
		edit(data["attributes"].(map[string]any), data["relationships"].(map[string]any))
		var err error
		f.responses[path], err = json.Marshal(document)
		require.NoError(t, err)
	}
}

type blankBaselineCase struct {
	name, code string
	edit       func(attrs, rels map[string]any)
	// state200 makes the current-state endpoint readable.
	state200 bool
	// idOnly applies the edit only to the by-ID workspace read, as if the
	// workspace changed after the first read.
	idOnly bool
}

func setStateRelationship(value any) func(attrs, rels map[string]any) {
	return func(_, rels map[string]any) {
		if value == "omit" {
			delete(rels, "current-state-version")
			return
		}
		rels["current-state-version"] = value
	}
}

// Workspace/state evidence combinations that must never be treated as blank,
// even though the current-state endpoint answers 404 (unless state200).
var blankBaselineCases = []blankBaselineCase{
	{name: "state present while current-state endpoint says 404", code: "blank_workspace_has_state",
		edit: setStateRelationship(map[string]any{"data": map[string]any{"type": "state-versions", "id": "sv-hidden"}})},
	{name: "state read permission false", code: "evidence_access_denied",
		edit: func(attrs, _ map[string]any) { attrs["permissions"] = map[string]any{"can-read-state-versions": false} }},
	{name: "state read permission false and relationship omitted", code: "evidence_access_denied",
		edit: func(attrs, rels map[string]any) {
			attrs["permissions"] = map[string]any{"can-read-state-versions": false}
			delete(rels, "current-state-version")
		}},
	{name: "state read permission missing", code: "backend_evidence_unavailable",
		edit: func(attrs, _ map[string]any) { delete(attrs, "permissions") }},
	{name: "state read permission empty", code: "backend_evidence_unavailable",
		edit: func(attrs, _ map[string]any) { attrs["permissions"] = map[string]any{} }},
	{name: "relationship omitted", code: "backend_evidence_unavailable", edit: setStateRelationship("omit")},
	{name: "relationship without data", code: "backend_evidence_unavailable", edit: setStateRelationship(map[string]any{})},
	{name: "relationship malformed", code: "evidence_json_invalid", idOnly: true, edit: setStateRelationship("oops")},
	{name: "relationship data malformed", code: "evidence_json_invalid", idOnly: true, edit: setStateRelationship(map[string]any{"data": "sv-1"})},
	{name: "relationship data without id", code: "evidence_json_invalid", idOnly: true,
		edit: setStateRelationship(map[string]any{"data": map[string]any{"type": "state-versions"}})},
	{name: "current configuration relationship omitted", code: "blank_workspace_baseline_changed_or_unsupported", idOnly: true,
		edit: func(_, rels map[string]any) { delete(rels, "current-configuration-version") }},
	{name: "null relationship conflicts with readable state", code: "blank_workspace_has_state", edit: func(_, _ map[string]any) {}, state200: true},
	{name: "current configuration appears", code: "blank_workspace_baseline_changed_or_unsupported", idOnly: true,
		edit: func(_, rels map[string]any) {
			rels["current-configuration-version"] = map[string]any{"data": map[string]any{"type": "configuration-versions", "id": "cv-late"}}
		}},
	{name: "organization mismatch", code: "blank_workspace_baseline_changed_or_unsupported", idOnly: true,
		edit: func(_, rels map[string]any) {
			rels["organization"] = map[string]any{"data": map[string]any{"type": "organizations", "id": "other-org"}}
		}},
	{name: "unsupported setting", code: "blank_workspace_baseline_changed_or_unsupported", idOnly: true,
		edit: func(attrs, _ map[string]any) { attrs["working-directory"] = "sub" }},
}

func (tc blankBaselineCase) apply(t *testing.T, f *importBackendTest) {
	t.Helper()
	mutateBlankWorkspace(t, f, tc.edit, tc.idOnly)
	if tc.state200 {
		f.stateStatus = http.StatusOK
	}
}

func blankCreateArgs(run bool) map[string]any {
	args := map[string]any{"baseline_cv_id": nil, "baseline_state_id": nil, "baseline_state_serial": nil}
	if run {
		args["configuration_version_id"] = "cv-import"
	}
	return args
}

func TestBlankBaselineAuthorizedAcceptedByAllEntryPoints(t *testing.T) {
	f, uploaded, _ := blankImportFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "ready_for_authoring", out.Status, out.Diagnostics)
	dl := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root", PreparedTargetID: "ws-fixture"}, "cv-current", silentLogger())
	assert.Equal(t, "blank_workspace", dl.Status, dl.Diagnostics)
	cv := callCreate(t, false, createArgs(blankCreateArgs(false)))
	require.Equal(t, "awaiting_agent_upload", cv.Status, cv.Diagnostics)
	*uploaded = true
	run := callCreate(t, true, createArgs(blankCreateArgs(true)))
	require.Equal(t, "pending", run.Status, run.Diagnostics)
}

func TestBlankBaselineUnestablishedEvidenceFailsClosed(t *testing.T) {
	for _, tc := range blankBaselineCases {
		// With a readable current state and no current configuration, prepare
		// treats the workspace as an ordinary stateful target, not a blank one.
		if !tc.state200 {
			t.Run("prepare "+tc.name, func(t *testing.T) {
				f, _, _ := blankImportFixture(t)
				tc.apply(t, f)
				out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
				assert.Equal(t, "blocked", out.Status)
				// QueryRun provenance reads the same workspace by ID first;
				// malformed relationships can fail SDK decoding before blank preflight.
				code := tc.code
				if tc.name == "relationship malformed" || tc.name == "relationship data malformed" {
					code = "backend_evidence_unavailable"
				}
				if tc.name == "organization mismatch" {
					code = "query_organization_mismatch"
				}
				assert.Contains(t, out.Diagnostics, code)
				assert.Nil(t, out.Carry)
			})
		}
		t.Run("download "+tc.name, func(t *testing.T) {
			f, _, _ := blankImportFixture(t)
			tc.apply(t, f)
			out := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root", PreparedTargetID: "ws-fixture"}, "cv-current", silentLogger())
			assert.Equal(t, "blocked", out.Status)
			assert.Empty(t, out.DownloadURL)
		})
		for _, run := range []bool{false, true} {
			name := "create cv "
			if run {
				name = "create run "
			}
			t.Run(name+tc.name, func(t *testing.T) {
				f, uploaded, _ := blankImportFixture(t)
				*uploaded = true
				tc.apply(t, f)
				out := callCreate(t, run, createArgs(blankCreateArgs(run)))
				assert.Equal(t, "blocked", out.Status)
				assert.Contains(t, out.Diagnostics, tc.code)
				f.mu.Lock()
				defer f.mu.Unlock()
				assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
				assert.Zero(t, f.requests["POST /api/v2/runs"])
			})
		}
	}
}

// Evidence that changes between prepare and a create call is caught by the
// create call's own read. This is a sequential recheck, not an atomic guarantee.
func TestBlankBaselineChangedBeforeCreatesIsRefused(t *testing.T) {
	for _, run := range []bool{false, true} {
		f, uploaded, _ := blankImportFixture(t)
		*uploaded = true
		out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
		require.Equal(t, "ready_for_authoring", out.Status, out.Diagnostics)
		mutateBlankWorkspace(t, f, setStateRelationship(map[string]any{"data": map[string]any{"type": "state-versions", "id": "sv-new"}}), false)
		created := callCreate(t, run, createArgs(blankCreateArgs(run)))
		assert.Equal(t, "blocked", created.Status)
		assert.Contains(t, created.Diagnostics, "blank_workspace_has_state")
		f.mu.Lock()
		assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
		assert.Zero(t, f.requests["POST /api/v2/runs"])
		f.mu.Unlock()
	}
}

func TestBlankBaselineRejectsConflictingBackendEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, original, duplicate string
	}{
		{"duplicate state data", `"current-state-version":{"data":null}`, `"current-state-version":{"data":{"id":"sv-existing"},"data":null}`},
		{"duplicate permission", `"can-read-state-versions":true`, `"can-read-state-versions":false,"can-read-state-versions":true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _ := blankImportFixture(t)
			path := "/api/v2/workspaces/ws-fixture"
			original := f.responses[path]
			changed := bytes.Replace(original, []byte(tc.original), []byte(tc.duplicate), 1)
			require.NotEqual(t, string(original), string(changed))
			f.responses[path] = changed
			out := callCreate(t, false, createArgs(blankCreateArgs(false)))
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, "evidence_json_invalid")
			f.mu.Lock()
			assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
			f.mu.Unlock()
		})
	}
}

func TestBlankBaselineCorroboratingStateDenial(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f, _, _ := blankImportFixture(t)
			f.stateStatus = status
			out := callCreate(t, false, createArgs(blankCreateArgs(false)))
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, "evidence_access_denied")
			f.mu.Lock()
			assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
			f.mu.Unlock()
		})
	}
}
