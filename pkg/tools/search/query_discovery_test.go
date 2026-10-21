// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	terraformclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestNonselectableQueryEvidenceCode(t *testing.T) {
	for _, expected := range []searchDiagnosticCode{
		queryNotSelectableCode,
		queryIdentityVersionInvalidCode,
		queryEvidenceIncompleteCode,
		queryDuplicateOrInvalidIdentityCode,
		queryIdentityInvalidCode,
		queryReportedErrorCode,
		queryGeneratedBlockConflictCode,
		queryGeneratedConfigurationInvalidCode,
		queryGeneratedConfigurationUnexpectedCode,
	} {
		t.Run(string(expected), func(t *testing.T) {
			actual, ok := nonselectableQueryEvidenceCode(fmt.Errorf("wrapped: %w", searchDiagnosticError(expected)))
			require.True(t, ok)
			require.Equal(t, expected, actual)
		})
	}

	for _, err := range []error{
		errors.New("query_not_selectable"),
		searchDiagnosticError(queryProviderSourceUnresolvedCode),
		searchDiagnosticError(queryLogSizeLimitCode),
	} {
		code, ok := nonselectableQueryEvidenceCode(err)
		require.False(t, ok)
		require.Empty(t, code)
	}
}

func TestNonselectableQueryNextActionsAreDiagnosticSpecific(t *testing.T) {
	tests := []struct {
		code      searchDiagnosticCode
		required  string
		forbidden string
	}{
		{queryNotSelectableCode, "failed or was canceled", "select one"},
		{queryIdentityVersionInvalidCode, "identity-version evidence", "partial"},
		{queryEvidenceIncompleteCode, "Do not present a prefix", "import from it"},
		{queryDuplicateOrInvalidIdentityCode, "Duplicate or conflicting", "continue selection"},
		{queryIdentityInvalidCode, "identity evidence is invalid", "select one"},
		{queryReportedErrorCode, "recorded an error", "import from it"},
		{queryGeneratedBlockConflictCode, "configuration evidence conflicts", "select one"},
		{queryGeneratedConfigurationInvalidCode, "structurally invalid", "select one"},
		{queryGeneratedConfigurationUnexpectedCode, "generation was disabled", "select one"},
	}
	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			action := nonselectableQueryNextAction(test.code)
			require.Contains(t, action, test.required)
			require.NotContains(t, action, test.forbidden)
			require.Contains(t, action, "another query")
		})
	}
}

func testDiscovery(t *testing.T, ids ...string) (*queryDiscoveryProvenance, *queryEvidence) {
	t.Helper()
	var lines []string
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf(`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":%q,"identity":{"bucket":%q},"identity_version":0}}`, id, id))
	}
	lines = append(lines, fmt.Sprintf(`{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":%d}}`, len(ids)))
	evidence, err := parseQueryEvidence("qry-test", []byte(strings.Join(lines, "\n")), testQueryProviders())
	require.NoError(t, err)
	return &queryDiscoveryProvenance{QueryRunID: "qry-test", Organization: "org", WorkspaceName: "ws", WorkspaceID: "ws-test", GenerateConfigOut: true}, evidence
}

func TestSummarizeQueryDiscoveryReturnsEveryMatchingResult(t *testing.T) {
	prov, evidence := testDiscovery(t, "first", "second", "third")
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.True(t, summary.Selectable)
	require.Equal(t, "org", summary.SourceOrganizationName)
	require.Equal(t, 3, summary.TotalMatching)
	require.Len(t, summary.Lists[0].Results, 3)
	require.Equal(t, evidence.Results[0].Marker, summary.Lists[0].Results[0].Marker)
	require.Contains(t, summary.NextAction, "report these results and stop")
	require.Contains(t, summary.NextAction, "do not request a target workspace or local path")
	require.Contains(t, summary.NextAction, "query_result_marker")

	filtered, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{NameContains: "second"})
	require.NoError(t, err)
	require.Equal(t, 1, filtered.TotalMatching)
	require.Equal(t, evidence.Results[1].Marker, filtered.Lists[0].Results[0].Marker)
	require.Contains(t, filtered.NextAction, "supplied filters affect presentation only")
}

func TestSummarizeQueryDiscoveryReturnsGeneratedAttributesWithItsMarker(t *testing.T) {
	prov, evidence := testDiscovery(t, "small", "large")
	evidence.Results[0].Configuration = `resource "aws_instance" "small" { instance_type = "t3.small" }`
	evidence.Results[0].ImportConfig = `import { id = "must-not-be-returned" }`
	evidence.Results[1].Configuration = `resource "aws_instance" "large" { instance_type = "m7i.large" }`

	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{NameContains: "large"})
	require.NoError(t, err)
	require.Equal(t, 1, summary.TotalMatching)
	require.Len(t, summary.Lists, 1)
	require.Len(t, summary.Lists[0].Results, 1)
	row := summary.Lists[0].Results[0]
	require.Equal(t, evidence.Results[1].Marker, row.Marker)
	require.True(t, row.ConfigurationGenerated)
	require.Equal(t, map[string]any{"instance_type": "m7i.large"}, row.ConfigurationAttributes)
	require.Contains(t, summary.NextAction, "potentially sensitive")
	require.Contains(t, summary.NextAction, "returned only by prepare_import")

	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.NotContains(t, text, `"configuration":`)
	require.NotContains(t, text, "must-not-be-returned")
	require.NotContains(t, text, "t3.small")
}

func TestSummarizeQueryDiscoveryDoesNotParseFilteredConfiguration(t *testing.T) {
	prov, evidence := testDiscovery(t, "filtered", "selected")
	evidence.Results[0].Configuration = `resource "private_resource" "filtered" {`
	evidence.Results[1].Configuration = `resource "aws_instance" "selected" { instance_type = "m7i.large" }`

	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{NameContains: "selected"})
	require.NoError(t, err)
	require.Equal(t, 1, summary.TotalMatching)
	require.Equal(t, map[string]any{"instance_type": "m7i.large"}, summary.Lists[0].Results[0].ConfigurationAttributes)
}

func TestSummarizeQueryDiscoveryReportsGeneratedConfigurationWithoutProjectedAttributes(t *testing.T) {
	prov, evidence := testDiscovery(t, "unsupported")
	evidence.Results[0].Configuration = `resource "example_resource" "unsupported" {
  policy = jsonencode({ key = "value" })
  nested_block { value = "ignored" }
}`

	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	row := summary.Lists[0].Results[0]
	require.True(t, row.ConfigurationGenerated)
	require.Nil(t, row.ConfigurationAttributes)
	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.Contains(t, text, `"configuration_generated":true`)
	require.NotContains(t, text, `"configuration_attributes"`)
	require.NotContains(t, text, `"configuration":`)
}

func TestSummarizeQueryDiscoveryReportsWhenConfigurationWasNotGenerated(t *testing.T) {
	prov, evidence := testDiscovery(t, "missing")
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.False(t, summary.Lists[0].Results[0].ConfigurationGenerated)

	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.NotContains(t, text, `"configuration":`)
}

func TestSummarizeQueryDiscoveryExplainsGenerationAvailability(t *testing.T) {
	prov, evidence := testDiscovery(t, "missing")
	prov.GenerateConfigOut = false
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.False(t, summary.GenerateConfigOut)
	require.Contains(t, summary.NextAction, "Generated configuration was not requested")
	require.Contains(t, summary.NextAction, "ask before creating another QueryRun")
	require.Contains(t, summary.NextAction, "new QueryRun has new markers")

	prov.GenerateConfigOut = true
	summary, err = summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Contains(t, summary.NextAction, "did not yield configuration")
	require.Contains(t, summary.NextAction, "do not promise")
}

func TestSummarizeQueryDiscoveryDoesNotApplyOutputSizeCaps(t *testing.T) {
	prov, evidence := testDiscovery(t, "large")
	evidence.Results[0].Identity["large"] = strings.Repeat("x", 100*1024)
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Len(t, summary.Lists[0].Results, 1)
	require.Len(t, summary.Lists[0].Results[0].Identity["large"], 100*1024)

	_, err = FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
}

func TestQueryProvenanceChecksBackendAssociations(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","generate-config-out":true},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`)
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"HashiCorp","name":"AWS","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"org"})
	prov, err := readQueryDiscoveryProvenance(ctx, client, "qry-test")
	require.NoError(t, err)
	require.Equal(t, "org", prov.Organization)
	require.True(t, prov.GenerateConfigOut)
	require.Equal(t, "registry.terraform.io/hashicorp/aws", prov.Providers["aws_s3_buckets"].Source)
}

func TestValidateGeneratedConfigurationEvidenceRejectsDisabledGenerationConflict(t *testing.T) {
	for name, result := range map[string]queryResultEvidence{
		"resource only": {Configuration: `resource "example" "result" {}`},
		"import only":   {ImportConfig: `import { to = example.result id = "example" }`},
		"both":          {Configuration: `resource "example" "result" {}`, ImportConfig: `import { to = example.result id = "example" }`},
	} {
		t.Run(name, func(t *testing.T) {
			evidence := &queryEvidence{Results: []queryResultEvidence{result}}
			require.EqualError(t, validateGeneratedConfigurationEvidence(false, evidence), "query_generated_configuration_unexpected")
			require.NoError(t, validateGeneratedConfigurationEvidence(true, evidence))
		})
	}
	require.NoError(t, validateGeneratedConfigurationEvidence(false, &queryEvidence{}))
}

func TestNonselectableGeneratedConfigurationSummaryDoesNotDiscloseRows(t *testing.T) {
	prov, evidence := testDiscovery(t, "private-result")
	evidence.Results[0].Configuration = `resource "private_resource" "result" {`

	_, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.EqualError(t, err, "query_generated_configuration_invalid")

	summary := nonselectableQuerySummary(prov, queryGeneratedConfigurationInvalidCode)
	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.False(t, summary.Selectable)
	require.Empty(t, summary.Lists)
	require.NotContains(t, text, "private-result")
	require.NotContains(t, text, "private_resource")
}

func TestQueryProvenanceRejectsMissingGenerateConfigOut(t *testing.T) {
	var storedQueryReads int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`)
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		default:
			storedQueryReads++
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)

	_, err = readQueryDiscoveryProvenance(context.Background(), client, "qry-test")
	require.EqualError(t, err, "query_provenance_unavailable")
	require.Zero(t, storedQueryReads)
}

func TestCanonicalQueryProviderSourceRejectsInvalidCatalogPartsWithoutReflection(t *testing.T) {
	for _, test := range []struct {
		name, namespace, providerType string
	}{
		{name: "empty namespace", namespace: "", providerType: "aws"},
		{name: "namespace with slash", namespace: "unsafe/namespace", providerType: "aws"},
		{name: "type with dot", namespace: "hashicorp", providerType: "unsafe.type"},
		{name: "type with control character", namespace: "hashicorp", providerType: "unsafe\ntype"},
		{name: "reserved type prefix", namespace: "hashicorp", providerType: "terraform-example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, err := canonicalQueryProviderSource(test.namespace, test.providerType)
			require.Empty(t, source)
			require.EqualError(t, err, "query_provider_source_unresolved")
			if test.namespace != "" {
				require.NotContains(t, err.Error(), test.namespace)
			}
			if test.providerType != "" {
				require.NotContains(t, err.Error(), test.providerType)
			}
		})
	}
}

func TestQuerySummaryRejectsDisallowedOrganizationBeforeEvidenceDisclosure(t *testing.T) {
	for _, status := range []tfe.QueryRunStatus{tfe.QueryRunFinished, tfe.QueryRunErrored, tfe.QueryRunCanceled} {
		t.Run(string(status), func(t *testing.T) {
			var queryReads, workspaceReads, evidenceReads int
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.api+json")
				switch r.URL.Path {
				case "/api/v2/ping":
					w.WriteHeader(http.StatusOK)
				case "/api/v2/queries/qry-test":
					queryReads++
					_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":%q,"log-read-url":"%s/logs"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}}}}}`, status, "https://example.invalid")
				case "/api/v2/workspaces/ws-test":
					workspaceReads++
					_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"private-workspace"},"relationships":{"organization":{"data":{"type":"organizations","id":"blocked-org"}}}}}`)
				default:
					evidenceReads++
					http.NotFound(w, r)
				}
			}))
			defer backend.Close()
			client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
			require.NoError(t, err)
			ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"allowed-org"})

			summary, err := ReadQueryDiscoverySummary(ctx, client, "qry-test", QueryDiscoveryFilter{})

			require.ErrorIs(t, err, terraformclient.ErrOrganizationNotAllowed)
			require.NotContains(t, err.Error(), "blocked-org")
			require.NotContains(t, err.Error(), "private-workspace")
			require.Nil(t, summary)
			require.Equal(t, 1, queryReads)
			require.Equal(t, 1, workspaceReads)
			require.Zero(t, evidenceReads)
		})
	}
}

func TestQuerySummaryFailsClosedWhenAllowlistedOwnershipCannotBeResolved(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}}}}}`)
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	ctx := terraformclient.WithOrganizationAllowlist(context.Background(), []string{"allowed-org"})

	summary, err := ReadQueryDiscoverySummary(ctx, client, "qry-test", QueryDiscoveryFilter{})

	require.EqualError(t, err, string(queryOwnershipUnverifiedCode))
	require.Nil(t, summary)
}

func TestQueryProvenanceRejectsWhitespaceOnlyRawRelationshipID(t *testing.T) {
	var noCodeQueryReads int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":" "}}}}}`)
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		default:
			noCodeQueryReads++
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)

	_, err = readQueryDiscoveryProvenance(context.Background(), client, "qry-test")
	require.EqualError(t, err, "query_provenance_unavailable")
	require.Zero(t, noCodeQueryReads)
}

func TestLegacyQuerySummaryProjectsBoundedSelectableResults(t *testing.T) {
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch r.URL.Path {
		case "/api/v2/ping":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/queries/qry-test":
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","generate-config-out":true,"log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs")
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		case "/logs":
			found := strings.Replace(string(queryEvidenceFixture("0")), `"identity_version":0`, `"identity_version":0,"resource_object":{"tags":{"team":"search"}},"config":"resource \"aws_s3_bucket\" \"example\" { bucket = \"example\" }","import_config":"import { id = \"must-not-be-returned\" }"`, 1)
			data := append([]byte("\x02Terraform query started\n"+`{"type":"provider_installation_start","provider":"aws"}`+"\n"), []byte(found)...)
			data = append(data, 3)
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset < len(data) {
				_, _ = w.Write(data[offset:min(len(data), offset+limit)])
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	client, err := tfe.NewClient(&tfe.Config{Address: backend.URL, Token: "test-token", HTTPClient: backend.Client()})
	require.NoError(t, err)
	result, err := getQuerySummaryWithClient(context.Background(), client, "  qry-test\t", QueryDiscoveryFilter{}, silentLogger())
	require.NoError(t, err)
	require.False(t, result.IsError, "tool content: %#v", result.Content)
	summary, ok := result.StructuredContent.(*QueryDiscoverySummary)
	require.True(t, ok)
	require.True(t, summary.Selectable)
	require.Equal(t, "qry-test", summary.QueryRunID)
	require.Equal(t, "org", summary.SourceOrganizationName)
	require.Len(t, summary.Lists, 1)
	require.Regexp(t, `^qrm1-`, summary.Lists[0].Results[0].Marker)
	require.True(t, summary.GenerateConfigOut)
	require.True(t, summary.Lists[0].Results[0].ConfigurationGenerated)
	require.Equal(t, map[string]any{"bucket": "example"}, summary.Lists[0].Results[0].ConfigurationAttributes)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"query_result_marker"`)
	require.Contains(t, text.Text, `"configuration_generated":true`)
	require.Contains(t, text.Text, `"configuration_attributes":{"bucket":"example"}`)
	require.NotContains(t, text.Text, "resource_object")
	require.NotContains(t, text.Text, `"configuration":`)
	require.NotContains(t, text.Text, "must-not-be-returned")
	require.NotContains(t, text.Text, "/logs")
}
