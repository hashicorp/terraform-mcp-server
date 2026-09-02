// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportQueryResultsDefinition(t *testing.T) {
	tool := ImportQueryResults(silentLogger(), nil)

	assert.Equal(t, "import_query_results", tool.Tool.Name)
	assert.ElementsMatch(t, []string{"phase", "organization_name", "workspace_name", "query_run_id"}, tool.Tool.InputSchema.Required)
	assert.Contains(t, tool.Tool.InputSchema.Properties, "configuration_path")
	assert.Contains(t, tool.Tool.InputSchema.Properties, "query_configuration")
	assert.Contains(t, tool.Tool.InputSchema.Properties, "confirm_speculative_run")
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
	assert.Contains(t, tool.Tool.Description, "zero add, change, or destroy")
}

func TestProvidersFromQueryConfiguration(t *testing.T) {
	providers, err := providersFromQueryConfiguration(`{
		"no_code_query_providers": [{
			"namespace": "hashicorp",
			"name": "aws",
			"version": "6.33.0",
			"no_code_query_resources": [{"body": {"resource_type": "aws_instance"}}]
		}]
	}`)

	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Equal(t, workspaceProvider{
		Source:  "registry.terraform.io/hashicorp/aws",
		Name:    "aws",
		Version: "6.33.0",
	}, providers[0])
}

func TestProvidersFromQueryConfigurationAllowsOmission(t *testing.T) {
	providers, err := providersFromQueryConfiguration("")

	require.NoError(t, err)
	assert.Empty(t, providers)
}

func TestParseImportCandidates(t *testing.T) {
	logs := strings.Join([]string{
		`Terraform query initialized`,
		`{"type":"list_start"}`,
		`not JSON`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_instance.example","display_name":"i-123","resource_type":"aws_instance","identity":{"id":"i-123"},"resource_object":{"region":"us-east-1"},"configuration":"resource \"aws_instance\" \"example_0\" {}","import_configuration":"import { to = aws_instance.example_0 id = \"i-123\" }"}}`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_instance.example","display_name":"i-456","resource_type":"aws_instance","identity":{"id":"i-456"},"configuration":"ignored","import_configuration":"ignored"}}`,
	}, "\n")

	candidates, err := parseImportCandidates(strings.NewReader(logs), 1)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "aws_instance", candidates[0].ResourceType)
	assert.Equal(t, "i-123", candidates[0].Identity["id"])
	assert.Contains(t, candidates[0].ImportConfig, "aws_instance.example_0")
}

func TestResolveModuleLayoutAndReadContext(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "infra", "prod")
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(`variable "region" { type = string }`), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(moduleDir, "child"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "child", "main.tf"), []byte(`resource "test" "child" {}`), 0o600))

	layout, err := resolveModuleLayout(root, "infra/prod")
	require.NoError(t, err)
	assert.Equal(t, moduleDir, layout.ModuleDir)
	assert.Empty(t, layout.Warning)

	files, err := readModuleContext(layout.ModuleDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "variables.tf", files[0].Name)
	assert.NotContains(t, files[0].Content, "child")
}

func TestReadWorkspaceProvidersUsesExactWorkspaceMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		assert.Equal(t, "/api/v2/organizations/acme/explorer", r.URL.Path)
		assert.Equal(t, "providers", r.URL.Query().Get("type"))
		assert.Equal(t, "prod", r.URL.Query().Get("filter[0][workspaces][contains][0]"))
		_, _ = io.WriteString(w, `{"data":[
			{"attributes":{"source":"registry.terraform.io/hashicorp/aws","name":"aws","version":"6.1.0","workspaces":"prod, dev"}},
			{"attributes":{"source":"registry.terraform.io/hashicorp/random","name":"random","version":"3.7.0","workspaces":"production"}}
		]}`)
	}))
	defer server.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "token", HTTPClient: server.Client()})
	require.NoError(t, err)

	providers, err := readWorkspaceProviders(context.Background(), client, "acme", "prod")

	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Equal(t, "6.1.0", providers[0].Version)
}

func TestReadWorkspaceProvidersRejectsIncompleteMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"attributes":{"source":"registry.terraform.io/hashicorp/aws","name":"aws","workspaces":"prod"}}]}`)
	}))
	defer server.Close()
	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "token", HTTPClient: server.Client()})
	require.NoError(t, err)

	_, err = readWorkspaceProviders(context.Background(), tfeClient, "acme", "prod")

	assert.ErrorContains(t, err, "incomplete provider metadata")
}

func TestValidateQueryRunWorkspace(t *testing.T) {
	workspace := &tfe.Workspace{ID: "ws-target"}
	assert.NoError(t, validateQueryRunWorkspace(&tfe.QueryRun{Workspace: &tfe.Workspace{ID: "ws-target"}}, workspace))
	assert.ErrorContains(t, validateQueryRunWorkspace(&tfe.QueryRun{}, workspace), "refusing to import")
	assert.ErrorContains(t, validateQueryRunWorkspace(&tfe.QueryRun{Workspace: &tfe.Workspace{ID: "ws-other"}}, workspace), "different workspace")
}

func TestVerifyQueryImportCreatesSpeculativeProvisionalRun(t *testing.T) {
	var server *httptest.Server
	var configurationVersionPayload map[string]any
	var runPayload map[string]any
	uploaded := false
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/workspaces/ws-test/configuration-versions":
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&configurationVersionPayload))
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"data":{"type":"configuration-versions","id":"cv-test","attributes":{"upload-url":%q,"status":"pending"}}}`, server.URL+"/upload")
		case "/upload":
			require.Equal(t, http.MethodPut, r.Method)
			_, err := io.Copy(io.Discard, r.Body)
			require.NoError(t, err)
			uploaded = true
			w.WriteHeader(http.StatusOK)
		case "/api/v2/runs":
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, json.NewDecoder(r.Body).Decode(&runPayload))
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"data":{"type":"runs","id":"run-test","attributes":{"status":"pending"}}}`)
		case "/api/v2/runs/run-test":
			require.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "plan", r.URL.Query().Get("include"))
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = io.WriteString(w, `{"data":{"type":"runs","id":"run-test","attributes":{"status":"planned_and_finished"},"relationships":{"plan":{"data":{"type":"plans","id":"plan-test"}}}}}`)
		case "/api/v2/plans/plan-test":
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = io.WriteString(w, `{"data":{"type":"plans","id":"plan-test","attributes":{"status":"finished","resource-imports":1,"resource-additions":0,"resource-changes":0,"resource-destructions":0}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	tfeClient, err := tfe.NewClient(&tfe.Config{Address: server.URL, Token: "token", HTTPClient: server.Client()})
	require.NoError(t, err)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.tf"), []byte("terraform {}\n"), 0o600))
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"query_run_id":            "qry-test",
		"generated_configuration": "resource \"test_resource\" \"example\" {}\nimport {\n  to = test_resource.example\n  id = \"example\"\n}",
		"confirm_speculative_run": true,
	}}}

	candidates := []importCandidate{{
		ResourceType:  "test_resource",
		Configuration: `resource "test_resource" "example" {}`,
		ImportConfig:  "import {\n  to = test_resource.example\n  id = \"example\"\n}",
	}}
	result, err := verifyQueryImport(context.Background(), request, tfeClient, &tfe.Workspace{ID: "ws-test"}, moduleLayout{UploadRoot: root, ModuleDir: root}, candidates, silentLogger())

	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.True(t, uploaded)
	assert.FileExists(t, filepath.Join(root, "imports.generated.tf"))
	cvAttributes := configurationVersionPayload["data"].(map[string]any)["attributes"].(map[string]any)
	assert.Equal(t, true, cvAttributes["speculative"])
	assert.Equal(t, true, cvAttributes["provisional"])
	assert.Equal(t, false, cvAttributes["auto-queue-runs"])
	runAttributes := runPayload["data"].(map[string]any)["attributes"].(map[string]any)
	assert.Equal(t, true, runAttributes["plan-only"])
	assert.Equal(t, false, runAttributes["allow-config-generation"])
}

func TestValidateGeneratedConfiguration(t *testing.T) {
	candidates := []importCandidate{{
		Configuration: `resource "test_resource" "example" {}`,
		ImportConfig:  "import {\n  to = test_resource.example\n  id = \"expected\"\n}",
	}}

	t.Run("accepts exact candidate binding", func(t *testing.T) {
		configuration := "resource \"test_resource\" \"example\" {}\nimport {\n  to = test_resource.example\n  id = \"expected\"\n}"
		require.NoError(t, validateGeneratedConfiguration(configuration, candidates))
	})

	t.Run("rejects changed identity", func(t *testing.T) {
		configuration := "resource \"test_resource\" \"example\" {}\nimport {\n  to = test_resource.example\n  id = \"different\"\n}"
		assert.ErrorContains(t, validateGeneratedConfiguration(configuration, candidates), "does not match")
	})

	t.Run("rejects malformed HCL", func(t *testing.T) {
		assert.ErrorContains(t, validateGeneratedConfiguration(`resource "test_resource"`, candidates), "invalid Terraform HCL")
	})
}

func TestValidateImportPlan(t *testing.T) {
	assert.NoError(t, validateImportPlan(&tfe.Plan{ID: "plan-safe", ResourceImports: 2}, 2))
	assert.ErrorContains(t, validateImportPlan(&tfe.Plan{ID: "plan-unsafe", ResourceImports: 2, ResourceChanges: 1}, 2), "not import-only")
	assert.ErrorContains(t, validateImportPlan(&tfe.Plan{ID: "plan-short", ResourceImports: 1}, 2), "expected 2")
}

func TestVerifyQueryImportRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "imports.generated.tf")
	require.NoError(t, os.WriteFile(output, []byte("existing"), 0o600))
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"generated_configuration": "resource \"test_resource\" \"example\" {}\nimport {\n  to = test_resource.example\n  id = \"example\"\n}",
		"confirm_speculative_run": true,
	}}}

	candidates := []importCandidate{{
		Configuration: `resource "test_resource" "example" {}`,
		ImportConfig:  "import {\n  to = test_resource.example\n  id = \"example\"\n}",
	}}
	result, err := verifyQueryImport(context.Background(), request, nil, &tfe.Workspace{}, moduleLayout{UploadRoot: root, ModuleDir: root}, candidates, silentLogger())

	require.NoError(t, err)
	assert.True(t, result.IsError)
	content, readErr := os.ReadFile(output)
	require.NoError(t, readErr)
	assert.Equal(t, "existing", string(content))
}
