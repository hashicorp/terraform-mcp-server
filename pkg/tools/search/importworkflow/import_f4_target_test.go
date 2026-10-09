// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Move only the QueryRun/no-code-query to a distinct source workspace. All
// target state, schema, CV and Run responses stay attached to ws-fixture.
func separateImportSource(t *testing.T, f *importBackendTest, org string) {
	t.Helper()
	f.responses["/api/v2/workspaces/ws-search"] = json.RawMessage(`{"data":{"type":"workspaces","id":"ws-search","attributes":{"name":"search-root"},"relationships":{"organization":{"data":{"type":"organizations","id":"` + org + `"}}}}}`)
	for _, path := range []string{"/api/v2/queries/qry-fixture", "/api/v2/search/no-code-query/ncqry-fixture"} {
		original := string(f.responses[path])
		changed := strings.ReplaceAll(original, `"id": "ws-fixture"`, `"id": "ws-search"`)
		require.NotEqual(t, original, changed)
		f.responses[path] = []byte(changed)
	}
}

func TestF4SeparateSearchAndTargetWorkspace(t *testing.T) {
	for _, route := range []string{"schema", "blank", "agent_schema"} {
		t.Run(route, func(t *testing.T) {
			var f *importBackendTest
			switch route {
			case "blank":
				f, _, _ = blankImportFixture(t)
			case "agent_schema":
				f = schemaFallbackFixture(t, "uploaded")
			default:
				f = importBackendFixture(t)
			}
			separateImportSource(t, f, "fixture-org")
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			require.NotEqual(t, "blocked", out.Status, out.Diagnostics)
			assert.Equal(t, "ws-search", out.SourceWorkspaceID)
			assert.Equal(t, "search-root", out.SourceWorkspaceName)
			assert.Equal(t, "fixture-org", out.SourceOrganizationName)
			assert.Equal(t, "ws-fixture", out.WorkspaceID)
			assert.Equal(t, "import-root", out.TargetWorkspaceName)
			require.NotNil(t, out.Carry)
			assert.Equal(t, "ws-fixture", out.Carry.Target.WorkspaceID)
			assert.Equal(t, importCarryDigest(*out.Carry), out.Carry.SelectionDigest)
		})
	}
}

func TestF4DiscoveryPageShowsBackendSourceWithoutTarget(t *testing.T) {
	f := importBackendFixture(t)
	separateImportSource(t, f, "fixture-org")
	result, err := ReadDiscoveryPage(context.Background(), f.client, "qry-fixture", DiscoveryFilter{})
	require.NoError(t, err)
	page := result.(*discoveryPage)
	assert.Equal(t, "fixture-org", page.SourceOrganizationName)
	assert.Equal(t, "search-root", page.SourceWorkspaceName)
	assert.Equal(t, "ws-search", page.SourceWorkspaceID)
	assert.NotEmpty(t, page.Lists[0].Candidates[0].CandidateID)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"target_workspace_id"`)
}

func TestF4Route2RejectsStateOrProducingCVChangedDuringLog(t *testing.T) {
	for _, change := range []string{"state", "run_cv"} {
		t.Run(change, func(t *testing.T) {
			f := schemaFallbackFixture(t, "uploaded")
			f.mutationHandler = func(_ http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/logs" {
					if change == "state" {
						f.stateChanged = true
					} else {
						f.responses["/api/v2/runs/run-schema"] = []byte(strings.ReplaceAll(string(f.responses["/api/v2/runs/run-schema"]), `"cv-run"`, `"cv-other"`))
					}
				}
				return false
			}
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, "baseline_changed")
			assert.Nil(t, out.Carry)
		})
	}
}

func TestF4RejectCrossOrganizationAndPreAuthoringRoot(t *testing.T) {
	f := importBackendFixture(t)
	separateImportSource(t, f, "different-org")
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "query_organization_mismatch")
	assert.Zero(t, logRequests(f))
	for _, route := range []string{"schema", "agent_schema"} {
		t.Run(route, func(t *testing.T) {
			f := importBackendFixture(t)
			if route == "agent_schema" {
				f = schemaFallbackFixture(t, "uploaded")
			}
			setWorkspaceAttribute(t, f, "working-directory", "root/subdir")
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, "configuration_root_setting_unsupported")
			assert.Zero(t, logRequests(f))
			assert.Nil(t, out.Carry)
		})
	}
}

func TestF4PreparedTargetIDStopsBothCreatesAndVerify(t *testing.T) {
	f, uploaded, finished := importExecutionFixture(t)
	for _, run := range []bool{false, true} {
		missing := createArgs(map[string]any{"prepared_target_workspace_id": nil, "configuration_version_id": "cv-import"})
		if !run {
			delete(missing, "configuration_version_id")
		}
		assert.Contains(t, callCreate(t, run, missing).Diagnostics, "import_input_invalid")
		args := createArgs(map[string]any{"prepared_target_workspace_id": "ws-other", "configuration_version_id": "cv-import"})
		if !run {
			delete(args, "configuration_version_id")
		}
		out := callCreate(t, run, args)
		assert.Equal(t, "blocked", out.Status)
		assert.Contains(t, out.Diagnostics, "prepared_target_workspace_mismatch")
	}
	f.mu.Lock()
	assert.Zero(t, f.requests["POST /api/v2/runs"])
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	f.mu.Unlock()
	// The verify call also rejects an internally consistent, but wrong-target,
	// carried selection before reading plan JSON.
	*uploaded, *finished = true, true
	selection := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	selection.carry.Target.WorkspaceID = "ws-other"
	selection.carry.SelectionDigest = importCarryDigest(*selection.carry)
	out := verifyImportPlan(context.Background(), f.client, importVerifyArgs{Organization: "fixture-org", Workspace: "import-root", RunID: "run-import", ConfigurationVersionID: "cv-import", Carry: selection.carry, Bindings: selection.bindings})
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "prepared_target_workspace_mismatch")
	f.mu.Lock()
	assert.Zero(t, f.requests["GET /api/v2/plans/plan-import/json-output"])
	f.mu.Unlock()
}

func TestF4TargetIDChangedDuringPreparationBlocks(t *testing.T) {
	f := importBackendFixture(t)
	f.mutationHandler = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/logs" {
			path := "/api/v2/organizations/fixture-org/workspaces/import-root"
			f.responses[path] = []byte(strings.ReplaceAll(string(f.responses[path]), `"ws-fixture"`, `"ws-other"`))
		}
		return false
	}
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "prepared_target_workspace_mismatch")
	assert.Nil(t, out.Carry)
}
