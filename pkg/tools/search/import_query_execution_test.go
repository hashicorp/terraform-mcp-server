// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func importExecutionFixture(t *testing.T) (*importBackendTest, *bool, *bool) {
	t.Helper()
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	uploaded, finished := new(bool), new(bool)
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v2/workspaces/ws-fixture/configuration-versions":
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `"speculative":true`)
			assert.Contains(t, string(body), `"auto-queue-runs":false`)
			assert.NotContains(t, string(body), `"provisional":true`)
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"data":{"id":"cv-import","type":"configuration-versions","attributes":{"status":"pending","speculative":true,"auto-queue-runs":false,"upload-url":%q}}}`, f.url+"/upload")
		case "PUT /upload":
			assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))
			assert.Empty(t, r.Header.Get("Authorization"))
			*uploaded = true
			w.WriteHeader(http.StatusOK)
		case "GET /api/v2/workspaces/ws-fixture/configuration-versions":
			assert.Equal(t, "1", r.URL.Query().Get("page[number]"))
			_, _ = io.WriteString(w, `{"data":[{"id":"cv-import","type":"configuration-versions","attributes":{"speculative":true}}],"meta":{"pagination":{"current-page":1,"next-page":0,"total-count":1,"total-pages":1}}}`)
		case "GET /api/v2/configuration-versions/cv-import":
			status := "pending"
			if *uploaded {
				status = "uploaded"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"id":"cv-import","type":"configuration-versions","attributes":{"status":%q,"speculative":true,"auto-queue-runs":false}}}`, status)
		case "POST /api/v2/runs":
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), `"plan-only":true`)
			assert.Contains(t, string(body), `"id":"cv-import"`)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":"pending","plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`)
		case "GET /api/v2/runs/run-import":
			status := "pending"
			if *finished {
				status = "planned_and_finished"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":%q,"plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`, status)
		case "GET /api/v2/plans/plan-import":
			status := "pending"
			if *finished {
				status = "finished"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"id":"plan-import","type":"plans","attributes":{"status":%q}}}`, status)
		case "GET /api/v2/plans/plan-import/json-output":
			_, _ = io.WriteString(w, `{"format_version":"1.2","variables":{"password":{"value":"MUST-NOT-LEAK"}},"resource_changes":[{"address":"aws_iam_role.selected","mode":"managed","type":"aws_iam_role","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"MUST-NOT-LEAK"}}},{"address":"aws_iam_role.unrelated","mode":"managed","change":{"actions":["update"]}}],"output_changes":{"new":{}}}`)
		default:
			return false
		}
		return true
	}
	return f, uploaded, finished
}

func callImportPhase(t *testing.T, input importPrepareInput) importPreparation {
	t.Helper()
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	var args map[string]any
	require.NoError(t, json.Unmarshal(raw, &args))
	res, err := ImportQueryResults(silentLogger(), nil).Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
	require.NoError(t, err)
	packet, ok := res.StructuredContent.(importPreparation)
	require.True(t, ok)
	assert.Equal(t, res.IsError, packet.Status == "blocked" || packet.Status == "failed")
	return packet
}

func importUploadInput(t *testing.T) importPrepareInput {
	t.Helper()
	i := importFixtureInput(t)
	i.Phase = "upload"
	i.BaselineCVID = "cv-current"
	i.BaselineStateID = "sv-current"
	i.BaselineSerial = 42
	i.ConfirmSpeculativeRun = true
	return i
}

func TestImportPhaseInputGuidanceBeforeCreates(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	upload := importUploadInput(t)
	plan := upload
	plan.Phase = "plan"
	plan.ConfigurationVersionID = "cv-import"
	plan.TargetAddress = "aws_iam_role.selected"
	cases := []struct {
		name, code, field string
		input             importPrepareInput
	}{
		{"existing upload rejects target", "speculative_upload_input_invalid", "target_address_not_allowed_on_existing_upload", func() importPrepareInput { i := upload; i.TargetAddress = plan.TargetAddress; return i }()},
		{"existing upload needs approval", "speculative_upload_input_invalid", "confirm_speculative_run_required", func() importPrepareInput { i := upload; i.ConfirmSpeculativeRun = false; return i }()},
		{"existing plan needs target", "plan_input_invalid", "target_address_required_for_import_plan", func() importPrepareInput { i := plan; i.TargetAddress = ""; return i }()},
		{"existing plan needs CV", "plan_input_invalid", "configuration_version_id_required", func() importPrepareInput { i := plan; i.ConfigurationVersionID = ""; return i }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callImportPhase(t, tc.input)
			assert.Equal(t, "blocked", result.Status)
			assert.Contains(t, result.Diagnostics, tc.code)
			assert.Contains(t, result.Diagnostics, tc.field)
			assert.Contains(t, result.NextAction, "No CV or Run was created by this request.")
			assert.Nil(t, result.Execution)
		})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func TestImportBlankPhaseInputGuidanceBeforeCreates(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	i := importFixtureInput(t)
	i.Phase = "upload"
	i.ConfirmSpeculativeRun = true
	i.TargetAddress = " invalid address\n"
	badAddress := callImportPhase(t, i)
	assert.Contains(t, badAddress.Diagnostics, "target_address_invalid")
	assert.Contains(t, badAddress.NextAction, "No CV or Run was created by this request.")
	i.TargetAddress = ""
	i.ConfirmSpeculativeRun = false
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "confirm_speculative_run_required")
	i.Phase, i.ConfirmSpeculativeRun = "plan", true
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "configuration_version_id_required")
	i.Phase, i.ConfigurationVersionID = "upload", ""
	i.SchemaCVID, i.SchemaRunID = "cv-schema", "run-schema"
	i.TargetAddress = "aws_iam_role.selected"
	schemaUpload := callImportPhase(t, i)
	assert.Contains(t, schemaUpload.Diagnostics, "target_address_not_allowed_on_schema_upload")
	assert.Contains(t, schemaUpload.NextAction, "No CV or Run was created by this request.")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func TestImportPhaseDescriptionsSeparateExistingAndBlankInputs(t *testing.T) {
	tool := ImportQueryResults(silentLogger(), nil).Tool
	assert.Contains(t, tool.Description, "OMITS target_address")
	assert.Contains(t, tool.Description, "requires that same baseline")
	assert.Contains(t, tool.Description, "provider-only schema probe")
	assert.Contains(t, tool.Description, "a browser is not required")
	assert.Contains(t, tool.Description, "binary-capable HTTP GET")
	assert.Contains(t, tool.Description, "status cannot reacquire it")
}

func TestImportStatelessCVRunAndPlanFacts(t *testing.T) {
	f, _, finished := importExecutionFixture(t)
	prepared := callImportPhase(t, importFixtureInput(t))
	assert.Contains(t, prepared.NextAction, "Before requesting the short-lived phase=context URL")
	assert.Contains(t, prepared.AgentInstructions[6], "BEFORE requesting the short-lived phase=context URL")
	require.NotNil(t, prepared.Continuation)
	assert.Contains(t, prepared.Continuation.Precondition, "Before phase=context, ask the user")
	assert.Equal(t, "cv-current", prepared.Continuation.Arguments["baseline_cv_id"])
	assert.Equal(t, "sv-current", prepared.Continuation.Arguments["baseline_state_id"])
	assert.Equal(t, int64(42), prepared.Continuation.Arguments["baseline_state_serial"])
	assert.NotContains(t, prepared.Continuation.Arguments, "target_address", "existing upload does not accept a target address")
	assert.Contains(t, prepared.Continuation.Precondition, "Existing-workspace upload must omit target_address")
	created := callImportPhase(t, importUploadInput(t))
	require.Equal(t, "awaiting_agent_upload", created.Status, created.Diagnostics)
	require.Equal(t, "cv-import", created.Execution.ConfigurationVersionID)
	assert.Contains(t, created.Execution.UploadInstructions, "Retain this one-use upload_url securely")
	assert.Contains(t, created.Execution.UploadInstructions, "status cannot reacquire it")
	require.NotNil(t, created.WorkflowContext)
	assert.Equal(t, "qry-fixture", created.WorkflowContext.QueryRunID)
	assert.Equal(t, "cv-import", created.WorkflowContext.ConfigurationVersionID)
	require.NotNil(t, created.Continuation)
	assert.Equal(t, "status", created.Continuation.NextPhase)
	assert.Equal(t, "cv-import", created.Continuation.Arguments["configuration_version_id"])
	assert.NotContains(t, fmt.Sprint(created.Continuation), "/upload")
	assert.Contains(t, created.Continuation.Precondition, "If lost, status cannot reacquire it")
	assert.NotContains(t, fmt.Sprint(created.Continuation), created.Execution.UploadURL)
	status := importPrepareInput{Phase: "status", Organization: "fixture-org", Workspace: "import-root", ConfigurationVersionID: created.Execution.ConfigurationVersionID}
	awaitingUpload := callImportPhase(t, status)
	assert.Equal(t, "awaiting_agent_upload", awaitingUpload.Status)
	assert.Contains(t, awaitingUpload.NextAction, "If lost or expired, status cannot reacquire it")
	require.NotNil(t, awaitingUpload.Continuation)
	assert.Contains(t, awaitingUpload.Continuation.Precondition, "reconcile the pending CV")
	assert.NotContains(t, fmt.Sprint(awaitingUpload.Continuation), created.Execution.UploadURL)
	plan := status
	plan.Phase = "plan"
	plan.QueryID = importUploadInput(t).QueryID
	plan.Selections = importUploadInput(t).Selections
	plan.BaselineCVID = "cv-current"
	plan.BaselineStateID = "sv-current"
	plan.BaselineSerial = 42
	plan.ConfirmSpeculativeRun = true
	plan.TargetAddress = "aws_iam_role.selected"
	assert.Contains(t, callImportPhase(t, plan).Diagnostics, "execution_cv_not_uploaded")
	req, err := http.NewRequest(http.MethodPut, created.Execution.UploadURL, strings.NewReader("agent-owned-archive"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ready_for_plan", callImportPhase(t, status).Status)
	ready := callImportPhase(t, status)
	require.NotNil(t, ready.Continuation)
	assert.Equal(t, "plan", ready.Continuation.NextPhase)
	assert.NotContains(t, ready.Continuation.Arguments, "query_run_id", "status cannot recover selection provenance from a CV")
	assert.Contains(t, ready.Continuation.RequiredInputs, "selections")
	assert.Contains(t, ready.Continuation.Precondition, "target_address (required for every import plan)")
	started := callImportPhase(t, plan)
	require.Equal(t, "pending", started.Status, started.Diagnostics)
	require.Equal(t, "run-import", started.Execution.RunID)
	require.NotNil(t, started.Continuation)
	assert.Equal(t, "run-import", started.Continuation.Arguments["run_id"])
	status.RunID = started.Execution.RunID
	status.TargetAddress = plan.TargetAddress
	assert.Equal(t, "pending", callImportPhase(t, status).Status)
	*finished = true
	done := callImportPhase(t, status)
	require.Equal(t, "plan_available_for_agent_assessment", done.Status, done.Diagnostics)
	assert.Equal(t, "aws_iam_role.selected", done.Execution.PlanFacts.Selected.Address)
	assert.Nil(t, done.Continuation)
	assert.Empty(t, done.WorkflowContext.QueryRunID, "status cannot assert query provenance from a CV")
	assert.Equal(t, "plan-import", done.WorkflowContext.PlanID)
	assert.Contains(t, done.AgentInstructions[1], "get_plan_json_output")
	assert.True(t, done.Execution.PlanFacts.Selected.ImportPresent)
	assert.Equal(t, 1, done.Execution.PlanFacts.OtherManagedActionCount)
	assert.Equal(t, 1, done.Execution.PlanFacts.OutputChangeCount)
	encoded, err := json.Marshal(done)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "MUST-NOT-LEAK")
	schemaJSON, err := json.Marshal(ImportQueryResults(silentLogger(), nil).Tool.OutputSchema)
	require.NoError(t, err)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	for _, packet := range []importPreparation{prepared, created, ready, started, done} {
		data, err := json.Marshal(packet)
		require.NoError(t, err)
		var instance any
		require.NoError(t, json.Unmarshal(data, &instance))
		require.NoError(t, resolved.Validate(instance))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Equal(t, 1, f.requests["POST /api/v2/runs"])
	assert.Zero(t, f.requests["GET /api/v2/configuration-versions/cv-import/download"])
}

func TestImportStatelessOwnershipAndBaseline(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	i := importUploadInput(t)
	i.ConfirmSpeculativeRun = false
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "speculative_upload_input_invalid")
	created := callImportPhase(t, importUploadInput(t))
	require.Equal(t, "awaiting_agent_upload", created.Status)
	input := importPrepareInput{Phase: "status", Organization: "fixture-org", Workspace: "import-root", ConfigurationVersionID: "cv-other"}
	assert.Contains(t, callImportPhase(t, input).Diagnostics, "execution_cv_workspace_mismatch")
	plan := input
	plan.ConfigurationVersionID = "cv-import"
	plan.Phase = "plan"
	plan.QueryID = importUploadInput(t).QueryID
	plan.Selections = importUploadInput(t).Selections
	plan.ConfirmSpeculativeRun = true
	plan.TargetAddress = "aws_iam_role.selected"
	plan.BaselineCVID = "cv-other"
	plan.BaselineStateID = "sv-current"
	plan.BaselineSerial = 42
	assert.Contains(t, callImportPhase(t, plan).Diagnostics, "baseline_changed")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func TestImportStatelessCVListBoundFailsClosed(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/workspaces/ws-fixture/configuration-versions" {
			page := r.URL.Query().Get("page[number]")
			_, _ = fmt.Fprintf(w, `{"data":[],"meta":{"pagination":{"current-page":%s,"next-page":%d,"total-pages":999}}}`, page, parseTestPage(page)+1)
			return true
		}
		return original(w, r)
	}
	i := importPrepareInput{Phase: "status", Organization: "fixture-org", Workspace: "import-root", ConfigurationVersionID: "cv-import"}
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "execution_cv_membership_limit")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 20, f.requests["GET /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Zero(t, f.requests["GET /api/v2/configuration-versions/cv-import"])
}

func parseTestPage(s string) int { var page int; _, _ = fmt.Sscan(s, &page); return page }

func TestImportStatusRejectsWrongRunAssociation(t *testing.T) {
	f, uploaded, _ := importExecutionFixture(t)
	*uploaded = true
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/runs/run-import" {
			_, _ = io.WriteString(w, `{"data":{"id":"run-import","type":"runs","attributes":{"plan-only":true,"status":"planned_and_finished"},"relationships":{"workspace":{"data":{"id":"ws-other","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`)
			return true
		}
		return original(w, r)
	}
	i := importPrepareInput{Phase: "status", Organization: "fixture-org", Workspace: "import-root", ConfigurationVersionID: "cv-import", RunID: "run-import", TargetAddress: "aws_iam_role.selected"}
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "run_association_unverified")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["GET /api/v2/plans/plan-import/json-output"])
}

func TestImportStatelessAmbiguousCreates(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/configuration-versions") {
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return original(w, r)
	}
	i := callImportPhase(t, importUploadInput(t))
	assert.Contains(t, i.Diagnostics, "cv_create_outcome_unknown")
	assert.Empty(t, i.Execution.ConfigurationVersionID)
	assert.Nil(t, i.Continuation, "an ambiguous create must not provide a retry recipe")
	f.mu.Lock()
	assert.Equal(t, 1, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	f.mu.Unlock()
	// Without an ID the server cannot claim idempotency or successful recovery.
}

func TestImportRunCreateOutcomeUnknownNoAutomaticRetry(t *testing.T) {
	f, uploaded, _ := importExecutionFixture(t)
	*uploaded = true
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/runs" {
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return original(w, r)
	}
	i := importUploadInput(t)
	i.Phase = "plan"
	i.ConfigurationVersionID = "cv-import"
	i.TargetAddress = "aws_iam_role.selected"
	result := callImportPhase(t, i)
	assert.Contains(t, result.Diagnostics, "run_create_outcome_unknown")
	require.NotNil(t, result.Execution)
	assert.Equal(t, "cv-import", result.Execution.ConfigurationVersionID)
	assert.Empty(t, result.Execution.RunID)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["POST /api/v2/runs"])
}

func TestImportPlanFactsReportsAbsentOrExtraImports(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v2/plans/plan-custom/json-output" {
			return false
		}
		_, _ = io.WriteString(w, `{"format_version":"1.1","resource_changes":[{"address":"aws_iam_role.other","mode":"managed","change":{"actions":["no-op"],"importing":{"id":"hidden"}}},{"address":"aws_iam_role.selected","mode":"managed","change":{"actions":["update"]}}]}`)
		return true
	}
	facts, err := readImportPlanFacts(context.Background(), f.client, "plan-custom", "aws_iam_role.selected")
	require.NoError(t, err)
	assert.False(t, facts.Selected.ImportPresent)
	assert.Equal(t, []string{"update"}, facts.Selected.Actions)
	assert.Equal(t, 1, facts.OtherImportCount)
}

func TestImportPlanFactsDriftSeparateAndBounded(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	planJSON := `{"format_version":"1.2","resource_changes":[{"address":"aws_iam_role.selected","mode":"managed","change":{"actions":["no-op"],"importing":{"id":"secret-id"}}}],"resource_drift":[{"address":"aws_iam_policy.baseline","change":{"actions":["update"],"before":{"secret":"must-not-appear"},"after":{"secret":"must-not-appear"}}}]}`
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v2/plans/plan-custom/json-output" {
			return false
		}
		_, _ = io.WriteString(w, planJSON)
		return true
	}
	facts, err := readImportPlanFacts(context.Background(), f.client, "plan-custom", "aws_iam_role.selected")
	require.NoError(t, err)
	assert.Equal(t, 1, facts.DriftCount)
	assert.Equal(t, []importResourceDrift{{Address: "aws_iam_policy.baseline", Actions: []string{"update"}}}, facts.DriftEntries)
	assert.Equal(t, 0, facts.OtherManagedActionCount, "refresh drift is not a planned managed update")
	encoded, err := json.Marshal(facts)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "must-not-appear")
	assert.NotContains(t, string(encoded), "secret-id")
	planJSON = `{"format_version":"1.2","resource_drift":[{"address":"aws_iam_policy.baseline","change":{}}]}`
	_, err = readImportPlanFacts(context.Background(), f.client, "plan-custom", "aws_iam_role.selected")
	assert.Equal(t, "plan_drift_evidence_invalid", importDiagnosticCode(err))
	drift := make([]map[string]any, 101)
	for i := range drift {
		drift[i] = map[string]any{"address": "aws_iam_policy.baseline", "change": map[string]any{"actions": []string{"update"}}}
	}
	large, err := json.Marshal(map[string]any{"format_version": "1.2", "resource_drift": drift})
	require.NoError(t, err)
	planJSON = string(large)
	_, err = readImportPlanFacts(context.Background(), f.client, "plan-custom", "aws_iam_role.selected")
	assert.Equal(t, "plan_drift_limit", importDiagnosticCode(err))
}

func TestImportCompletedPlanStatusStopsOnTruncatedFacts(t *testing.T) {
	f, uploaded, finished := importExecutionFixture(t)
	*uploaded, *finished = true, true
	changes := make([]map[string]any, 101)
	for i := range changes {
		changes[i] = map[string]any{"address": fmt.Sprintf("aws_iam_role.other_%d", i), "mode": "managed", "change": map[string]any{"actions": []string{"update"}}}
	}
	plan, err := json.Marshal(map[string]any{"format_version": "1.2", "resource_changes": changes})
	require.NoError(t, err)
	previous := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import/json-output" {
			_, _ = w.Write(plan)
			return true
		}
		return previous(w, r)
	}
	status := importPrepareInput{Phase: "status", Organization: "fixture-org", Workspace: "import-root", ConfigurationVersionID: "cv-import", RunID: "run-import", TargetAddress: "aws_iam_role.selected"}
	result := callImportPhase(t, status)
	assert.Equal(t, "blocked", result.Status)
	assert.Contains(t, result.Diagnostics, "plan_facts_truncated")
	assert.Contains(t, result.NextAction, "full finished plan JSON")
	require.NotNil(t, result.Execution.PlanFacts)
	assert.True(t, result.Execution.PlanFacts.Truncated)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func blankImportFixture(t *testing.T) (*importBackendTest, *bool, *bool) {
	t.Helper()
	f, uploaded, finished := importExecutionFixture(t)
	f.stateStatus = http.StatusNotFound
	for _, path := range []string{"/api/v2/organizations/fixture-org/workspaces/import-root", "/api/v2/workspaces/ws-fixture"} {
		// Update structured fixture data, independent of whitespace or field order.
		var document map[string]any
		require.NoError(t, json.Unmarshal(f.responses[path], &document))
		rel := document["data"].(map[string]any)["relationships"].(map[string]any)
		rel["current-configuration-version"] = map[string]any{"data": nil}
		var err error
		f.responses[path], err = json.Marshal(document)
		require.NoError(t, err)
	}
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/api/v2/plans/plan-import/json-output":
				_, _ = io.WriteString(w, `{"format_version":"1.2","resource_changes":[]}`)
				return true
			case "/api/v2/runs/run-import/plan/json-schema":
				http.Redirect(w, r, f.url+"/schema-download?signed=fixture", http.StatusTemporaryRedirect)
				return true
			}
		}
		return original(w, r)
	}
	return f, uploaded, finished
}

func TestImportBlankWorkspaceBootstrapSchemaAndExecution(t *testing.T) {
	f, uploaded, _ := blankImportFixture(t)
	i := importFixtureInput(t)
	initial := prepareImportFromAPIs(context.Background(), f.client, i)
	assert.Equal(t, "ready_for_authoring", initial.Status)
	assert.Equal(t, "unknown", initial.ManagedTypeSupport)
	assert.Nil(t, initial.SchemaSource)
	assert.Empty(t, initial.Baseline.ConfigurationVersionID)
	assert.Empty(t, initial.Baseline.StateVersionID)
	initial = callImportPhase(t, i)
	require.NotNil(t, initial.Continuation)
	assert.Equal(t, "upload", initial.Continuation.NextPhase)
	assert.Contains(t, initial.Continuation.RequiredInputs, "target_address")
	assert.Contains(t, initial.Continuation.Precondition, "Blank-workspace direct import requires target_address")
	assert.NotContains(t, initial.Continuation.Arguments, "confirm_speculative_run")
	assert.Contains(t, initial.AgentInstructions[2], "terraform init -backend=false -input=false")
	contextResult := callImportPhase(t, importPrepareInput{Phase: "context", Organization: i.Organization, Workspace: i.Workspace})
	assert.Equal(t, "blank_workspace", contextResult.Status)
	assert.Nil(t, contextResult.Context)
	assert.Contains(t, contextResult.NextAction, "target_address")
	assert.Contains(t, contextResult.NextAction, "optional, not required")
	i.Phase, i.ConfirmSpeculativeRun = "upload", true
	created := callImportPhase(t, i)
	require.Equal(t, "awaiting_agent_upload", created.Status, created.Diagnostics)
	assert.Contains(t, created.Execution.UploadInstructions, "status cannot reacquire it")
	assert.Contains(t, created.Execution.UploadInstructions, "provider-only archive")
	require.NotNil(t, created.Continuation)
	assert.NotContains(t, fmt.Sprint(created.Continuation), created.Execution.UploadURL)
	status := importPrepareInput{Phase: "status", Organization: i.Organization, Workspace: i.Workspace, ConfigurationVersionID: created.Execution.ConfigurationVersionID}
	assert.Equal(t, "awaiting_agent_upload", callImportPhase(t, status).Status)
	i.Phase, i.ConfigurationVersionID = "plan", created.Execution.ConfigurationVersionID
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "execution_cv_not_uploaded")
	*uploaded = true
	assert.Equal(t, "ready_for_plan", callImportPhase(t, status).Status)
	started := callImportPhase(t, i)
	require.Equal(t, "pending", started.Status, started.Diagnostics)
	status.RunID = started.Execution.RunID
	// A completed bootstrap Run is required: a pending one is not schema proof.
	i.Phase, i.ConfigurationVersionID, i.ConfirmSpeculativeRun = "prepare", "", false
	i.SchemaCVID, i.SchemaRunID = created.Execution.ConfigurationVersionID, started.Execution.RunID
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "bootstrap_run_not_ready_or_unverified")
	// The fixture's Run and Plan are now terminal.
	previous := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/runs/run-import" {
			_, _ = io.WriteString(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":"planned_and_finished","plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`)
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import" {
			_, _ = io.WriteString(w, `{"data":{"id":"plan-import","type":"plans","attributes":{"status":"finished"}}}`)
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import/json-output" {
			_, _ = io.WriteString(w, `{"format_version":"1.2","resource_changes":[]}`)
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/runs/run-import/plan/json-schema" {
			http.Redirect(w, r, f.url+"/schema-download?signed=fixture", http.StatusTemporaryRedirect)
			return true
		}
		return previous(w, r)
	}
	prepared := callImportPhase(t, i)
	require.Equal(t, "prepared", prepared.Status, prepared.Diagnostics)
	require.NotNil(t, prepared.Continuation)
	assert.Equal(t, "upload", prepared.Continuation.NextPhase)
	assert.NotContains(t, prepared.Continuation.RequiredInputs, "target_address")
	assert.Contains(t, prepared.Continuation.Precondition, "upload with bootstrap schema IDs must omit target_address")
	assert.Equal(t, "bootstrap_schema_ready", callImportPhase(t, status).Status)
	assert.Equal(t, "bootstrap_speculative_no_current_configuration", prepared.SchemaSource.ConfigurationBaselineRelation)
	assert.Contains(t, prepared.ManagedSchema, "block")
	i.Phase, i.ConfirmSpeculativeRun = "upload", true
	i.SchemaRunID = "run-schema"
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "bootstrap_run_not_ready_or_unverified")
	i.SchemaRunID = started.Execution.RunID
	secondOriginal := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/workspaces/ws-fixture/configuration-versions" {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"data":{"id":"cv-import-final","type":"configuration-versions","attributes":{"status":"pending","speculative":true,"auto-queue-runs":false,"upload-url":"`+f.url+`/upload"}}}`)
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/workspaces/ws-fixture/configuration-versions" {
			_, _ = io.WriteString(w, `{"data":[{"id":"cv-import","type":"configuration-versions"},{"id":"cv-import-final","type":"configuration-versions"}],"meta":{"pagination":{"current-page":1,"next-page":0,"total-count":2,"total-pages":1}}}`)
			return true
		}
		return secondOriginal(w, r)
	}
	i.Phase, i.ConfirmSpeculativeRun = "upload", true
	finalCV := callImportPhase(t, i)
	require.Equal(t, "awaiting_agent_upload", finalCV.Status, finalCV.Diagnostics)
	assert.Contains(t, finalCV.Execution.UploadInstructions, "status cannot reacquire it")
	assert.Equal(t, "cv-import-final", finalCV.Execution.ConfigurationVersionID)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 2, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Equal(t, 1, f.requests["POST /api/v2/runs"])
}

func TestImportBlankWorkspaceDoesNotTreatStateAsEmpty(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	f.stateStatus = http.StatusForbidden
	i := importFixtureInput(t)
	assert.NotEqual(t, "ready_for_authoring", prepareImportFromAPIs(context.Background(), f.client, i).Status)
	i.Phase, i.ConfirmSpeculativeRun = "upload", true
	assert.NotEqual(t, "awaiting_agent_upload", callImportPhase(t, i).Status)
	i.Phase, i.ConfigurationVersionID = "plan", "cv-import"
	assert.NotEqual(t, "pending", callImportPhase(t, i).Status)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func TestImportDirectBlankWorkspacePlanWithoutSchemaSource(t *testing.T) {
	f, uploaded, finished := blankImportFixture(t)
	previous := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import/json-output" {
			_, _ = io.WriteString(w, `{"format_version":"1.2","resource_changes":[{"address":"aws_iam_role.selected","mode":"managed","type":"aws_iam_role","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"secret-not-returned"}}}]}`)
			return true
		}
		return previous(w, r)
	}
	i := importFixtureInput(t)
	i.Phase, i.ConfirmSpeculativeRun, i.TargetAddress = "upload", true, "aws_iam_role.selected"
	created := callImportPhase(t, i)
	require.Equal(t, "awaiting_agent_upload", created.Status, created.Diagnostics)
	assert.Equal(t, i.TargetAddress, created.Execution.TargetAddress)
	require.NotNil(t, created.WorkflowContext)
	assert.Equal(t, i.Selections[0].CandidateID, created.WorkflowContext.CandidateID)
	assert.Equal(t, i.TargetAddress, created.WorkflowContext.TargetAddress)
	assert.Empty(t, created.WorkflowContext.BaselineCVID)
	*uploaded = true
	i.Phase, i.ConfigurationVersionID = "plan", created.Execution.ConfigurationVersionID
	started := callImportPhase(t, i)
	require.Equal(t, "pending", started.Status, started.Diagnostics)
	assert.Equal(t, "run_created", started.Execution.Stage)
	*finished = true
	status := importPrepareInput{Phase: "status", Organization: i.Organization, Workspace: i.Workspace, ConfigurationVersionID: i.ConfigurationVersionID, RunID: started.Execution.RunID, TargetAddress: i.TargetAddress}
	done := callImportPhase(t, status)
	require.Equal(t, "plan_available_for_agent_assessment", done.Status, done.Diagnostics)
	require.NotNil(t, done.Execution.PlanFacts.Selected)
	assert.True(t, done.Execution.PlanFacts.Selected.ImportPresent)
	encoded, err := json.Marshal(done)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret-not-returned")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Equal(t, 1, f.requests["POST /api/v2/runs"])
	assert.Zero(t, f.requests["GET /api/v2/runs/run-import/plan/json-schema"])
}

func TestImportDirectBlankWorkspaceRejectsUnverifiedTypeAndState(t *testing.T) {
	f, uploaded, _ := blankImportFixture(t)
	i := importFixtureInput(t)
	i.Phase, i.ConfirmSpeculativeRun, i.TargetAddress = "upload", true, "aws_iam_role.selected"
	i.Selections[0].ManagedType = "aws_iam_policy"
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "blank_managed_type_unverified")
	i.Selections[0].ManagedType = "aws_iam_role"
	f.stateStatus = http.StatusForbidden
	assert.NotEqual(t, "awaiting_agent_upload", callImportPhase(t, i).Status)
	f.stateStatus = http.StatusNotFound
	*uploaded = true
	i.Phase, i.ConfigurationVersionID = "plan", "cv-import"
	i.Selections[0].ManagedType = "aws_iam_policy"
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "blank_managed_type_unverified")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Zero(t, f.requests["POST /api/v2/runs"])
}

func TestImportBootstrapSchemaRejectsNonEmptyPlanAndUnrelatedRun(t *testing.T) {
	f, uploaded, _ := blankImportFixture(t)
	*uploaded = true
	original := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/runs/run-import" {
			_, _ = io.WriteString(w, `{"data":{"id":"run-import","type":"runs","attributes":{"status":"planned_and_finished","plan-only":true},"relationships":{"workspace":{"data":{"id":"ws-fixture","type":"workspaces"}},"configuration-version":{"data":{"id":"cv-import","type":"configuration-versions"}},"plan":{"data":{"id":"plan-import","type":"plans"}}}}}`)
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import" {
			_, _ = io.WriteString(w, `{"data":{"id":"plan-import","type":"plans","attributes":{"status":"finished"}}}`)
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import/json-output" {
			_, _ = io.WriteString(w, `{"format_version":"1.2","resource_changes":[{"address":"aws_iam_role.unexpected","mode":"managed","change":{"actions":["no-op"]}}]}`)
			return true
		}
		return original(w, r)
	}
	i := importFixtureInput(t)
	i.SchemaCVID, i.SchemaRunID = "cv-import", "run-import"
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "bootstrap_plan_not_empty")
	i.SchemaRunID = "run-schema"
	assert.Contains(t, callImportPhase(t, i).Diagnostics, "bootstrap_run_not_ready_or_unverified")
	i.Phase, i.ConfirmSpeculativeRun = "upload", true
	assert.NotEqual(t, "awaiting_agent_upload", callImportPhase(t, i).Status)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
}
