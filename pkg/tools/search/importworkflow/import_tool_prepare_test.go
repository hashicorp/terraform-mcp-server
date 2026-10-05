// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformVersionAtLeast(t *testing.T) {
	for _, tt := range []struct {
		version        string
		atLeast, known bool
	}{
		{"1.12.0", true, true}, {"1.16.1", true, true}, {"1.11.9", false, true}, {"1.5.7", false, true},
		{"v1.12.0-beta1", true, true}, {"2.0.0", true, true}, {"", false, false}, {"latest", false, false}, {"1", false, false},
	} {
		atLeast, known := terraformVersionAtLeast(tt.version, 1, 12)
		assert.Equal(t, tt.atLeast, atLeast, tt.version)
		assert.Equal(t, tt.known, known, tt.version)
	}
}

func TestImportIdentitySupportFor(t *testing.T) {
	has := json.RawMessage(`{"version":0,"attributes":{}}`)
	assert.Equal(t, importIdentitySupported, importIdentitySupportFor(has, "1.11.0"))
	assert.Equal(t, importIdentityNone, importIdentitySupportFor(nil, "1.12.0"))
	assert.Equal(t, importIdentityUnknown, importIdentitySupportFor(nil, "1.11.4"), "below 1.12 cannot distinguish none from not produced")
	assert.Equal(t, importIdentityUnknown, importIdentitySupportFor(nil, ""))
	assert.Equal(t, importIdentityUnknown, importIdentitySupportFor(json.RawMessage("null"), "unparseable"))
}

func TestPrepareImportToolSingleLogReadAndCarryBlock(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	require.NotNil(t, out.Carry)
	require.Len(t, out.Types, 1)
	assert.Equal(t, importIdentitySupported, out.Types[0].IdentitySupport)
	assert.NotEmpty(t, out.Types[0].ManagedSchema)
	assert.True(t, out.HasCurrentConfiguration)
	assert.Equal(t, "cv-current", out.CurrentConfigurationVersionID)
	assert.Equal(t, importIdentitySupported, out.Carry.Target.IdentitySupport["aws_iam_role"])
	assert.Equal(t, "1.16.1", out.Carry.Target.TerraformVersion)
	assert.Equal(t, importCarryDigest(*out.Carry), out.Carry.SelectionDigest)
	assert.Contains(t, out.NextAction, "Ask the user")
	assert.NotContains(t, out.NextAction, "download_url")

	f.mu.Lock()
	defer f.mu.Unlock()
	for key := range f.requests {
		assert.NotContains(t, key, "/download", "prepare_import must not request the configuration download")
	}

	// A single discovery read is the baseline: the whole prepare adds no log reads.
	baseline := importBackendFixture(t)
	_, err := readImportDiscovery(context.Background(), baseline.client, "qry-fixture")
	require.NoError(t, err)
	baseline.mu.Lock()
	defer baseline.mu.Unlock()
	for _, key := range []string{"GET /api/v2/queries/qry-fixture", "GET /logs"} {
		assert.Equal(t, baseline.requests[key], f.requests[key], key)
	}
	assert.Positive(t, f.requests["GET /logs"])
}

func TestPrepareImportToolCarryCandidateIDIsRecomputable(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	c := out.Carry.Candidates[0]
	p := out.Carry.Providers[c.ListType]
	assert.Equal(t, c.CandidateID, importCandidateID("qry-fixture", workspaceProvider{Source: p.Source, Version: p.Version}, c.ListType, c.Identity))
}

func TestPrepareImportToolIdentitySupportByDestinationVersion(t *testing.T) {
	for _, tt := range []struct {
		name, version, want string
		stripIdentity       bool
	}{
		{"identity in schema", "1.16.1", importIdentitySupported, false},
		{"no identity on 1.12 or later", "1.16.1", importIdentityNone, true},
		{"no identity below 1.12 is undetermined", "1.11.0", importIdentityUnknown, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := importBackendFixture(t)
			f.responses["/api/v2/runs/run-schema"] = json.RawMessage(strings.Replace(string(f.responses["/api/v2/runs/run-schema"]), `"1.16.1"`, `"`+tt.version+`"`, 1))
			if tt.stripIdentity {
				var schema map[string]any
				require.NoError(t, json.Unmarshal(f.responses["/schema-download"], &schema))
				for _, p := range schema["provider_schemas"].(map[string]any) {
					delete(p.(map[string]any), "resource_identity_schemas")
				}
				raw, _ := json.Marshal(schema)
				f.responses["/schema-download"] = raw
			}
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			require.Equal(t, "prepared", out.Status, out.Diagnostics)
			assert.Equal(t, tt.want, out.Types[0].IdentitySupport)
			assert.Equal(t, tt.want, out.Carry.Target.IdentitySupport["aws_iam_role"])
			assert.Equal(t, tt.version, out.Carry.Target.TerraformVersion)
		})
	}
}

func TestPrepareImportToolRejectsTargetAddressAndUnknownCandidate(t *testing.T) {
	f := importBackendFixture(t)
	input := importFixtureInput(t)
	input.Selections[0].CandidateID = "candidate-" + strings.Repeat("0", 64)
	out := prepareImportTool(context.Background(), f.client, input)
	assert.Equal(t, "blocked", out.Status)
	assert.Contains(t, out.Diagnostics, "selected_candidate_not_found")
	assert.Contains(t, out.NextAction, "No CV or Run was created")
}

func TestPrepareImportToolRejectsUnknownAndCreateOnlyFields(t *testing.T) {
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	good := func() map[string]any {
		return map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "query_run_id": "qry-fixture",
			"selections": []any{map[string]any{"candidate_id": importFixtureInput(t).Selections[0].CandidateID, "managed_type": "aws_iam_role"}}}
	}
	for name, mutate := range map[string]func(map[string]any){
		"phase":                 func(a map[string]any) { a["phase"] = "prepare" },
		"baseline":              func(a map[string]any) { a["baseline_cv_id"] = "cv-current" },
		"configuration version": func(a map[string]any) { a["configuration_version_id"] = "cv-current" },
		"target address": func(a map[string]any) {
			a["selections"].([]any)[0].(map[string]any)["target_address"] = "aws_iam_role.x"
		},
		"include_import_...": func(a map[string]any) { a["include_import_candidates"] = true },
	} {
		args := good()
		mutate(args)
		res, err := HandlePrepareImport(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}, silentLogger())
		require.NoError(t, err, name)
		assert.True(t, res.IsError, name)
		assert.Contains(t, res.StructuredContent.(importPrepared).Diagnostics, "import_input_invalid", name)
	}
	res, err := HandlePrepareImport(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: good()}}, silentLogger())
	require.NoError(t, err)
	assert.False(t, res.IsError, res.StructuredContent)
}

func TestPrepareImportToolResponseSizesAndSingleLogRead(t *testing.T) {
	for _, total := range []int{10, 100, 1000} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			f := importBackendFixture(t)
			f.queryLog = largeDiscoveryLog(total)
			discovery, err := readImportDiscovery(context.Background(), f.client, "qry-fixture")
			require.NoError(t, err)
			require.Len(t, discovery.Candidates, total)
			input := importFixtureInput(t)
			input.Selections = nil
			for _, c := range discovery.Candidates[len(discovery.Candidates)-min(total, maxImportSelections):] {
				input.Selections = append(input.Selections, importSelection{CandidateID: c.CandidateID, ManagedType: "aws_iam_role"})
			}
			out := prepareImportTool(context.Background(), f.client, input)
			require.Equal(t, "prepared", out.Status, out.Diagnostics)
			encoded, _ := json.Marshal(out)
			carry, _ := json.Marshal(out.Carry)
			t.Logf("prepare_import, %d results, %d selected: %d bytes (carry block %d bytes)", total, len(input.Selections), len(encoded), len(carry))
			assert.Less(t, len(encoded), maxImportPreparationBytes)
		})
	}
}
