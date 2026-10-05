// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Agent-supplied schema (ADR 0008): the state's run has no plan, so there is no schema
// artifact, but its configuration version is downloadable.
func TestPrepareImportToolAgentSchemaRequiredWhenStateRunHasNoPlan(t *testing.T) {
	f := schemaFallbackFixture(t, "uploaded")
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "agent_schema_required", out.Status)
	assert.Equal(t, "cv-run", out.StateRunConfigurationVersionID)
	assert.Contains(t, out.Diagnostics, "schema_source_plan_unavailable")
	require.NotNil(t, out.Carry, "candidates and carry are still returned")
	assert.Greater(t, logRequests(f), 0)
	assert.Equal(t, importIdentityUnknown, out.Carry.Destination.IdentitySupport["aws_iam_role"])
	assert.Equal(t, "unknown", out.Types[0].ManagedTypeSupport)
	assert.Empty(t, out.Types[0].ManagedSchema)
	assert.NotEmpty(t, out.Carry.Destination.TerraformVersion)
	assert.Contains(t, out.Notes, "target_schema_not_read_from_hcp; the agent must obtain it")
	assert.NotContains(t, out.NextAction, "Route")
	i := strings.Index(out.NextAction, ".terraform.lock.hcl")
	j := strings.Index(out.NextAction, "terraform init")
	require.True(t, i >= 0 && j > i, "the lock file check must come before init")
	assert.Contains(t, out.NextAction, "acknowledgement")
	assert.Contains(t, out.NextAction, "lock_file_absent")
	assert.Contains(t, out.NextAction, "Never run apply")
}

func TestPrepareImportToolGuideOnlyWhenStateRunConfigurationNotDownloadable(t *testing.T) {
	f := schemaFallbackFixture(t, "errored")
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "target_config_not_downloadable")
	assert.Empty(t, out.StateRunConfigurationVersionID)
	assert.Equal(t, 0, logRequests(f))
	assert.Contains(t, out.NextAction, "cannot assess or verify")
}

func TestHCPSchemaArtifactIsPreferredWhenSchemaArtifactIsAvailable(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	assert.Empty(t, out.StateRunConfigurationVersionID)
}

func TestDownloadToolAcceptsStateRunConfigurationVersionOnlyWhenAgentMustObtainSchema(t *testing.T) {
	f := schemaFallbackFixture(t, "uploaded")
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	call := func(cv string) importDownload {
		result, err := HandleGetImportConfigurationDownload(context.Background(), downloadRequest(map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "configuration_version_id": cv}), silentLogger())
		require.NoError(t, err)
		return result.StructuredContent.(importDownload)
	}
	ok := call("cv-run")
	assert.Equal(t, "available", ok.Status, ok.Diagnostics)
	assert.Equal(t, "cv-run", ok.ConfigurationVersionID)
	assert.Equal(t, f.url+"/cv-archive?signed=fixture", ok.DownloadURL)
	other := call("cv-speculative")
	assert.Equal(t, "blocked", other.Status)
	assert.Contains(t, other.Diagnostics, "configuration_version_not_current")
	assert.Empty(t, other.DownloadURL)
	assert.Equal(t, "available", call("cv-current").Status, "the current version still works")
	f.mu.Lock()
	defer f.mu.Unlock()
	for request := range f.requests {
		assert.True(t, strings.HasPrefix(request, http.MethodGet+" "), request)
	}
}

func TestDownloadToolRejectsOtherVersionWhenStateRunHasPlan(t *testing.T) {
	f := importBackendFixture(t)
	f.responses["/api/v2/configuration-versions/cv-run"] = json.RawMessage(`{"data":{"type":"configuration-versions","id":"cv-run","attributes":{"status":"uploaded"}}}`)
	out := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root"}, "cv-run", silentLogger())
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "configuration_version_not_current")
}

// Scenario matrix S01 to S22 in WORKFLOW_SPEC; rows listed here are the ones a
// hermetic prepare_import fixture can express. Other rows are planned.
func TestPrepareImportToolScenarioMatrix(t *testing.T) {
	for _, tc := range []struct {
		id, status, code string
		logReads         bool
		setup            func(*testing.T) *importBackendTest
	}{
		{"S01 blank remote", "ready_for_authoring", "", true, func(t *testing.T) *importBackendTest { f, _, _ := blankImportFixture(t); return f }},
		{"S03 local no state", "blocked", "workspace_execution_mode_local", false, func(t *testing.T) *importBackendTest {
			f, _, _ := blankImportFixture(t)
			setWorkspaceAttribute(t, f, "execution-mode", "local")
			return f
		}},
		{"S05 schema from HCP, identity classified", "prepared", "", true, importBackendFixture},
		{"S07 below 1.5", "blocked", "terraform_version_unsupported", false, func(t *testing.T) *importBackendTest {
			f := importBackendFixture(t)
			setWorkspaceAttribute(t, f, "terraform-version", "1.4.7")
			return f
		}},
		{"S09 agent obtains schema", "agent_schema_required", "schema_source_plan_unavailable", true, func(t *testing.T) *importBackendTest { return schemaFallbackFixture(t, "uploaded") }},
		{"S13 no run link", "blocked", "state_producing_run_missing", false, func(t *testing.T) *importBackendTest {
			f := importBackendFixture(t)
			f.responses["/api/v2/workspaces/ws-fixture/current-state-version"] = json.RawMessage(`{"data":{"id":"sv-manual","type":"state-versions"}}`)
			return f
		}},
		{"S14 configuration not downloadable", "blocked", "target_config_not_downloadable", false, func(t *testing.T) *importBackendTest { return schemaFallbackFixture(t, "archived") }},
		{"S16 local with state", "blocked", "workspace_execution_mode_local", false, func(t *testing.T) *importBackendTest {
			f := importBackendFixture(t)
			setWorkspaceAttribute(t, f, "execution-mode", "local")
			return f
		}},
		{"S19 agent prepares", "prepared", "", true, func(t *testing.T) *importBackendTest {
			f := importBackendFixture(t)
			setWorkspaceAttribute(t, f, "execution-mode", "agent")
			return f
		}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			f := tc.setup(t)
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			assert.Equal(t, tc.status, out.Status, out.Diagnostics)
			if tc.code != "" {
				assert.Contains(t, out.Diagnostics, tc.code)
			} else {
				assert.Empty(t, out.Diagnostics)
			}
			assert.Equal(t, tc.logReads, logRequests(f) > 0)
			if tc.status == "blocked" {
				assert.Nil(t, out.Carry)
				assert.NotContains(t, strings.ToLower(out.NextAction), "apply now")
			}
		})
	}
}

// With support unknown (agent-supplied schema) the plan's own identity decides.
func TestIdentityClassificationWhenSupportUnknown(t *testing.T) {
	carried := importCarryCandidate{Identity: map[string]any{"name": "role-a"}}
	status, _ := classifyIdentity("1.16.1", carried, importIdentityUnknown, json.RawMessage(`{"name":"role-a"}`))
	assert.Equal(t, identityMatched, status)
	status, reason := classifyIdentity("1.16.1", carried, importIdentityUnknown, nil)
	assert.Equal(t, identityUnsupported, status)
	assert.Equal(t, reasonUndetermined, reason)
}

func TestCreateToolTextAsksForOneConfirmation(t *testing.T) {
	for _, tool := range []struct{ name, text string }{
		{"cv", CreateImportCVDefinition().Description},
		{"run", CreateImportRunDefinition().Description},
	} {
		assert.NotContains(t, tool.text, "separate explicit", tool.name)
		assert.Contains(t, tool.text, "speculative", tool.name)
		assert.Contains(t, tool.text, "no input changes that", tool.name)
	}
	assert.Contains(t, CreateImportCVDefinition().Description, "terraform fmt")
	assert.Contains(t, CreateImportCVDefinition().Description, "Do not run them after the upload")
	f, uploaded, _ := importExecutionFixture(t)
	_ = uploaded
	out := callCreate(t, false, createArgs(nil))
	require.Equal(t, "awaiting_agent_upload", out.Status, out.Diagnostics)
	assert.NotContains(t, out.NextAction, "separate confirmation")
	assert.Contains(t, out.NextAction, "already confirmed the speculative path")
	assert.Contains(t, out.NextAction, "archive or baseline")
	_ = f
}

func TestPrepareImportToolTextCoversReviewValidationAndSecrets(t *testing.T) {
	joined := strings.Join(importToolInstructions, " ")
	assert.Contains(t, joined, "ask once")
	assert.Contains(t, joined, "terraform fmt and terraform validate")
	assert.Contains(t, joined, "Do not run them after the upload")
	assert.Contains(t, joined, ".envrc")
	assert.Contains(t, strings.Join(importDownloadInstructions, " "), ".envrc")
	assert.Contains(t, importGuideOnlyNextAction("x"), ".envrc")
	assert.Contains(t, importAgentSchemaNextAction, ".envrc")
	assert.Contains(t, importGuideOnlyNextAction("x"), "HCP Terraform run write its state")
}

func TestPrepareImportDoesNotOwnDiscoveryGuidance(t *testing.T) {
	f := importBackendFixture(t)
	f.responses["/api/v2/queries/qry-fixture"] = []byte(strings.Replace(string(f.responses["/api/v2/queries/qry-fixture"]), `"generate-config-out": true`, `"generate-config-out": false`, 1))
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	assert.NotContains(t, out.Notes, "query_run_without_generated_config")
	assert.NotContains(t, out.NextAction, "re-run")
	joined := strings.Join(importToolInstructions, " ")
	assert.Contains(t, joined, "only for candidates the user has chosen to import")
	assert.Contains(t, joined, "get_query_summary returns each result's tags")
	assert.Contains(t, PrepareImportDefinition().Description, "not to search or filter by tag")
}

func TestDiscoveryPageReturnsTagsAndOmitsWhenAbsent(t *testing.T) {
	d := &importDiscovery{QueryRunID: "q", LogDigest: "d", Candidates: []importDiscoveryCandidate{
		{CandidateID: "c1", Address: "list.a.b", ResourceType: "a", DisplayName: "one", ResourceObject: map[string]any{"tags": map[string]any{"test": "true", "n": 1.0, "nested": map[string]any{"x": "y"}}, "tags_all": map[string]any{"test": "true"}}},
		{CandidateID: "c2", Address: "list.a.b", ResourceType: "a", DisplayName: "two", ResourceObject: map[string]any{"name": "two"}},
		{CandidateID: "c3", Address: "list.a.b", ResourceType: "a", DisplayName: "three"},
	}}
	page, err := pageImportDiscovery(d, DiscoveryFilter{})
	require.NoError(t, err)
	rows := page.Lists[0].Candidates
	assert.Equal(t, map[string]any{"test": "true", "n": 1.0}, rows[0].Tags, "scalars only, tags_all not returned")
	assert.Nil(t, rows[1].Tags)
	assert.Nil(t, rows[2].Tags)
	encoded, _ := json.Marshal(page)
	assert.NotContains(t, string(encoded), "tags_all")
	assert.Equal(t, 1, strings.Count(string(encoded), `"tags"`), "absent tags are omitted")
	assert.NotContains(t, page.NextAction, "re-run", "no hint when the query reports nothing")
}

func TestDiscoveryPageOwnsGenerateConfigOutGuidance(t *testing.T) {
	no, yes := false, true
	for _, tc := range []struct {
		flag *bool
		hint bool
	}{{&no, true}, {&yes, false}, {nil, false}} {
		d := &importDiscovery{QueryRunID: "q", LogDigest: "d", GenerateConfigOut: tc.flag, Candidates: []importDiscoveryCandidate{{CandidateID: "c1", Address: "list.a.b", ResourceType: "a"}}}
		page, err := pageImportDiscovery(d, DiscoveryFilter{})
		require.NoError(t, err)
		assert.Equal(t, tc.hint, strings.Contains(page.NextAction, "re-run the query with generate_config_out true"))
		assert.Equal(t, tc.hint, len(page.Notes) == 1 && page.Notes[0] == "query_run_without_generated_config")
		assert.NotContains(t, strings.ToLower(page.NextAction), "aws cli")
	}
}
