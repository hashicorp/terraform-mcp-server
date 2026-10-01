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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportBatchCandidatePlanCorrelation(t *testing.T) {
	f, _, finished := importExecutionFixture(t)
	first := `{"type":"list_resource_found","list_resource_found":{"address":"list.aws_iam_role.roles","resource_type":"aws_iam_role","display_name":"other","identity":{"account_id":"123456789012","name":"second"}}}`
	f.queryLog = []byte(strings.Replace(string(f.queryLog), `{"type":"list_complete"`, first+"\n"+`{"type":"list_complete"`, 1))
	f.queryLog = []byte(strings.Replace(string(f.queryLog), `"total":1`, `"total":2`, 1))
	candidates, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
	require.NoError(t, err)
	require.Len(t, candidates.Candidates, 2)
	oldHandler := f.mutationHandler
	f.mutationHandler = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v2/plans/plan-import/json-output" {
			_, _ = io.WriteString(w, `{"format_version":"1.2","complete":true,"resource_changes":[{"address":"aws_iam_role.selected","mode":"managed","type":"aws_iam_role","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"first-id"},"after_identity":{"account_id":"123456789012","name":"search-import-fixture"}}},{"address":"aws_iam_role.second","mode":"managed","type":"aws_iam_role","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"second-id"},"after_identity":{"account_id":"123456789012","name":"second"}}}]}`)
			return true
		}
		return oldHandler(w, r)
	}
	input := importFixtureInput(t)
	input.Selections = []importSelection{
		{CandidateID: candidates.Candidates[0].CandidateID, ManagedType: "aws_iam_role", TargetAddress: "aws_iam_role.selected"},
		{CandidateID: candidates.Candidates[1].CandidateID, ManagedType: "aws_iam_role", TargetAddress: "aws_iam_role.second"},
	}
	prepared := callImportPhase(t, input)
	require.Equal(t, "prepared", prepared.Status, prepared.Diagnostics)
	require.Len(t, prepared.SelectedCandidates, 2)
	assert.Equal(t, candidates.Candidates[1].CandidateID, prepared.SelectedCandidates[1].CandidateID)
	require.NotNil(t, prepared.Continuation)
	assert.NotContains(t, fmt.Sprint(prepared.Continuation), "/upload")
	input.Phase, input.ConfirmSpeculativeRun = "upload", true
	input.BaselineCVID, input.BaselineStateID, input.BaselineSerial = "cv-current", "sv-current", 42
	created := callImportPhase(t, input)
	require.Equal(t, "awaiting_agent_upload", created.Status, created.Diagnostics)
	request, err := http.NewRequest(http.MethodPut, created.Execution.UploadURL, strings.NewReader("agent-owned-archive"))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	input.Phase, input.ConfigurationVersionID = "plan", created.Execution.ConfigurationVersionID
	started := callImportPhase(t, input)
	require.Equal(t, "pending", started.Status, started.Diagnostics)
	require.NotNil(t, started.Continuation)
	assert.Contains(t, fmt.Sprint(started.Continuation.Arguments), candidates.Candidates[1].CandidateID)
	*finished = true
	input.Phase, input.RunID, input.ConfirmSpeculativeRun = "status", started.Execution.RunID, false
	input.BaselineCVID, input.BaselineStateID, input.BaselineSerial = "", "", 0
	done := callImportPhase(t, input)
	require.Equal(t, "plan_available_for_agent_assessment", done.Status, done.Diagnostics)
	require.NotNil(t, done.Execution.BatchPlanFacts)
	facts := done.Execution.BatchPlanFacts
	require.Len(t, facts.Candidates, 2)
	for i, candidate := range facts.Candidates {
		assert.Equal(t, input.Selections[i].CandidateID, candidate.CandidateID)
		assert.Equal(t, "matched", candidate.PlanBinding)
		assert.Equal(t, "matched", candidate.ObjectIdentity)
	}
	assert.Zero(t, facts.OtherImportCount)
	encoded, err := json.Marshal(done)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "first-id", "do not expose raw import IDs")
	schemaJSON, err := json.Marshal(ImportQueryResults(silentLogger(), nil).Tool.OutputSchema)
	require.NoError(t, err)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	for _, packet := range []importPreparation{prepared, created, started, done} {
		body, err := json.Marshal(packet)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(body, &value))
		require.NoError(t, resolved.Validate(value))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
	assert.Equal(t, 1, f.requests["POST /api/v2/runs"])
	assert.Equal(t, 1, f.requests["GET /api/v2/plans/plan-import/json-output"], "read the plan once for the entire batch")
}

func TestImportBatchSelectionBoundsBeforePOST(t *testing.T) {
	f, _, _ := importExecutionFixture(t)
	input := importUploadInput(t)
	input.Selections = []importSelection{
		{CandidateID: importFixtureInput(t).Selections[0].CandidateID, ManagedType: "aws_iam_role", TargetAddress: "aws_iam_role.first"},
		{CandidateID: importFixtureInput(t).Selections[0].CandidateID, ManagedType: "aws_iam_role", TargetAddress: "aws_iam_role.second"},
	}
	assert.Contains(t, callImportPhase(t, input).Diagnostics, "import_selection_invalid")
	input.Selections = make([]importSelection, maxImportSelections)
	for index := range input.Selections {
		input.Selections[index] = importSelection{CandidateID: fmt.Sprintf("candidate-%064x", index), ManagedType: "aws_iam_role", TargetAddress: fmt.Sprintf("aws_iam_role.r%d", index)}
	}
	assert.True(t, validImportSelections(input), "100 reviewed bindings fit the input ceiling")
	input.Selections = append(input.Selections, importSelection{CandidateID: fmt.Sprintf("candidate-%064x", maxImportSelections), ManagedType: "aws_iam_role", TargetAddress: "aws_iam_role.extra"})
	assert.Contains(t, callImportPhase(t, input).Diagnostics, "import_input_invalid")
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Zero(t, f.requests["POST /api/v2/workspaces/ws-fixture/configuration-versions"])
}

func TestImportBatchIdentityEvidenceNeverUsesAuthoredIdentityAsProof(t *testing.T) {
	source := map[string]any{"account_id": "123456789012", "name": "role-a"}
	for _, tc := range []struct {
		name, after, status string
	}{
		{"provider returned same identity", `{"name":"role-a","account_id":"123456789012"}`, "matched"},
		{"provider returned different object", `{"account_id":"123456789012","name":"role-b"}`, "mismatched"},
		{"only authored import ID", `null`, "unverified"},
		{"provider returned extra identity key", `{"account_id":"123456789012","name":"role-a","region":"us-east-1"}`, "unverified"},
		{"unknown numeric semantics", `{"account_id":"123456789012","name":1}`, "unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := importCompareProviderIdentity(source, json.RawMessage(tc.after))
			assert.Equal(t, tc.status, status)
		})
	}
}
