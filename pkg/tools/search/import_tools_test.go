// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

type importTestSession struct{}

func TestVerifyImportPlanInputSchemaMatchesOfficialSDK(t *testing.T) {
	legacy := VerifyImportPlan(silentLogger())
	require.NotEmpty(t, legacy.Tool.RawInputSchema)
	official, err := jsonschema.For[VerifyImportInput](nil)
	require.NoError(t, err)
	officialJSON, err := json.Marshal(official)
	require.NoError(t, err)
	require.JSONEq(t, string(officialJSON), string(legacy.Tool.RawInputSchema))
}

type importRewriteTransport struct {
	backend *url.URL
	next    http.RoundTripper
}

func (tr importRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme, copy.URL.Host, copy.Host = tr.backend.Scheme, tr.backend.Host, tr.backend.Host
	return tr.next.RoundTrip(copy)
}

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

func requireImportReportGuidance(t *testing.T, next string) {
	t.Helper()
	sections := []string{
		"first summarize selected imports with both change=none and object_identity=matched; group them if numerous",
		"Then report selected Plan changes by action",
		"Separately group identities that are not matched by status and identity_reason",
	}
	for _, section := range sections {
		require.Contains(t, next, section)
	}
	require.Less(t, strings.Index(next, sections[0]), strings.Index(next, sections[1]))
	require.Less(t, strings.Index(next, sections[1]), strings.Index(next, sections[2]))
	for _, boundary := range []string{
		"Unverified is neither a match nor proof of a mismatch",
		"Keep unselected actions, outputs and drift separate",
		"An import-only Plan or N-to-import summary is not identity proof",
		"separate approval for a new speculative plan",
		"After full-Plan review",
		"Do not run terraform apply, terraform import or apply_run",
	} {
		require.Contains(t, next, boundary)
	}
}

func TestImportToolsEndToEndLocalHTTPAndLegacyWire(t *testing.T) {
	backend, reads := importWireBackend(t, "planned_and_finished", "finished")
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	importContext, bindings, _ := importTestImportContext(t)
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	prep, err := ReadPrepareImport(context.Background(), c, PrepareImportInput{OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example", Selections: []ImportSelection{{QueryResultMarker: bindings[0].QueryResultMarker, ManagedType: "aws_s3_bucket"}}, LockedProviders: importContext.LockedProviders})
	require.NoError(t, err)
	require.Equal(t, "prepared", prep.Status)
	require.Equal(t, "source", prep.SourceWorkspaceName)
	require.NotEmpty(t, prep.ImportContext.SelectionDigest)
	require.Equal(t, "unknown", prep.Types[0].SchemaStatus)
	require.Equal(t, "6.66.0", prep.Types[0].LockedProviderVersion)
	require.Equal(t, "6.66.0", prep.Types[0].SearchProviderVersion)
	result, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example"})
	require.NoError(t, err)
	require.Equal(t, "import_context_required", result.Status)
	require.Zero(t, *reads)
	result, err = ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", ImportContext: prep.ImportContext, Bindings: bindings, DetailAddresses: []string{"aws_s3_bucket.sample"}})
	require.NoError(t, err)
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, 1, result.Changes["none"])
	require.Equal(t, 1, result.ObjectIdentity["matched"])
	require.Empty(t, result.Attention, "matched no-op imports do not need selected-result attention")
	require.Empty(t, result.RunUIURL, "do not advertise a browser link for an HTTP-only mock API")
	require.Contains(t, result.NextAction, "get_plan_json_output")
	require.Contains(t, result.NextAction, "raw Plan JSON may contain sensitive values")
	require.Contains(t, result.NextAction, "outside this session")
	requireImportReportGuidance(t, result.NextAction)
	require.Equal(t, 1, *reads)
	require.Len(t, result.Detail, 1)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"attention"`)
	require.NotContains(t, string(encoded), "private-import-id-sentinel")
	require.NotContains(t, string(encoded), "overall")
	// A real mark3labs CallToolRequest must preserve the same structured/text
	// response, not merely the plain-Go helper's result.
	args := map[string]any{"organization_name": "org", "workspace_name": "target", "run_id": "run-example", "import_context": prep.ImportContext, "bindings": bindings}
	ctx := server.NewMCPServer("test", "0").WithContext(context.Background(), importTestSession{})
	wire, err := VerifyImportPlan(silentLogger()).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
	require.NoError(t, err)
	require.False(t, wire.IsError, "tool content: %#v", wire.Content)
	require.Contains(t, fmt.Sprint(wire.StructuredContent), "plan_available")
	text, ok := mcp.AsTextContent(wire.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"plan_available"`)
	require.NotContains(t, text.Text, `"attention"`)
	require.NotContains(t, text.Text, "private-import-id-sentinel")
	prepareArgs := map[string]any{"organization_name": "org", "workspace_name": "target", "query_run_id": "qry-example", "locked_providers": prep.ImportContext.LockedProviders, "selections": []ImportSelection{{QueryResultMarker: bindings[0].QueryResultMarker, ManagedType: "aws_s3_bucket"}}}
	wire, err = PrepareImport(silentLogger()).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: prepareArgs}})
	require.NoError(t, err)
	require.False(t, wire.IsError, "tool content: %#v", wire.Content)
	require.Contains(t, fmt.Sprint(wire.StructuredContent), "prepared")
	require.Contains(t, fmt.Sprint(wire.Content), "selection_digest")
	require.NotContains(t, fmt.Sprint(wire.Content), "signed-secret-sentinel")
}

func TestImportRunUIURLOnlyForKnownHTTPSBrowserHosts(t *testing.T) {
	for _, tc := range []struct {
		base, org, workspace, runID, want string
	}{
		{"https://app.terraform.io/api/v2/", "my-org", "my_workspace", "run-example", "https://app.terraform.io/app/my-org/my_workspace/runs/run-example"},
		{"https://app.staging.terraform.io/api/v2/", "my-org", "my_workspace", "run-example", "https://app.staging.terraform.io/app/my-org/my_workspace/runs/run-example"},
		{"http://app.terraform.io/api/v2/", "my-org", "my_workspace", "run-example", ""},
		{"https://user:secret@app.terraform.io/api/v2/", "my-org", "my_workspace", "run-example", ""},
		{"https://app.terraform.io/api/v2/?token=secret", "my-org", "my_workspace", "run-example", ""},
		{"https://app.terraform.io/api/v2/#secret", "my-org", "my_workspace", "run-example", ""},
		{"https://app.terraform.io:8443/api/v2/", "my-org", "my_workspace", "run-example", ""},
		{"https://api.internal.example/api/v2/", "my-org", "my_workspace", "run-example", ""},
		{"https://app.terraform.io.evil.example/api/v2/", "my-org", "my_workspace", "run-example", ""},
		{"https://app.terraform.io/api/v2/", "bad/org", "my_workspace", "run-example", ""},
		{"https://app.terraform.io/api/v2/", "my-org", "my_workspace", "run-bad?token=secret", ""},
	} {
		t.Run(tc.base+"/"+tc.org+"/"+tc.runID, func(t *testing.T) {
			base, err := url.Parse(tc.base)
			require.NoError(t, err)
			require.Equal(t, tc.want, importRunUIURL(*base, tc.org, tc.workspace, tc.runID))
		})
	}
}

func TestVerifyImportPlanReturnsKnownHostRunUIURLAfterAssociationCheck(t *testing.T) {
	backend, _ := importWireBackend(t, "planned_and_finished", "finished")
	backendURL, err := url.Parse(backend.URL)
	require.NoError(t, err)
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	// Keep all API reads inside the mock while exercising the trusted staging
	// HCP address from which a browser Run URL can be constructed.
	c, err := tfe.NewClient(&tfe.Config{Address: "https://app.staging.terraform.io", Token: "test-token", HTTPClient: &http.Client{Transport: importRewriteTransport{backend: backendURL, next: http.DefaultTransport}}})
	require.NoError(t, err)
	importContext, bindings, _ := importTestImportContext(t)
	result, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", ImportContext: importContext, Bindings: bindings})
	require.NoError(t, err)
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, "plan-example", result.PlanID)
	require.Equal(t, "https://app.staging.terraform.io/app/org/target/runs/run-example", result.RunUIURL)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"run_ui_url":"https://app.staging.terraform.io/app/org/target/runs/run-example"`)
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
	require.Equal(t, "aws_sqs_queue", prepared.ImportContext.SelectedResults[0].ListType)
	require.Equal(t, "aws_sqs_queue", prepared.ImportContext.SelectedResults[0].ManagedType)
	require.Equal(t, "6.66.0", prepared.Types[0].SearchProviderVersion)
	require.Equal(t, "6.18.0", prepared.Types[0].LockedProviderVersion)
	require.Equal(t, "unknown", prepared.Types[0].ManagedTypeSupport, "no historical target schema was read")
	require.Contains(t, prepared.NextAction, "proposed managed type is not proved")
	for _, hint := range []string{"relevant module calls and instances", "target locked provider's managed/identity schema", "locked provider_version", "sibling effects"} {
		require.Contains(t, prepared.NextAction, hint)
	}
	require.Contains(t, prepared.NextAction, "Approval is for one speculative plan only")
	require.Contains(t, prepared.NextAction, "ends after Plan review and handoff of the proposed HCL and findings; do not apply or run terraform import")
	for _, hint := range []string{"effective CLI host", "without dumping environment contents", "Do not read, print, request, copy or set credential values", "MCP server configuration and authentication are separate", "terraform plan -no-color -input=false", "plan_only=true"} {
		require.Contains(t, prepared.NextAction, hint)
	}
	for _, guidance := range []string{prepared.NextAction, PrepareImportDescription, VerifyImportPlanDescription} {
		for _, name := range []string{"TF_CLOUD_", "TF_WORKSPACE", "TF_TOKEN_", "TFE_ADDRESS", "TFE_HOSTNAME", "TFE_TOKEN"} {
			require.NotContains(t, guidance, name)
		}
	}
	require.NotContains(t, prepared.NextAction, ".envrc")
	require.Contains(t, PrepareImportDescription, "respecting explicit cloud settings, applicable CLI fallbacks and workspace-selection overrides without dumping the environment")
	require.Contains(t, PrepareImportDescription, "without reading or printing credentials")
	require.Contains(t, PrepareImportDescription, "Approval in this Search-to-import workflow is for one speculative plan only; preparation does not authorize apply")
	require.Contains(t, PrepareImportDescription, "target Terraform version's official import and module documentation")
	require.Contains(t, PrepareImportDescription, "Do not substitute a Search list schema for the target managed schema or guess an identity mapping")
	require.Contains(t, VerifyImportPlanDescription, "CLI and MCP authentication are separate")
	require.Contains(t, VerifyImportPlanDescription, "Do not run terraform apply, terraform import or apply_run as a continuation of this Search-to-import workflow")

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

func TestVerifyImportPlanAttentionOnlyIncludesSelectedExceptions(t *testing.T) {
	base := `{"format_version":"1.2","terraform_version":"1.14.0","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[{"address":"aws_s3_bucket.sample","identity_schema_version":0}]}},"resource_changes":[{"address":"aws_s3_bucket.sample","mode":"managed","type":"aws_s3_bucket","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"private-import-id-sentinel"},"after_identity":{"bucket":"sample"}}}],"output_changes":{},"deferred_changes":[],"resource_drift":[]}`
	for _, tc := range []struct {
		name, planJSON, change, identity string
	}{
		{"identity mismatch", strings.Replace(base, `"after_identity":{"bucket":"sample"}`, `"after_identity":{"bucket":"different"}`, 1), "none", "mismatched"},
		{"identity version differs", strings.Replace(base, `"identity_schema_version":0`, `"identity_schema_version":1`, 1), "none", "unverified"},
		{"identity version absent", strings.Replace(base, `"identity_schema_version":0`, `"identity_schema_version":null`, 1), "none", "unverified"},
		{"import marker missing", strings.Replace(base, `"importing":{"id":"private-import-id-sentinel"},`, "", 1), "import_missing", "unverified"},
		{"selected address missing", strings.Replace(base, `"address":"aws_s3_bucket.sample","mode"`, `"address":"aws_s3_bucket.other","mode"`, 1), "not_in_plan", "unverified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend, _ := importWireBackendWithPlan(t, "planned_and_finished", "finished", "aws_s3_buckets", tc.planJSON)
			c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
			require.NoError(t, err)
			t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
			importContext, bindings, _ := importTestImportContext(t)
			result, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", ImportContext: importContext, Bindings: bindings})
			require.NoError(t, err)
			require.Equal(t, "plan_available", result.Status)
			require.Equal(t, 1, result.Changes[tc.change])
			require.Equal(t, 1, result.ObjectIdentity[tc.identity])
			require.Len(t, result.Attention, 1)
			require.Equal(t, tc.change, result.Attention[0].Change)
			require.Equal(t, tc.identity, result.Attention[0].ObjectIdentity)
			if tc.name == "identity version differs" {
				require.Equal(t, "identity_schema_version_differs", result.Attention[0].IdentityReason)
				require.Contains(t, result.NextAction, "Search 0, Plan 1")
				require.Contains(t, result.NextAction, "If the user asks to investigate")
			}
			if tc.name == "identity version absent" {
				require.Equal(t, "identity_schema_version_not_compared", result.Attention[0].IdentityReason)
				require.Contains(t, result.NextAction, "Plan identity schema version was not reported")
				require.Contains(t, result.NextAction, "Keep object_identity unverified")
			}
		})
	}
}

func TestImportIdentityReviewActionDoesNotConflateMixedFindings(t *testing.T) {
	zero, one := uint64(0), uint64(1)
	result := &VerifyImportResult{
		Changes:        map[string]int{"none": 1, "update": 2},
		ObjectIdentity: map[string]int{"unverified": 2, "mismatched": 1},
		Attention: []ImportPlanAttention{
			{Change: "none", ObjectIdentity: "unverified", IdentityReason: "identity_schema_version_differs", SearchIdentityVersion: 1, PlanIdentityVersion: &zero},
			{Change: "update", ObjectIdentity: "unverified", IdentityReason: "identity_schema_version_differs", SearchIdentityVersion: 2, PlanIdentityVersion: &one},
			{Change: "update", ObjectIdentity: "mismatched", IdentityReason: "after_identity_differs"},
		},
	}
	action := importIdentityReviewAction(result, 3)
	require.Contains(t, action, "2 selected object identities are unverified")
	require.Contains(t, action, "see attention for each version pair")
	require.NotContains(t, action, "(Search 1, Plan 0)")
	require.Contains(t, action, "Identity mismatches: 1")
	require.Contains(t, action, "1 of 3 selected imports have no-op actions")
	require.NotContains(t, action, "all 3 selected imports have no-op actions")
}

func TestVerifyImportPlanVersionDifferenceKeepsPlanAndIdentitySeparateOnMark3labsWire(t *testing.T) {
	provider := queryResultProvider{Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}
	importContext := &ImportContext{QueryRunID: "qry-example", Providers: map[string]queryResultProvider{"aws_sqs_queue": provider}, Target: ImportContextTarget{WorkspaceID: "ws-example", OrganizationName: "org", IdentitySupport: map[string]string{"aws_sqs_queue": "unknown"}}, LockedProviders: map[string]string{provider.Source: "6.18.0"}}
	var bindings []ImportBinding
	var resources, changes []string
	for _, name := range []string{"unmanaged", "modtest_jobs"} {
		identity := map[string]any{"url": "https://example.invalid/" + name}
		marker, err := queryResultMarker("qry-example", provider, "aws_sqs_queue", identity)
		require.NoError(t, err)
		addr := "aws_sqs_queue." + name
		importContext.SelectedResults = append(importContext.SelectedResults, ImportContextSelection{QueryResultMarker: marker, ListType: "aws_sqs_queue", ManagedType: "aws_sqs_queue", Identity: identity, SearchIdentityVersion: 1})
		bindings = append(bindings, ImportBinding{QueryResultMarker: marker, TargetAddress: addr})
		resources = append(resources, fmt.Sprintf(`{"address":%q,"identity_schema_version":0}`, addr))
		changes = append(changes, fmt.Sprintf(`{"address":%q,"mode":"managed","type":"aws_sqs_queue","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"id":"private-id-sentinel"},"after_identity":{"url":%q,"account_id":"private-account-sentinel","region":"private-region-sentinel"}}}`, addr, identity["url"]))
	}
	var err error
	importContext.SelectionDigest, err = importDigest(*importContext)
	require.NoError(t, err)
	planJSON := fmt.Sprintf(`{"format_version":"1.2","terraform_version":"1.14.0","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[%s]}},"resource_changes":[%s],"output_changes":{},"deferred_changes":[],"resource_drift":[]}`, strings.Join(resources, ","), strings.Join(changes, ","))
	backend, _ := importWireBackendWithPlan(t, "planned_and_finished", "finished", "aws_sqs_queue", planJSON)
	t.Setenv("TFE_ADDRESS", backend.URL)
	t.Setenv("TFE_TOKEN", "test-token")
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	ctx := server.NewMCPServer("test", "0").WithContext(context.Background(), importTestSession{})
	wire, err := VerifyImportPlan(silentLogger()).Handler(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{"organization_name": "org", "workspace_name": "target", "run_id": "run-example", "import_context": importContext, "bindings": bindings}}})
	require.NoError(t, err)
	require.False(t, wire.IsError)
	data, err := json.Marshal(wire.StructuredContent)
	require.NoError(t, err)
	var result VerifyImportResult
	require.NoError(t, json.Unmarshal(data, &result))
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, 2, result.Changes["none"])
	require.Equal(t, 2, result.ObjectIdentity["unverified"])
	require.Len(t, result.Attention, 2)
	for _, item := range result.Attention {
		require.Equal(t, "none", item.Change)
		require.Equal(t, "unverified", item.ObjectIdentity)
		require.Equal(t, "identity_schema_version_differs", item.IdentityReason)
		require.Equal(t, 1, item.SearchIdentityVersion)
		require.Equal(t, uint64(0), *item.PlanIdentityVersion)
	}
	require.Equal(t, []string{"identity_requires_review"}, result.Diagnostics)
	require.Contains(t, result.NextAction, "Identity needs review: 2 selected object identities are unverified")
	require.Contains(t, result.NextAction, "Search 1, Plan 0")
	require.Contains(t, result.NextAction, "Plan actions, separately: all 2 selected imports have no-op actions")
	require.Contains(t, result.NextAction, "A satisfactory first Plan requires no second Plan; identity uncertainty alone does not require changing HCL or starting another Run")
	require.Less(t, strings.Index(result.NextAction, "Identity needs review"), strings.Index(result.NextAction, "Plan actions, separately"))
	for _, hint := range []string{"If the user asks to investigate", "locked provider-release documentation", "aliases and module mappings", "corroboration, unknowns and conflicts", "Keep object_identity unverified", "identity uncertainty alone does not require changing configuration or starting another Run"} {
		require.Contains(t, result.NextAction, hint)
	}
	text, ok := mcp.AsTextContent(wire.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"identity_schema_version_differs"`)
	for _, secret := range []string{"private-id-sentinel", "private-account-sentinel", "private-region-sentinel", "https://example.invalid/"} {
		require.NotContains(t, string(data), secret)
		require.NotContains(t, text.Text, secret)
	}
}

func TestVerifyImportPlanEightCleanImportsHaveNoAttention(t *testing.T) {
	importContext, _, _ := importTestImportContext(t)
	importContext.SelectedResults = nil
	provider := importContext.Providers["aws_s3_buckets"]
	var bindings []ImportBinding
	var resources, changes []string
	for i := range 8 {
		name := fmt.Sprintf("sample_%d", i)
		addr := "aws_s3_bucket." + name
		identity := map[string]any{"bucket": name}
		marker, err := queryResultMarker("qry-example", provider, "aws_s3_buckets", identity)
		require.NoError(t, err)
		importContext.SelectedResults = append(importContext.SelectedResults, ImportContextSelection{QueryResultMarker: marker, ListType: "aws_s3_buckets", ManagedType: "aws_s3_bucket", Identity: identity, SearchIdentityVersion: 0})
		bindings = append(bindings, ImportBinding{QueryResultMarker: marker, TargetAddress: addr})
		resources = append(resources, fmt.Sprintf(`{"address":%q,"identity_schema_version":0}`, addr))
		changes = append(changes, fmt.Sprintf(`{"address":%q,"mode":"managed","type":"aws_s3_bucket","provider_name":"registry.terraform.io/hashicorp/aws","change":{"actions":["no-op"],"importing":{"identity":{"bucket":%q}},"after_identity":{"bucket":%q}}}`, addr, name, name))
	}
	var err error
	importContext.SelectionDigest, err = importDigest(*importContext)
	require.NoError(t, err)
	planJSON := fmt.Sprintf(`{"format_version":"1.2","terraform_version":"1.14.0","complete":true,"errored":false,"planned_values":{"root_module":{"resources":[%s]}},"resource_changes":[%s],"output_changes":{},"deferred_changes":[],"resource_drift":[]}`, strings.Join(resources, ","), strings.Join(changes, ","))
	backend, _ := importWireBackendWithPlan(t, "planned_and_finished", "finished", "aws_s3_buckets", planJSON)
	c, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	t.Setenv("MCP_ORGANIZATION_ALLOWLIST", "org")
	result, err := ReadVerifyImportPlan(context.Background(), c, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", ImportContext: importContext, Bindings: bindings, DetailAddresses: []string{bindings[0].TargetAddress}})
	require.NoError(t, err)
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, 8, result.Changes["none"])
	require.Equal(t, 8, result.ObjectIdentity["matched"])
	require.Empty(t, result.Attention)
	require.Len(t, result.Detail, 1, "requested detail remains available for a clean import")
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"attention"`)

	// Selected imports can all be clean while the rest of the Plan needs review.
	withOutputChange := strings.Replace(planJSON, `"output_changes":{}`, `"output_changes":{"other":{"actions":["update"]}}`, 1)
	backendWithOutput, _ := importWireBackendWithPlan(t, "planned_and_finished", "finished", "aws_s3_buckets", withOutputChange)
	cWithOutput, err := tfe.NewClient(&tfe.Config{Address: backendWithOutput.URL, Token: "test-token", HTTPClient: backendWithOutput.Client()})
	require.NoError(t, err)
	result, err = ReadVerifyImportPlan(context.Background(), cWithOutput, VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", ImportContext: importContext, Bindings: bindings})
	require.NoError(t, err)
	require.Empty(t, result.Attention)
	require.Equal(t, 1, result.Plan.OutputChanges)
	require.Contains(t, result.Diagnostics, "plan_actions_require_review")
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
	input := VerifyImportInput{OrganizationName: "org", WorkspaceName: "target", RunID: "run-example", ImportContext: prep.ImportContext,
		Bindings: []ImportBinding{{QueryResultMarker: marker, TargetAddress: "aws_sqs_queue.unmanaged"}}, DetailAddresses: []string{"aws_sqs_queue.unmanaged"}}
	result, err := ReadVerifyImportPlan(context.Background(), c, input)
	require.NoError(t, err)
	require.Equal(t, "plan_available", result.Status)
	require.Equal(t, "1.2", result.Plan.FormatVersion)
	require.Equal(t, "1.17.0-dev.1", result.Plan.TerraformVersion)
	require.Equal(t, 1, result.Changes["update"])
	require.Equal(t, 1, result.ObjectIdentity["matched"], "identity classification must not mask the update")
	require.Len(t, result.Attention, 1)
	require.Equal(t, "update", result.Attention[0].Change)
	require.Equal(t, "matched", result.Attention[0].ObjectIdentity)
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
	require.Equal(t, "aws_s3_buckets", prepared.ImportContext.SelectedResults[0].ListType)
	require.Equal(t, "aws_s3_bucket_logging", prepared.ImportContext.SelectedResults[0].ManagedType)
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
	require.Contains(t, prepare.OutputSchema.Properties, "import_context")
	require.NotContains(t, string(prepare.RawInputSchema), `"maxItems":100`)
	require.NotContains(t, prepare.Description, "1–100")
	verify := VerifyImportPlan(silentLogger()).Tool
	var verifyInputSchema jsonschema.Schema
	require.NoError(t, json.Unmarshal(verify.RawInputSchema, &verifyInputSchema))
	require.Contains(t, verifyInputSchema.Required, "run_id")
	require.NotContains(t, verifyInputSchema.Required, "import_context")
	require.Contains(t, verifyInputSchema.Properties, "import_context")
	require.NotContains(t, string(verify.RawInputSchema), "maximum 100")
	require.NotContains(t, string(verify.RawInputSchema), "maximum 20")
	require.Contains(t, verify.OutputSchema.Properties, "object_identity")
}
