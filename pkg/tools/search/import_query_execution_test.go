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

func TestImportStatelessCVRunAndPlanFacts(t *testing.T) {
	f, _, finished := importExecutionFixture(t)
	created := callImportPhase(t, importUploadInput(t))
	require.Equal(t, "awaiting_agent_upload", created.Status, created.Diagnostics)
	require.Equal(t, "cv-import", created.Execution.ConfigurationVersionID)
	status := importPrepareInput{Phase: "status", Organization: "fixture-org", Workspace: "import-root", ConfigurationVersionID: created.Execution.ConfigurationVersionID}
	assert.Equal(t, "awaiting_agent_upload", callImportPhase(t, status).Status)
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
	started := callImportPhase(t, plan)
	require.Equal(t, "pending", started.Status, started.Diagnostics)
	require.Equal(t, "run-import", started.Execution.RunID)
	status.RunID = started.Execution.RunID
	status.TargetAddress = plan.TargetAddress
	assert.Equal(t, "pending", callImportPhase(t, status).Status)
	*finished = true
	done := callImportPhase(t, status)
	require.Equal(t, "plan_available_for_agent_assessment", done.Status, done.Diagnostics)
	assert.Equal(t, "aws_iam_role.selected", done.Execution.PlanFacts.Selected.Address)
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
	var instance any
	require.NoError(t, json.Unmarshal(encoded, &instance))
	require.NoError(t, resolved.Validate(instance))
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
