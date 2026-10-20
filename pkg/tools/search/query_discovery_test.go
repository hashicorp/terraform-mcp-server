// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
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
	} {
		t.Run(string(expected), func(t *testing.T) {
			actual, ok := nonselectableQueryEvidenceCode(fmt.Errorf("wrapped: %w", searchDiagnosticError(expected)))
			require.True(t, ok)
			require.Equal(t, expected, actual)
		})
	}

	for _, err := range []error{errors.New("query_not_selectable"), searchDiagnosticError(queryProviderSourceUnresolvedCode)} {
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
	return &queryDiscoveryProvenance{QueryRunID: "qry-test", Organization: "org", WorkspaceName: "ws", WorkspaceID: "ws-test"}, evidence
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

func TestSummarizeQueryDiscoveryReturnsGeneratedConfigurationWithItsMarker(t *testing.T) {
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
	require.Equal(t, evidence.Results[1].Configuration, row.Configuration)
	require.Contains(t, summary.NextAction, "untrusted review-required draft")

	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.Contains(t, text, `"configuration":"resource \"aws_instance\" \"large\"`)
	require.NotContains(t, text, "must-not-be-returned")
	require.NotContains(t, text, "t3.small")
}

func TestSummarizeQueryDiscoveryOmitsConfigurationWhenNotGenerated(t *testing.T) {
	prov, evidence := testDiscovery(t, "missing")
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Empty(t, summary.Lists[0].Results[0].Configuration)

	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.NotContains(t, text, `"configuration"`)
}

func TestSummarizeQueryDiscoveryProjectsProviderMetadataAndExplainsMissingAttributes(t *testing.T) {
	prov, evidence := testDiscovery(t, "captured", "missing", "untagged")
	evidence.Results[0].AttributesCaptured = true
	evidence.Results[0].ObservedAttributes = queryDiscoveryObservedAttributes(evidence.Results[0].Provider, map[string]any{
		"tags":     map[string]any{"team": "search", "enabled": true, "count": json.Number("2"), "nested": map[string]any{"private": "value"}},
		"tags_all": map[string]any{"team": "search", "inherited": "yes"},
		"labels":   map[string]any{"must": "not be projected for aws"},
	})
	evidence.Results[2].AttributesCaptured = true
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Equal(t, 1, summary.RowsWithoutAttributes)
	require.Equal(t, map[string]any{
		"tags":     map[string]any{"team": "search", "enabled": true, "count": json.Number("2")},
		"tags_all": map[string]any{"team": "search", "inherited": "yes"},
	}, summary.Lists[0].Results[0].ObservedAttributes)
	require.True(t, summary.Lists[0].Results[0].AttributesCaptured)
	require.False(t, summary.Lists[0].Results[1].AttributesCaptured)
	require.Empty(t, summary.Lists[0].Results[1].ObservedAttributes)
	require.True(t, summary.Lists[0].Results[2].AttributesCaptured)
	require.Empty(t, summary.Lists[0].Results[2].ObservedAttributes)
	require.Contains(t, summary.NextAction, "attributes_captured false")
	require.Contains(t, summary.NextAction, "ask before creating another QueryRun")
	require.Contains(t, summary.NextAction, "generate_config_out true")
	require.Contains(t, summary.NextAction, "preserving supported filters")
	require.Contains(t, summary.NextAction, "omitting unsupported conditions")
	require.Contains(t, summary.NextAction, "does not guarantee capture")
	require.Contains(t, summary.NextAction, "new QueryRun has new markers")
	require.Contains(t, summary.NextAction, "rather than arbitrary resource attributes")
	text, err := FormatQueryDiscoverySummary(summary)
	require.NoError(t, err)
	require.Contains(t, text, `"attributes_captured":false`)
	require.NotContains(t, text, "private")
}

func TestSummarizeQueryDiscoveryDoesNotRecommendRetryWhenAllRowsCaptured(t *testing.T) {
	prov, evidence := testDiscovery(t, "captured", "captured-without-tags")
	evidence.Results[0].AttributesCaptured = true
	evidence.Results[0].ObservedAttributes = map[string]any{"tags": map[string]any{"environment": "production"}}
	evidence.Results[1].AttributesCaptured = true
	summary, err := summarizeQueryDiscovery(prov, evidence, QueryDiscoveryFilter{})
	require.NoError(t, err)
	require.Zero(t, summary.RowsWithoutAttributes)
	require.NotContains(t, summary.NextAction, "repeat the same schema-backed query")
	require.Contains(t, summary.NextAction, "missing projected metadata does not prove")
}

func TestQueryDiscoveryObservedAttributesUsesSmallProviderQualifiedSubset(t *testing.T) {
	resource := map[string]any{
		"tags":                  map[string]any{"team": "search", "nested": map[string]any{"secret": "omit"}},
		"tags_all":              map[string]any{"team": "search", "default": "yes"},
		"labels":                map[string]any{"environment": "prod"},
		"terraform_labels":      map[string]any{"managed": true},
		"effective_labels":      map[string]any{"external": "present"},
		"resource_manager_tags": map[string]any{"tagKeys/1": "tagValues/2"},
		"password":              "must-not-be-projected",
	}

	tests := []struct {
		name, source string
		want         map[string]any
	}{
		{"aws", "registry.terraform.io/hashicorp/aws", map[string]any{"tags": map[string]any{"team": "search"}, "tags_all": map[string]any{"team": "search", "default": "yes"}}},
		{"azurerm", "registry.terraform.io/hashicorp/azurerm", map[string]any{"tags": map[string]any{"team": "search"}}},
		{"google", "registry.terraform.io/hashicorp/google", map[string]any{"labels": map[string]any{"environment": "prod"}, "terraform_labels": map[string]any{"managed": true}, "effective_labels": map[string]any{"external": "present"}, "resource_manager_tags": map[string]any{"tagKeys/1": "tagValues/2"}}},
		{"google beta", "registry.terraform.io/hashicorp/google-beta", map[string]any{"labels": map[string]any{"environment": "prod"}, "terraform_labels": map[string]any{"managed": true}, "effective_labels": map[string]any{"external": "present"}, "resource_manager_tags": map[string]any{"tagKeys/1": "tagValues/2"}}},
		{"unsupported provider", "registry.terraform.io/example/example", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, queryDiscoveryObservedAttributes(queryResultProvider{Source: test.source}, resource))
		})
	}
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
			_, _ = fmt.Fprint(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished"},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`)
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
	require.Equal(t, "registry.terraform.io/hashicorp/aws", prov.Providers["aws_s3_buckets"].Source)
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
			_, _ = fmt.Fprintf(w, `{"data":{"type":"queries","id":"qry-test","attributes":{"status":"finished","log-read-url":%q},"relationships":{"workspace":{"data":{"type":"workspaces","id":"ws-test"}},"no-code-query":{"data":{"type":"no-code-queries","id":"ncqry-test"}}}}}`, backend.URL+"/logs")
		case "/api/v2/workspaces/ws-test":
			_, _ = fmt.Fprint(w, `{"data":{"type":"workspaces","id":"ws-test","attributes":{"name":"ws"},"relationships":{"organization":{"data":{"type":"organizations","id":"org"}}}}}`)
		case "/api/v2/search/no-code-query/ncqry-test":
			_, _ = fmt.Fprint(w, `{"data":{"id":"ncqry-test","attributes":{"no-code-query-providers":[{"namespace":"hashicorp","name":"aws","version":"6.66.0","no-code-query-resources":[{"body":{"resource_type":"aws_s3_buckets"}}]}]},"relationships":{"workspace":{"data":{"id":"ws-test"}}}}}`)
		case "/logs":
			found := strings.Replace(string(queryEvidenceFixture("0")), `"identity_version":0`, `"identity_version":0,"resource_object":{"tags":{"team":"search"}},"config":"resource \"aws_s3_bucket\" \"example\" {}","import_config":"import { id = \"must-not-be-returned\" }"`, 1)
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
	require.Equal(t, map[string]any{"tags": map[string]any{"team": "search"}}, summary.Lists[0].Results[0].ObservedAttributes)
	require.Equal(t, `resource "aws_s3_bucket" "example" {}`, summary.Lists[0].Results[0].Configuration)
	require.True(t, summary.Lists[0].Results[0].AttributesCaptured)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, `"query_result_marker"`)
	require.Contains(t, text.Text, `"observed_attributes":{"tags":{"team":"search"}}`)
	require.Contains(t, text.Text, `"configuration":"resource \"aws_s3_bucket\" \"example\" {}"`)
	require.NotContains(t, text.Text, "must-not-be-returned")
	require.NotContains(t, text.Text, "/logs")
}
