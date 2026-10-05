// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createArgs(extra map[string]any) map[string]any {
	args := map[string]any{
		"organization_name": "fixture-org", "workspace_name": "import-root",
		"baseline_cv_id": "cv-current", "baseline_state_id": "sv-current", "baseline_state_serial": float64(42),
		"confirm_speculative_run": true,
	}
	for k, v := range extra {
		if v == nil {
			delete(args, k)
		} else {
			args[k] = v
		}
	}
	return args
}

func callCreate(t *testing.T, run bool, args map[string]any) importCreated {
	t.Helper()
	handler := HandleCreateImportCV
	if run {
		handler = HandleCreateImportRun
	}
	res, err := handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}, silentLogger())
	require.NoError(t, err)
	out, ok := res.StructuredContent.(importCreated)
	require.True(t, ok)
	assert.Equal(t, res.IsError, out.Status == "blocked")
	return out
}

func TestCreateImportToolsPlanOnlyFlowWithoutQueryRunReads(t *testing.T) {
	f, uploaded, _ := importExecutionFixture(t)
	cv := callCreate(t, false, createArgs(nil))
	require.Equal(t, "awaiting_agent_upload", cv.Status, cv.Diagnostics)
	assert.Equal(t, "cv-import", cv.ConfigurationVersionID)
	assert.Equal(t, f.url+"/upload", cv.UploadURL)
	assert.NotContains(t, cv.NextAction, cv.UploadURL)

	// Not uploaded yet: the run is refused before any POST.
	blocked := callCreate(t, true, createArgs(map[string]any{"configuration_version_id": "cv-import"}))
	assert.Equal(t, "blocked", blocked.Status)
	assert.Contains(t, blocked.Diagnostics, "execution_cv_not_uploaded")
	f.mu.Lock()
	assert.Zero(t, f.requests["POST /api/v2/runs"])
	f.mu.Unlock()

	*uploaded = true
	run := callCreate(t, true, createArgs(map[string]any{"configuration_version_id": "cv-import"}))
	require.Equal(t, "pending", run.Status, run.Diagnostics)
	assert.Equal(t, "run-import", run.RunID)
	assert.Contains(t, run.NextAction, "verify_import_plan")

	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["POST /api/v2/runs"])
	assert.Zero(t, f.requests["GET /logs"], "creates never read the QueryRun log")
	assert.Zero(t, f.requests["GET /api/v2/queries/qry-fixture"])
	assert.Zero(t, f.requests["GET /api/v2/runs/run-schema/plan/json-schema"], "creates read no schema")
}

func TestCreateImportToolsRefuseBeforePOST(t *testing.T) {
	cases := []struct {
		name  string
		run   bool
		extra map[string]any
		code  string
	}{
		{"cv needs confirmation", false, map[string]any{"confirm_speculative_run": false}, "confirm_speculative_run_required"},
		{"run needs confirmation", true, map[string]any{"confirm_speculative_run": false, "configuration_version_id": "cv-import"}, "confirm_speculative_run_required"},
		{"partial baseline", false, map[string]any{"baseline_state_id": nil}, "import_input_invalid"},
		{"cv call rejects a CV ID", false, map[string]any{"configuration_version_id": "cv-import"}, "import_input_invalid"},
		{"run needs CV", true, nil, "import_input_invalid"},
		{"changed config baseline", false, map[string]any{"baseline_cv_id": "cv-other"}, "baseline_changed"},
		{"changed state baseline", false, map[string]any{"baseline_state_id": "sv-other"}, "baseline_changed"},
		{"changed serial", false, map[string]any{"baseline_state_serial": float64(41)}, "baseline_changed"},
		{"blank claim on non-blank workspace", false, map[string]any{"baseline_cv_id": nil, "baseline_state_id": nil, "baseline_state_serial": nil}, "blank_workspace_baseline_changed_or_unsupported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, uploaded, _ := importExecutionFixture(t)
			*uploaded = true
			out := callCreate(t, tc.run, createArgs(tc.extra))
			assert.Equal(t, "blocked", out.Status)
			assert.Contains(t, out.Diagnostics, tc.code)
			assert.Contains(t, out.NextAction, "No CV or Run was created by this request.")
			f.mu.Lock()
			defer f.mu.Unlock()
			assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
			assert.Zero(t, f.requests["POST /api/v2/runs"])
		})
	}
}

func TestCreateImportRunRejectsUnknownConfigurationVersion(t *testing.T) {
	f, uploaded, _ := importExecutionFixture(t)
	*uploaded = true
	out := callCreate(t, true, createArgs(map[string]any{"configuration_version_id": "cv-not-in-workspace"}))
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "execution_cv_workspace_mismatch")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func TestCreateImportToolDefinitionsAreStrictMutations(t *testing.T) {
	for _, tool := range []mcp.Tool{CreateImportCVDefinition(), CreateImportRunDefinition()} {
		require.NotNil(t, tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Annotations.ReadOnlyHint)
		assert.Contains(t, tool.InputSchema.Required, "confirm_speculative_run")
		assert.NotContains(t, tool.InputSchema.Properties, "query_run_id")
		assert.NotContains(t, tool.InputSchema.Properties, "selections")
		assert.True(t, strings.Contains(tool.Description, "confirmation"))
	}
	assert.Contains(t, CreateImportRunDefinition().InputSchema.Required, "configuration_version_id")
	assert.NotContains(t, CreateImportCVDefinition().InputSchema.Properties, "configuration_version_id")
}

func TestCreateImportCVRejectsKnownTerraformVersionBelow1_5(t *testing.T) {
	f, uploaded, _ := importExecutionFixture(t)
	setWorkspaceAttribute(t, f, "terraform-version", "1.4.7")
	out := callCreate(t, false, createArgs(nil))
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "terraform_version_unsupported")
	assert.False(t, *uploaded)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/runs"])
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
}
