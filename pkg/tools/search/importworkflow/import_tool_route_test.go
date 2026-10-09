// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"fmt"
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
	assert.Equal(t, importIdentityUnknown, out.Carry.Target.IdentitySupport["aws_iam_role"])
	assert.Equal(t, "unknown", out.Types[0].ManagedTypeSupport)
	assert.Empty(t, out.Types[0].ManagedSchema)
	assert.NotEmpty(t, out.Carry.Target.TerraformVersion)
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
		result, err := HandleGetImportConfigurationDownload(context.Background(), downloadRequest(map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "prepared_target_workspace_id": "ws-fixture", "configuration_version_id": cv}), silentLogger())
		require.NoError(t, err)
		return result.StructuredContent.(importDownload)
	}
	ok := call("cv-run")
	assert.Equal(t, "available", ok.Status, ok.Diagnostics)
	assert.Equal(t, "cv-run", ok.ConfigurationVersionID)
	assert.Equal(t, "state_run_configuration", ok.ConfigurationRole)
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
	out := downloadImportConfiguration(context.Background(), f.client, importPrepareInput{Organization: "fixture-org", Workspace: "import-root", PreparedTargetID: "ws-fixture"}, "cv-run", silentLogger())
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
		{"S19 agent stops before authoring", "blocked", "execution_source_not_supported", false, func(t *testing.T) *importBackendTest {
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
	carried := importCarryCandidate{Identity: map[string]any{"name": "role-a"}, SearchIdentityVersion: intPtr(0)}
	planVersion := uint64(0)
	status, _ := classifyIdentity("1.16.1", carried, importIdentityUnknown, json.RawMessage(`{"name":"role-a"}`), &planVersion)
	assert.Equal(t, identityMatched, status)
	// Without a plan-time version the same identity is not confirmed.
	status, reason := classifyIdentity("1.16.1", carried, importIdentityUnknown, json.RawMessage(`{"name":"role-a"}`), nil)
	assert.Equal(t, identityUnverified, status)
	assert.Equal(t, reasonIdentityVersionNotCompared, reason)
	status, reason = classifyIdentity("1.16.1", carried, importIdentityUnknown, nil, nil)
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
	assert.Contains(t, CreateImportCVDefinition().Description, "Do not run these after the upload")
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
	assert.Contains(t, joined, "terraform fmt before the upload")
	assert.Contains(t, joined, "Do not run these after the upload")
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

func discoveryWith(addresses []string, withAttrs func(i int) bool) *importDiscovery {
	d := &importDiscovery{QueryRunID: "q", LogDigest: "d"}
	for i, a := range addresses {
		c := importDiscoveryCandidate{CandidateID: fmt.Sprintf("c%d", i), Address: a, ResourceType: "a", DisplayName: fmt.Sprintf("n%d", i)}
		if withAttrs(i) {
			c.ResourceObject = map[string]any{"name": "x"}
		}
		d.Candidates = append(d.Candidates, c)
	}
	return d
}

func TestDiscoveryPageSaysWhetherAttributesWereCaptured(t *testing.T) {
	addrs := []string{"list.a.b", "list.a.b", "list.a.b", "list.a.b"}
	for _, tc := range []struct {
		name      string
		withAttrs func(int) bool
		without   int
		note      string
		hint      string
	}{
		{"all captured", func(int) bool { return true }, 0, "", ""},
		{"none captured", func(int) bool { return false }, 4, "resource_attributes_not_captured", "means tags were not captured, not that the resource has none"},
		{"partly captured", func(i int) bool { return i < 3 }, 1, "resource_attributes_partly_captured", "1 matching rows carry no resource attributes"},
	} {
		page, err := pageImportDiscovery(discoveryWith(addrs, tc.withAttrs), DiscoveryFilter{})
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.without, page.RowsWithoutAttributes, tc.name)
		if tc.note == "" {
			assert.Empty(t, page.Notes, tc.name)
			assert.NotContains(t, page.NextAction, "generate_config_out", tc.name)
			continue
		}
		assert.Contains(t, page.Notes, tc.note, tc.name)
		assert.Contains(t, page.NextAction, tc.hint, tc.name)
		assert.Contains(t, page.NextAction, "generate_config_out true", tc.name)
		assert.NotContains(t, strings.ToLower(page.NextAction), "aws cli", tc.name)
	}
}

func TestDiscoveryPageStatesPossibleMissingResults(t *testing.T) {
	few, err := pageImportDiscovery(discoveryWith([]string{"list.a.b", "list.a.b"}, func(int) bool { return true }), DiscoveryFilter{})
	require.NoError(t, err)
	assert.Contains(t, few.NextAction, "more matching resources may exist")
	assert.Contains(t, few.NextAction, "These are all the results this query returned")
	assert.NotContains(t, few.Notes, "list_total_equals_default_limit")
	assert.NotContains(t, few.NextAction, "default list limit")

	addrs := make([]string, defaultListLimit)
	for i := range addrs {
		addrs[i] = "list.a.b"
	}
	full, err := pageImportDiscovery(discoveryWith(addrs, func(int) bool { return true }), DiscoveryFilter{})
	require.NoError(t, err)
	assert.Contains(t, full.Notes, "list_total_equals_default_limit")
	assert.Contains(t, full.NextAction, "exactly 100 results, Terraform's default list limit, so it may have been cut off")
	assert.Contains(t, full.NextAction, "more matching resources may exist")
	assert.NotContains(t, full.NextAction, "All matching results are listed")
}

func TestPrepareImportGuidesAdaptationWithoutBlanketRules(t *testing.T) {
	g := importAdaptationGuidance
	for _, want := range []string{"target provider schema JSON", "managed_schema", "provider_version", "get_provider_details", "versioned", "terraform validate", "differs between Terraform versions", "follow it", "list each adaptation"} {
		if want == "list each adaptation" {
			assert.Contains(t, importConfirmationRule, "each adaptation you made")
			continue
		}
		assert.Contains(t, g, want)
	}
	lower := strings.ToLower(importToolTextsJoined())
	for _, banned := range []string{"always remove provider", "always drop", "remove provider from", "drop provider", "provider is rejected"} {
		assert.NotContains(t, lower, banned)
	}
	assert.Contains(t, strings.Join(importToolInstructions, " "), g)
}

func importToolTextsJoined() string {
	parts := append([]string{}, importToolInstructions...)
	parts = append(parts, importAdaptationGuidance, importConfirmationRule, PrepareImportDefinition().Description)
	return strings.Join(parts, "\n")
}

func TestDownloadInstructionsKeepSignedURLOffCommandLines(t *testing.T) {
	joined := strings.Join(importDownloadInstructions, " ")
	assert.Contains(t, joined, importArchiveURLRule)
	assert.Contains(t, importArchiveURLRule, "curl --config -")
	assert.Contains(t, importArchiveURLRule, "reduces exposure but does not remove it")
	assert.Contains(t, importArchiveURLRule, "stop and ask")
	assert.NotContains(t, strings.ToLower(joined), "print the url")
}
