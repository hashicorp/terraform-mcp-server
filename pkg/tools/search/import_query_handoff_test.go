// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportConfigurationHandoffDoesNotDownloadArchive(t *testing.T) {
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	tool := ImportQueryResults(silentLogger(), nil)
	args := map[string]any{"phase": "context", "organization_name": "fixture-org", "workspace_name": "import-root"}
	result, err := tool.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
	require.NoError(t, err)
	require.False(t, result.IsError, result.StructuredContent)
	packet, ok := result.StructuredContent.(importPreparation)
	require.True(t, ok)
	require.NotNil(t, packet.Context)
	assert.Equal(t, "cv-current", packet.Baseline.ConfigurationVersionID)
	assert.Nil(t, packet.Baseline.StateSerial, "context does not read current state")
	assert.Empty(t, packet.Baseline.StateVersionID)
	assert.Contains(t, packet.Notes, "current_state_not_observed_by_context; use prepare to verify current state ID and serial before upload or plan")
	assert.Equal(t, "cv-current", packet.Context.ConfigurationVersionID)
	assert.Equal(t, f.url+"/cv-archive?signed=fixture", packet.Context.DownloadURL)
	assert.Equal(t, "configuration_handoff", packet.Stage)
	assert.Contains(t, packet.NextAction, "isolated temporary directory")
	assert.Contains(t, packet.NextAction, "user-approved directory")
	assert.Contains(t, packet.NextAction, "fresh context URL")
	assert.Contains(t, packet.NextAction, "working_directory")
	assert.Contains(t, packet.NextAction, "binary-capable HTTP GET")
	assert.Contains(t, packet.NextAction, "a browser is not required")
	assert.Contains(t, packet.NextAction, "archive bytes, not text/JSON")
	assert.Contains(t, packet.NextAction, "exposed command arguments")
	assert.Contains(t, packet.NextAction, "stop and ask")
	assert.Contains(t, strings.Join(packet.AgentInstructions, " "), ".terraform.lock.hcl")
	assert.Contains(t, strings.Join(packet.AgentInstructions, " "), "client-local choice")
	assert.Contains(t, strings.Join(packet.AgentInstructions, " "), "binary-read method your client actually supports")
	assert.NotContains(t, packet.NextAction, packet.Context.DownloadURL, "signed URL stays only in configuration_context")
	schemaJSON, err := json.Marshal(tool.Tool.OutputSchema)
	require.NoError(t, err)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(schemaJSON, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	encoded, err := json.Marshal(packet)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"state_serial":0`)
	var object any
	require.NoError(t, json.Unmarshal(encoded, &object))
	require.NoError(t, resolved.Validate(object))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["GET /api/v2/configuration-versions/cv-current/download"])
	assert.Equal(t, 0, f.requests["GET /cv-archive"])
	for request := range f.requests {
		assert.True(t, strings.HasPrefix(request, http.MethodGet+" "), request)
	}
}

func TestImportConfigurationHandoffFailures(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		change     func(*importBackendTest)
	}{
		{"missing CV", "configuration_source_unavailable", func(f *importBackendTest) {
			f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"] = []byte(strings.Replace(string(f.responses["/api/v2/organizations/fixture-org/workspaces/import-root"]), `"current-configuration-version": {"data": {"type": "configuration-versions", "id": "cv-current"}},`, "", 1))
		}},
		{"unuploaded CV", "configuration_source_not_uploaded", func(f *importBackendTest) {
			f.responses["/api/v2/configuration-versions/cv-current"] = []byte(strings.Replace(string(f.responses["/api/v2/configuration-versions/cv-current"]), `"status": "uploaded"`, `"status": "pending"`, 1))
		}},
		{"missing artifact", "backend_evidence_unavailable", func(f *importBackendTest) { f.cvDownloadStatus = http.StatusNotFound }},
		{"no redirect", "configuration_download_location_unavailable", func(f *importBackendTest) { f.cvDownloadStatus = http.StatusOK }},
		{"changed baseline", "baseline_changed", func(f *importBackendTest) { f.baselineChanged = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := importBackendFixture(t)
			t.Setenv(client.TerraformAddress, f.url)
			t.Setenv(client.TerraformToken, "fixture-token")
			tc.change(f)
			result := importConfigurationContextFromAPIs(context.Background(), f.client, importPrepareInput{Phase: "context", Organization: "fixture-org", Workspace: "import-root"}, silentLogger())
			assert.Equal(t, "blocked", result.Status)
			assert.Contains(t, result.Diagnostics, tc.code)
			assert.Nil(t, result.Context)
			f.mu.Lock()
			assert.Equal(t, 0, f.requests["GET /cv-archive"])
			f.mu.Unlock()
		})
	}
}

func TestImportBlankContextDoesNotClaimUnobservedState(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	result := callImportPhase(t, importPrepareInput{Phase: "context", Organization: "fixture-org", Workspace: "import-root"})
	require.Equal(t, "blank_workspace", result.Status, result.Diagnostics)
	require.NotNil(t, result.Baseline)
	assert.Nil(t, result.Baseline.StateSerial)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"state_serial":0`)
	f.stateStatus = http.StatusForbidden
	blocked := callImportPhase(t, importPrepareInput{Phase: "context", Organization: "fixture-org", Workspace: "import-root"})
	assert.Equal(t, "blocked", blocked.Status)
	assert.NotEqual(t, "blank_workspace", blocked.Status, "inaccessible state is not an empty workspace")
}
