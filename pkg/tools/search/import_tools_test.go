// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
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
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type importTestSession struct{}

func (importTestSession) Initialize()                                         {}
func (importTestSession) Initialized() bool                                   { return true }
func (importTestSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return nil }
func (importTestSession) SessionID() string                                   { return "" }

// All requests are GETs against a synthetic backend; no CLI or HCP Run is
// started. The Plan JSON contains values that must never be copied into MCP.
func importWireBackend(t *testing.T, status, planStatus string) (*httptest.Server, *int) {
	return importWireBackendForListType(t, status, planStatus, "aws_s3_buckets")
}

func importWireBackendForListType(t *testing.T, status, planStatus, listType string) (*httptest.Server, *int) {
	return importWireBackendWithPlan(t, status, planStatus, listType, "")
}

func importWireBackendWithPlan(t *testing.T, status, planStatus, listType, planJSON string) (*httptest.Server, *int) {
	t.Helper()
	planJSONReads := new(int)
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
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-example","attributes":{"status":"finished","log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-source"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs?X-Amz-Signature=signed-secret-sentinel")
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
		case "/api/v2/runs/run-example":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"runs","id":"run-example","attributes":{"status":%q,"plan-only":true,"auto-apply":false},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-example"}},"plan":{"data":{"type":"plans","id":"plan-example"}},"configuration-version":{"data":{"type":"configuration-versions","id":"cv-example"}}}}}`, status)
		case "/api/v2/plans/plan-example":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"plans","id":"plan-example","attributes":{"status":%q}}}`, planStatus)
		case "/api/v2/plans/plan-example/json-output":
			*planJSONReads++
			if planJSON == "" {
				planJSON = `{"format_version":"1.2","terraform_version":"1.14.0","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[{"address":"aws_s3_bucket.sample","identity_schema_version":0}]}},"resource_changes":[{"address":"aws_s3_bucket.sample","mode":"managed","type":"aws_s3_bucket","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"private-import-id-sentinel"},"after_identity":{"bucket":"sample"}}}],"output_changes":{"stable":{"actions":["no-op"]}},"deferred_changes":[],"resource_drift":[]}`
			}
			_, _ = fmt.Fprint(w, planJSON)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(backend.Close)
	return backend, planJSONReads
}

func TestImportToolsEndToEndLocalHTTPAndLegacyWire(t *testing.T) {
	backend, reads := importWireBackend(t, "planned_and_finished", "finished")
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	carry, bindings, _ := importTestCarry(t)
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	prep, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example", Selections: []ImportSelection{{QueryResultMarker: bindings[0].QueryResultMarker, ManagedType: "aws_s3_bucket"}}, LockedProviders: carry.LockedProviders})
	require.NoError(t, err)
	require.Equal(t, "prepared", prep.Status)
	require.Equal(t, "source", prep.SourceWorkspaceName)
	require.NotEmpty(t, prep.Carry.SelectionDigest)
	require.Equal(t, "unknown", prep.Types[0].SchemaStatus)
	require.Equal(t, "6.66.0", prep.Types[0].LockedProviderVersion)
	require.Equal(t, "6.66.0", prep.Types[0].SearchProviderVersion)
	result, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"})
	require.NoError(t, err)
	require.Equal(t, "carry_required", result.Status)
	require.Zero(t, *reads)
	result, err = ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", Carry: prep.Carry, Bindings: bindings, DetailAddresses: []string{"aws_s3_bucket.sample"}})
	require.NoError(t, err)
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, 1, result.Changes["none"])
	require.Equal(t, 1, result.ObjectIdentity["matched"])
	require.Equal(t, 1, *reads)
	require.Len(t, result.Detail, 1)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-import-id-sentinel")
	require.NotContains(t, string(encoded), "overall")
	// A real mark3labs CallToolRequest must preserve the same structured/text
	// response, not merely the plain-Go helper's result.
	args := map[string]any{"organization_name": "org", "workspace_name": "target", "run_id": "run-example", "carry": prep.Carry, "bindings": bindings}
	ctx := server.NewMCPServer("test", "0").WithContext(context.Background(), importTestSession{})
	wire, err := VerifyImportPlan(silentLogger()).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
	require.NoError(t, err)
	require.False(t, wire.IsError, "tool content: %#v", wire.Content)
	require.Contains(t, fmt.Sprint(wire.StructuredContent), "plan_available")
	text, ok := mcp.AsTextContent(wire.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"plan_available"`)
	require.NotContains(t, text.Text, "private-import-id-sentinel")
	prepareArgs := map[string]any{"organization_name": "org", "workspace_name": "target", "query_run_id": "qry-example", "locked_providers": prep.Carry.LockedProviders, "selections": []ImportSelection{{QueryResultMarker: bindings[0].QueryResultMarker, ManagedType: "aws_s3_bucket"}}}
	var logged bytes.Buffer
	logger := log.New()
	logger.SetOutput(&logged)
	wire, err = PrepareImport(logger).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: prepareArgs}})
	require.NoError(t, err)
	require.False(t, wire.IsError, "tool content: %#v", wire.Content)
	require.Contains(t, fmt.Sprint(wire.StructuredContent), "prepared")
	require.Contains(t, fmt.Sprint(wire.Content), "selection_digest")
	require.NotContains(t, logged.String(), "signed-secret-sentinel")
	require.NotContains(t, fmt.Sprint(wire.Content), "signed-secret-sentinel")
}

func TestPrepareImportKeepsSameNameSearchListAndManagedType(t *testing.T) {
	backend, _ := importWireBackendForListType(t, "planned_and_finished", "finished", "aws_sqs_queue")
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
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
	require.Equal(t, "aws_sqs_queue", prepared.Carry.SelectedResults[0].ListType)
	require.Equal(t, "aws_sqs_queue", prepared.Carry.SelectedResults[0].ManagedType)
	require.Equal(t, "6.66.0", prepared.Types[0].SearchProviderVersion)
	require.Equal(t, "6.18.0", prepared.Types[0].LockedProviderVersion)
	require.Equal(t, "unknown", prepared.Types[0].ManagedTypeSupport, "no historical target schema was read")
	require.Contains(t, prepared.NextAction, "proposed managed type is not proved")
	for _, hint := range []string{"TF_CLOUD_HOSTNAME", "TF_CLOUD_ORGANIZATION", "TF_CLOUD_PROJECT", "TF_WORKSPACE", "Explicit cloud block values take precedence", "credentials for that host", "terraform plan -no-color -input=false", "plan_only=true"} {
		require.Contains(t, prepared.NextAction, hint)
	}
	for _, hint := range []string{"TFE_ADDRESS", "TFE_HOSTNAME", "TFE_TOKEN", "do not establish CLI cloud routing or authentication"} {
		require.Contains(t, prepared.NextAction, hint)
	}
	require.NotContains(t, prepared.NextAction, ".envrc")
	require.Contains(t, PrepareImportDescription, "cloud block values take precedence")
	require.Contains(t, PrepareImportDescription, "TFE_TOKEN may configure terraform-mcp-server")

	input.Selections[0].ManagedType = "google_sqs_queue"
	blocked, err := ReadPrepareImport(context.Background(), c, input)
	require.Nil(t, blocked)
	require.EqualError(t, err, "selection_invalid", "provider-source mismatch still fails")

	input.Selections[0].ManagedType = "aws_sqs_queue"
	input.Selections[0].QueryResultMarker, err = queryResultMarker("qry-other", provider, "aws_sqs_queue", identity)
	require.NoError(t, err)
	blocked, err = ReadPrepareImport(context.Background(), c, input)
	require.Nil(t, blocked)
	require.EqualError(t, err, "selection_invalid", "unknown QueryRun marker still fails")
}

// A finished speculative Run may import and also update cloud settings. The
// import marker and update action are independent of object identity and
// neither makes the Plan safe to apply.
func TestVerifyImportPlanSQSImportWithUpdate(t *testing.T) {
	planJSON := `{"format_version":"1.2","terraform_version":"1.17.0-dev.1","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[{"address":"aws_sqs_queue.unmanaged","identity_schema_version":0}]}},"resource_changes":[{"address":"aws_sqs_queue.unmanaged","mode":"managed","type":"aws_sqs_queue","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["update"],"importing":{"id":"private-queue-url-sentinel"},"before":{"max_message_size":100,"tags":{"private":"secret-sentinel"}},"after":{"max_message_size":200,"tags":{}},"after_identity":{"url":"https://example.invalid/queue"}}}],"output_changes":{},"deferred_changes":[],"resource_drift":[]}`
	backend, reads := importWireBackendWithPlan(t, "planned_and_finished", "finished", "aws_sqs_queue", planJSON)
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	marker, err := queryResultMarker("qry-example", provider, "aws_sqs_queue", map[string]any{"url": "https://example.invalid/queue"})
	require.NoError(t, err)
	prep, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_sqs_queue"}},
		LockedProviders: map[string]string{provider.Source: "6.18.0"},
	})
	require.NoError(t, err)
	input := VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", Carry: prep.Carry,
		Bindings: []ImportBinding{{QueryResultMarker: marker, TargetAddress: "aws_sqs_queue.unmanaged"}}, DetailAddresses: []string{"aws_sqs_queue.unmanaged"}}
	result, err := ReadVerifyImportPlan(context.Background(), c, input)
	require.NoError(t, err)
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, "1.2", result.Plan.FormatVersion)
	require.Equal(t, "1.17.0-dev.1", result.Plan.TerraformVersion)
	require.Equal(t, 1, result.Changes["update"])
	require.Equal(t, 1, result.ObjectIdentity["matched"], "identity classification must not mask the update")
	require.Equal(t, []string{"update"}, result.Detail[0].Actions)
	require.Contains(t, result.Diagnostics, "plan_actions_require_review")
	require.Contains(t, result.NextAction, "not safe to apply")
	require.Equal(t, 1, *reads)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	for _, secret := range []string{"private-queue-url-sentinel", "secret-sentinel", "https://example.invalid/queue"} {
		require.NotContains(t, string(encoded), secret)
	}

	// The same Run cannot be classified when a preflight field is absent or
	// contradictory. Expose only a static reason, never raw Plan values.
	for _, tt := range []struct{ name, payload, reason string }{
		{"missing Terraform version", strings.Replace(planJSON, `"terraform_version":"1.17.0-dev.1",`, "", 1), "plan_terraform_version_unavailable"},
		{"incomplete JSON", strings.Replace(planJSON, `"complete":true`, `"complete":false`, 1), "plan_json_incomplete"},
		{"errored JSON", strings.Replace(planJSON, `"errored":false`, `"errored":true`, 1), "plan_json_errored"},
		{"unsupported format", strings.Replace(planJSON, `"format_version":"1.2"`, `"format_version":"2.0"`, 1), "plan_format_unsupported"},
		{"old Terraform", strings.Replace(planJSON, `"terraform_version":"1.17.0-dev.1"`, `"terraform_version":"1.4.9-dev.1"`, 1), "plan_terraform_version_unsupported"},
		{"malformed Terraform version", strings.Replace(planJSON, `"terraform_version":"1.17.0-dev.1"`, `"terraform_version":"1.17.0 arbitrary-private-sentinel"`, 1), "plan_terraform_version_unavailable"},
		{"invalid JSON", planJSON + `invalid-private-sentinel`, "plan_json_invalid"},
		{"deep JSON", `{"nested":` + strings.Repeat("[", 65) + `"private-sentinel"` + strings.Repeat("]", 65) + `}`, "plan_json_depth_limit"},
		{"oversized JSON", `{"padding":"` + strings.Repeat("x", 32<<20) + `"}`, "plan_json_size_limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixtureServer, _ := importWireBackendWithPlan(t, "planned_and_finished", "finished", "aws_sqs_queue", tt.payload)
			client, err := tfe.NewClient(&tfe.Config{Address: fixtureServer.URL, Token: "test-token", HTTPClient: fixtureServer.Client()})
			require.NoError(t, err)
			blocked, err := ReadVerifyImportPlan(context.Background(), client, input)
			require.NoError(t, err)
			require.Equal(t, "blocked", blocked.Status)
			require.Equal(t, []string{tt.reason}, blocked.Diagnostics)
			require.Equal(t, "run-example", blocked.RunID)
			require.Equal(t, "plan-example", blocked.PlanID)
			require.Nil(t, blocked.Plan)
			resultJSON, err := json.Marshal(blocked)
			require.NoError(t, err)
			require.NotContains(t, string(resultJSON), "private-queue-url-sentinel")
			require.NotContains(t, string(resultJSON), "secret-sentinel")
			if tt.reason == "plan_terraform_version_unavailable" {
				t.Setenv("TFE_ADDRESS", fixtureServer.URL)
				t.Setenv("TFE_TOKEN", "test-token")
				ctx := server.NewMCPServer("test", "0").WithContext(context.Background(), importTestSession{})
				wire, err := VerifyImportPlan(silentLogger()).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: input}})
				require.NoError(t, err)
				require.False(t, wire.IsError)
				require.Contains(t, fmt.Sprint(wire.StructuredContent), tt.reason)
				text, ok := mcp.AsTextContent(wire.Content[0])
				require.True(t, ok)
				require.Contains(t, text.Text, tt.reason)
				require.NotContains(t, text.Text, "private-queue-url-sentinel")
			}
		})
	}
}

func TestTerraformImportSyntaxVersionPrereleaseAndMalformed(t *testing.T) {
	for _, tt := range []struct {
		version    string
		old, known bool
	}{
		{"1.14.0", false, true},
		{"v1.17.0-dev.20261012+build.1", false, true},
		{"1.4.9-dev.1", true, true},
		{"1.5.0", false, true},
		{"1.17.0 arbitrary", false, false},
		{"1.17.0-", false, false},
		{"1.17.0/secret", false, false},
		{"1.999999999999999999999999999.0", false, false},
	} {
		t.Run(tt.version, func(t *testing.T) {
			old, known := terraformImportSyntaxVersion(tt.version)
			require.Equal(t, tt.old, old)
			require.Equal(t, tt.known, known)
		})
	}
}

func TestPrepareImportAllowsCallerProposedDifferentNameWithoutClaimingSupport(t *testing.T) {
	backend, _ := importWireBackend(t, "planned_and_finished", "finished")
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	marker, err := queryResultMarker("qry-example", provider, "aws_s3_buckets", map[string]any{"bucket": "sample"})
	require.NoError(t, err)
	prepared, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_s3_bucket_logging"}},
		LockedProviders: map[string]string{provider.Source: "6.18.0"},
	})
	require.NoError(t, err)
	require.Equal(t, "prepared", prepared.Status)
	require.Equal(t, "aws_s3_buckets", prepared.Carry.SelectedResults[0].ListType)
	require.Equal(t, "aws_s3_bucket_logging", prepared.Carry.SelectedResults[0].ManagedType)
	require.Equal(t, "unknown", prepared.Types[0].ManagedTypeSupport, "different-name pairing is not proof of target support")
}

func TestImportToolUnknownArgumentFailsWithoutBackend(t *testing.T) {
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"run_id": "run-example", "configuration_version_id": "cv-forbidden"}}}
	result, err := VerifyImportPlan(silentLogger()).Handler(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.IsError)
}

func TestVerifyImportStopsOnFailedPlanWhileRunLags(t *testing.T) {
	backend, reads := importWireBackend(t, "planning", "errored")
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	result, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"})
	require.NoError(t, err)
	require.Equal(t, "failed", result.Status)
	require.Equal(t, []string{"plan_failed"}, result.Diagnostics)
	require.Equal(t, "errored", result.PlanStatus)
	require.Zero(t, *reads)
}

func TestImportToolDescriptionsAndSchemas(t *testing.T) {
	for _, tool := range []mcp.Tool{PrepareImport(silentLogger()).Tool, VerifyImportPlan(silentLogger()).Tool} {
		require.True(t, *tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
		require.NotNil(t, tool.OutputSchema)
		require.Contains(t, tool.Description, "CLI")
	}
	prepare := PrepareImport(silentLogger()).Tool
	require.Contains(t, prepare.InputSchema.Required, "selections")
	require.Contains(t, prepare.InputSchema.Required, "locked_providers")
	require.Contains(t, prepare.OutputSchema.Properties, "carry")
	verify := VerifyImportPlan(silentLogger()).Tool
	require.Contains(t, verify.InputSchema.Required, "run_id")
	require.NotContains(t, verify.InputSchema.Required, "carry")
	require.Contains(t, verify.OutputSchema.Properties, "object_identity")
}
