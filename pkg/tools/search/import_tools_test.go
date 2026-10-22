// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

type importTestSession struct{}

func TestPrepareImportInputSchema(t *testing.T) {
	prepare := PrepareImport(silentLogger())
	require.NotContains(t, prepare.Tool.InputSchema.Required, ScopeOrganizationInput)
	require.Contains(t, prepare.Tool.InputSchema.Properties, ScopeOrganizationInput)
	require.NotContains(t, prepare.Tool.InputSchema.Properties, "organization_name")
	require.NotContains(t, prepare.Tool.InputSchema.Required, "workspace_name")
	require.Contains(t, prepare.Tool.InputSchema.Required, "query_run_id")
}

func TestImportToolsRequestMissingTargetScopeBeforeClientUse(t *testing.T) {
	for _, name := range []string{"TF_CLOUD_ORGANIZATION", "TF_MCP_QUERY_WORKSPACE", "TF_MCP_IMPORT_WORKSPACE", "TF_WORKSPACE"} {
		t.Setenv(name, "")
	}
	for _, tool := range []server.ServerTool{PrepareImport(silentLogger())} {
		t.Run(tool.Tool.Name, func(t *testing.T) {
			result, err := tool.Handler(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{}}})
			require.NoError(t, err)
			require.False(t, result.IsError)
			text, ok := mcp.AsTextContent(result.Content[0])
			require.True(t, ok)
			require.Contains(t, text.Text, `"status":"input_required"`)
			require.Contains(t, text.Text, `"missing_inputs":["terraform_org_name","workspace_name"]`)
		})
	}
}

func (importTestSession) Initialize()                                         {}
func (importTestSession) Initialized() bool                                   { return true }
func (importTestSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }
func (importTestSession) SessionID() string                                   { return "" }

// All requests are GETs against a synthetic backend; no CLI or HCP Run is
// started. Ordinary Run/Plan routes are deferred to PR6.
func importWireBackend(t *testing.T) *httptest.Server {
	return importWireBackendForListType(t, "aws_s3_buckets")
}

func importWireBackendForListType(t *testing.T, listType string) *httptest.Server {
	return importWireBackendWithProvider(t, listType, "hashicorp", "aws")
}

func importWireBackendWithProvider(t *testing.T, listType, providerNamespace, providerName string) *httptest.Server {
	t.Helper()
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected mutation: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-example":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-example","attributes":{"status":"finished","generate-config-out":true,"log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-source"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs?X-Amz-Signature=signed-secret-sentinel")
		case "/api/v2/workspaces/ws-source":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-source","attributes":{"name":"source"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprintf(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":%q}}]}]},"relationships":{"workspace":{"data":{"id":"ws-source"}}}}}`, listType)
		case "/logs":
			address := "list." + listType + ".all"
			identity := map[string]any{"bucket": "sample"}
			if listType == "aws_sqs_queue" {
				identity = map[string]any{"url": "https://example.invalid/queue"}
			}
			found := map[string]any{"address": address, "resource_type": listType, "display_name": "sample", "identity": identity, "identity_version": 0}
			if listType == "aws_s3_buckets" {
				found["config"] = `resource "aws_s3_bucket" "sample" {}`
				found["import_config"] = `import { to = aws_s3_bucket.sample id = "review-before-use" }`
				found["resource_object"] = map[string]any{
					"password": "resource-object-secret-sentinel",
					"tags_all": map[string]any{"inherited": "yes"},
					"tags": map[string]any{
						"team":   "search",
						"nested": map[string]any{"secret": "nested-tag-secret-sentinel"},
					},
				}
			}
			foundJSON, _ := json.Marshal(map[string]any{"type": "list_resource_found", "list_resource_found": found})
			completeJSON, _ := json.Marshal(map[string]any{"type": "list_complete", "list_complete": map[string]any{"address": address, "resource_type": listType, "total": 1}})
			body := []byte("\x02Starting query\n" + `{"type":"provider_installation_start","provider":"aws"}` + "\n" + string(foundJSON) + "\n" + string(completeJSON) + "\n\x03")
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(body) {
				_, _ = w.Write(body[offset:min(len(body), offset+limit)])
			}
		case "/api/v2/organizations/org/workspaces/target":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-example","attributes":{"name":"target","execution-mode":"remote","terraform-version":"1.14.0"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/workspaces/ws-example/current-state-version":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected preparation read: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(backend.Close)
	return backend
}

func TestPrepareImportLocalHTTPAndLegacyWire(t *testing.T) {
	backend := importWireBackend(t)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("TF_CLOUD_ORGANIZATION", "org")
	t.Setenv("TF_MCP_IMPORT_WORKSPACE", "target")
	t.Setenv("TF_MCP_QUERY_WORKSPACE", "source-must-not-be-import-target")
	t.Setenv("TF_WORKSPACE", "fallback-must-not-win")
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	marker, err := queryResultMarker("qry-example", provider, "aws_s3_buckets", map[string]any{"bucket": "sample"})
	require.NoError(t, err)
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	prep, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example", Selections: []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_s3_bucket"}}, LockedProviders: map[string]string{provider.Source: "6.66.0"}})
	require.NoError(t, err)
	require.Equal(t, "prepared", prep.Status)
	require.Equal(t, "source", prep.SourceWorkspaceName)
	require.NotEmpty(t, prep.ImportContext.SelectionDigest)
	require.Equal(t, "unknown", prep.Types[0].SchemaStatus)
	require.Equal(t, "6.66.0", prep.Types[0].LockedProviderVersion)
	require.Equal(t, "6.66.0", prep.Types[0].SearchProviderVersion)
	require.Len(t, prep.SelectedResults, 1)
	require.True(t, prep.GenerateConfigOut)
	require.True(t, prep.SelectedResults[0].ConfigurationGenerated)
	require.Equal(t, `resource "aws_s3_bucket" "sample" {}`, prep.SelectedResults[0].Configuration)
	require.Equal(t, `import { to = aws_s3_bucket.sample id = "review-before-use" }`, prep.SelectedResults[0].ImportConfig)
	preparedJSON, err := json.Marshal(prep)
	require.NoError(t, err)
	require.NotContains(t, string(preparedJSON), "configuration_attributes")
	require.NotContains(t, string(preparedJSON), "resource_object")
	require.NotContains(t, string(preparedJSON), "resource-object-secret-sentinel")
	require.NotContains(t, string(preparedJSON), "nested-tag-secret-sentinel")
	// A real mark3labs CallToolRequest must preserve the same structured/text
	// response, not merely the plain-Go helper's result.
	ctx := server.NewMCPServer("test", "0").WithContext(context.Background(), importTestSession{})
	prepareArgs := map[string]any{ScopeOrganizationInput: "org", ScopeWorkspaceInput: "target", "query_run_id": "qry-example", "locked_providers": prep.ImportContext.LockedProviders, "selections": []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_s3_bucket"}}}
	wire, err := PrepareImport(silentLogger()).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: prepareArgs}})
	require.NoError(t, err)
	require.False(t, wire.IsError, "tool content: %#v", wire.Content)
	require.Contains(t, fmt.Sprint(wire.StructuredContent), "prepared")
	require.Contains(t, fmt.Sprint(wire.Content), "selection_digest")
	require.NotContains(t, fmt.Sprint(wire.Content), "signed-secret-sentinel")
	require.NotContains(t, fmt.Sprint(wire.StructuredContent), "resource-object-secret-sentinel")
	require.NotContains(t, fmt.Sprint(wire.Content), "resource-object-secret-sentinel")
	require.NotContains(t, fmt.Sprint(wire.Content), "nested-tag-secret-sentinel")
}

func TestPrepareImportCanonicalizesMixedCaseQueryProviderThroughLock(t *testing.T) {
	backend := importWireBackendWithProvider(t, "aws_s3_buckets", "HashiCorp", "AWS")
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)

	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	identity := map[string]any{"bucket": "sample"}
	marker, err := queryResultMarker("qry-example", provider, "aws_s3_buckets", identity)
	require.NoError(t, err)

	prepared, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{
		OrganizationName: "org",
		WorkspaceName:    "target",
		QueryRunID:       "qry-example",
		Selections:       []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_s3_bucket"}},
		LockedProviders:  map[string]string{provider.Source: "6.66.0"},
	})
	require.NoError(t, err)
	require.Equal(t, "prepared", prepared.Status)
	require.Equal(t, provider.Source, prepared.Types[0].ProviderSource)
	require.Equal(t, provider, prepared.ImportContext.Providers["aws_s3_buckets"])
	require.Equal(t, map[string]string{provider.Source: "6.66.0"}, prepared.ImportContext.LockedProviders)

}

func TestPrepareImportKeepsSameNameSearchListAndManagedType(t *testing.T) {
	backend := importWireBackendForListType(t, "aws_sqs_queue")
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	identity := map[string]any{"url": "https://example.invalid/queue"}
	marker, err := queryResultMarker("qry-example", provider, "aws_sqs_queue", identity)
	require.NoError(t, err)
	input := PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_sqs_queue"}},
		LockedProviders: map[string]string{provider.Source: "6.18.0"},
	}
	prepared, err := ReadPrepareImport(context.Background(), c, input)
	require.NoError(t, err)
	require.Equal(t, "prepared", prepared.Status)
	require.Len(t, prepared.SelectedResults, 1)
	require.Equal(t, "aws_sqs_queue", prepared.SelectedResults[0].ResourceType)
	require.Equal(t, "aws_sqs_queue", prepared.ImportContext.SelectedResults[0].ListType)
	require.Equal(t, "aws_sqs_queue", prepared.ImportContext.SelectedResults[0].ManagedType)
	require.Equal(t, "6.66.0", prepared.Types[0].SearchProviderVersion)
	require.Equal(t, "6.18.0", prepared.Types[0].LockedProviderVersion)
	require.Equal(t, "unknown", prepared.Types[0].ManagedTypeSupport, "no historical target schema was read")
	for _, hint := range []string{"confirmed existing local tree", "exact locked provider releases", "effective provider scope", "returned drafts and historical schema", "review changed files"} {
		require.Contains(t, prepared.NextAction, hint)
	}
	require.Contains(t, prepared.NextAction, "speculative Plan requires separate approval")
	for _, forbidden := range []string{"terraform plan -no-color", "get_plan_json_output", "plan_only=true", "authentication", "Run URL", "terraform apply"} {
		require.NotContains(t, prepared.NextAction, forbidden)
	}
	for _, guidance := range []string{prepared.NextAction, PrepareImportDescription} {
		for _, name := range []string{"TF_TOKEN_", "TFE_ADDRESS", "TFE_HOSTNAME", "TFE_TOKEN"} {
			require.NotContains(t, guidance, name)
		}
	}
	for _, name := range []string{"TF_CLOUD_", "TF_WORKSPACE", "TF_MCP_IMPORT_WORKSPACE"} {
		require.NotContains(t, prepared.NextAction, name, "tool output must not prescribe CLI environment overrides")
	}
	require.Contains(t, PrepareImportDescription, "TF_MCP_IMPORT_WORKSPACE")
	require.NotContains(t, prepared.NextAction, ".envrc")
	require.Contains(t, PrepareImportDescription, "does not read")
	require.Contains(t, PrepareImportDescription, "exact provider releases")
	require.Contains(t, PrepareImportDescription, "custom/private sources require")
	require.Contains(t, PrepareImportDescription, "Search list types do not prove managed types")

	input.LockedProviders = map[string]string{"registry.example.com/hashicorp/aws": "6.18.0"}
	blocked, err := ReadPrepareImport(context.Background(), c, input)
	require.Nil(t, blocked)
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, `"registry.terraform.io/hashicorp/aws"`)
	input.LockedProviders = map[string]string{provider.Source: "6.18.0"}

	input.Selections[0].ManagedType = "google_sqs_queue"
	blocked, err = ReadPrepareImport(context.Background(), c, input)
	require.Nil(t, blocked)
	require.EqualError(t, err, "selection_invalid", "provider-source mismatch still fails")

	input.Selections[0].ManagedType = "aws_sqs_queue"
	input.Selections[0].QueryResultMarker, err = queryResultMarker("qry-other", provider, "aws_sqs_queue", identity)
	require.NoError(t, err)
	blocked, err = ReadPrepareImport(context.Background(), c, input)
	require.Nil(t, blocked)
	require.EqualError(t, err, "selection_invalid", "unknown QueryRun marker still fails")
}

func TestPrepareImportAllowsCallerProposedDifferentNameWithoutClaimingSupport(t *testing.T) {
	backend := importWireBackend(t)
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	marker, err := queryResultMarker("qry-example", provider, "aws_s3_buckets", map[string]any{"bucket": "sample"})
	require.NoError(t, err)
	prepared, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_s3_bucket_logging"}},
		LockedProviders: map[string]string{"hashicorp/aws": "6.18.0"},
	})
	require.NoError(t, err)
	require.Equal(t, "prepared", prepared.Status)
	require.Equal(t, "aws_s3_buckets", prepared.ImportContext.SelectedResults[0].ListType)
	require.Equal(t, "aws_s3_bucket_logging", prepared.ImportContext.SelectedResults[0].ManagedType)
	require.Equal(t, "unknown", prepared.Types[0].ManagedTypeSupport, "different-name pairing is not proof of target support")
	require.Equal(t, map[string]string{provider.Source: "6.18.0"}, prepared.ImportContext.LockedProviders)
}

func TestPrepareImportInvalidProviderSourceIsActionableOnMark3labsWire(t *testing.T) {
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		ScopeOrganizationInput: "org", ScopeWorkspaceInput: "target", "query_run_id": "qry-example",
		"selections":       []any{map[string]any{"query_result_marker": "qrm1-" + strings.Repeat("a", 64), "managed_type": "aws_s3_bucket"}},
		"locked_providers": map[string]any{"aws": "6.18.0"},
	}}}
	result, err := PrepareImport(silentLogger()).Handler(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Nil(t, result.StructuredContent)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, "locked_provider_source_invalid")
	require.Contains(t, text.Text, `locked_providers key "aws"`)
	require.Contains(t, text.Text, "lacks a provider namespace")
}

func TestImportToolDescriptionsAndSchemas(t *testing.T) {
	for _, tool := range []mcp.Tool{PrepareImport(silentLogger()).Tool} {
		require.True(t, *tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
		require.NotNil(t, tool.OutputSchema)
	}
	prepare := PrepareImport(silentLogger()).Tool
	require.Contains(t, prepare.InputSchema.Required, "selections")
	require.Contains(t, prepare.InputSchema.Required, "locked_providers")
	lockedProvidersSchema := prepare.InputSchema.Properties["locked_providers"].(map[string]any)
	require.Contains(t, lockedProvidersSchema["description"], "namespace/type shorthand")
	require.Contains(t, lockedProvidersSchema["description"], "custom/private sources require hostname/namespace/type")
	require.Contains(t, prepare.OutputSchema.Properties, "import_context")
	prepareOutputSchema, err := json.Marshal(prepare.OutputSchema)
	require.NoError(t, err)
	require.NotContains(t, string(prepareOutputSchema), "resource_object")
	require.Contains(t, string(prepareOutputSchema), "configuration_generated")
	require.Contains(t, string(prepareOutputSchema), "generate_config_out")
	require.NotContains(t, string(prepareOutputSchema), "configuration_attributes")
	require.NotContains(t, string(prepareOutputSchema), "attributes_captured")
	require.NotContains(t, string(prepare.RawInputSchema), `"maxItems":100`)
	require.NotContains(t, prepare.Description, "1–100")
}
