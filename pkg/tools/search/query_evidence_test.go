// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func testQueryProviders() map[string]queryResultProvider {
	return map[string]queryResultProvider{"aws_s3_buckets": {Source: "registry.terraform.io/hashicorp/aws", Version: "6.66.0"}}
}

func queryEvidenceFixture(version string) []byte {
	return []byte(fmt.Sprintf("%s\n%s\n",
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"example","identity":{"bucket":"example"},"identity_version":`+version+`}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","total":1}}`))
}

func TestQueryResultMarkerIsScopedDeterministicAndNotAV0ID(t *testing.T) {
	evidence, err := parseQueryEvidence("qry-one", queryEvidenceFixture("0"), testQueryProviders())
	require.NoError(t, err)
	require.Len(t, evidence.Results, 1)
	require.Equal(t, 0, evidence.Results[0].IdentityVersion)
	require.Regexp(t, `^qrm1-[a-f0-9]{64}$`, evidence.Results[0].Marker)
	again, err := parseQueryEvidence("qry-one", queryEvidenceFixture("0"), testQueryProviders())
	require.NoError(t, err)
	require.Equal(t, evidence.Results[0].Marker, again.Results[0].Marker)
	other, err := parseQueryEvidence("qry-two", queryEvidenceFixture("0"), testQueryProviders())
	require.NoError(t, err)
	require.NotEqual(t, evidence.Results[0].Marker, other.Results[0].Marker)
}

func TestQueryEvidenceIgnoresHarmlessTextAndJSONProgressWithoutSkippingResults(t *testing.T) {
	log := strings.Join([]string{
		"Starting Terraform query...",
		"https://archivist.example/log?X-Amz-Signature=secret-sentinel",
		`{"type":"version","terraform":"1.14.0"}`,
		`{"type":"provider_installation_start","provider":"registry.terraform.io/hashicorp/aws"}`,
		`{"type":"list_start","list_start":{"address":"list.aws_s3_buckets.all"}}`,
		strings.TrimSpace(string(queryEvidenceFixture("0"))),
		`{"type":"policy_query_summary","policy_query_summary":{"address":"list.aws_s3_buckets.all"}}`,
		"Query finished.",
	}, "\n")
	evidence, err := parseQueryEvidence("qry-one", []byte(log), testQueryProviders())
	require.NoError(t, err)
	require.Len(t, evidence.Results, 1)
	require.Equal(t, 1, evidence.ByType["aws_s3_buckets"])
	require.Regexp(t, `^qrm1-[a-f0-9]{64}$`, evidence.Results[0].Marker)
	require.NotContains(t, fmt.Sprint(evidence.Results), "secret-sentinel")
}

func TestQueryEvidenceRejectsEntireResultOnInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name, data, code string
	}{
		{"missing version", string(queryEvidenceFixture("null")), "query_identity_version_invalid"},
		{"negative version", string(queryEvidenceFixture("-1")), "query_identity_version_invalid"},
		{"fractional version", string(queryEvidenceFixture("1.0")), "query_identity_version_invalid"},
		{"string version", string(queryEvidenceFixture(`"1"`)), "query_identity_version_invalid"},
		{"duplicate identity", string(queryEvidenceFixture("0")) + strings.Split(string(queryEvidenceFixture("0")), "\n")[0] + "\n", "query_duplicate_or_invalid_identity"},
		{"duplicate key", strings.Replace(string(queryEvidenceFixture("0")), `"identity_version":0`, `"identity_version":0,"identity_version":0`, 1), "query_evidence_invalid"},
		{"wrong total", strings.Replace(string(queryEvidenceFixture("0")), `"total":1`, `"total":2`, 1), "query_evidence_incomplete"},
		{"no completion", strings.Split(string(queryEvidenceFixture("0")), "\n")[0], "query_evidence_incomplete"},
		{"unknown found record", strings.Replace(string(queryEvidenceFixture("0")), `"type":"list_resource_found"`, `"type":"new_resource_found"`, 1), "query_evidence_invalid"},
		{"damaged found text", "list_resource_found: not JSON\n" + string(queryEvidenceFixture("0")), "query_evidence_invalid"},
		{"damaged completion text", string(queryEvidenceFixture("0")) + "list_complete: not JSON\n", "query_evidence_invalid"},
		{"damaged diagnostic text", string(queryEvidenceFixture("0")) + "diagnostic: not JSON\n", "query_evidence_invalid"},
		{"array record", string(queryEvidenceFixture("0")) + `[{"type":"list_resource_found"}]`, "query_evidence_invalid"},
		{"unknown result payload", string(queryEvidenceFixture("0")) + `{"type":"log","list_resource_found":{"identity":{"bucket":"hidden"}}}`, "query_evidence_invalid"},
		{"malformed JSON progress", string(queryEvidenceFixture("0")) + `{"type":"provider_installation_start",`, "query_evidence_invalid"},
		{"error diagnostic", string(queryEvidenceFixture("0")) + `{"type":"diagnostic","diagnostic":{"severity":"error","detail":"https://signed.example/token"}}`, "query_reported_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence, err := parseQueryEvidence("qry-one", []byte(tc.data), testQueryProviders())
			require.Nil(t, evidence)
			require.EqualError(t, err, tc.code)
			require.NotContains(t, err.Error(), "signed.example")
		})
	}
}

func TestQueryEvidenceRequiresCompletionForEveryStartedList(t *testing.T) {
	start := `{"type":"list_start","list_start":{"address":"list.aws_s3_buckets.empty"}}`
	complete := `{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.empty","total":0}}`
	for _, tc := range []struct {
		name string
		data string
		code string
	}{
		{"started only", start, "query_evidence_incomplete"},
		{"completed list then unfinished list", string(queryEvidenceFixture("0")) + start, "query_evidence_incomplete"},
		{"zero result list completed", start + "\n" + complete, ""},
		{"all started lists completed", start + "\n" + string(queryEvidenceFixture("0")) + complete, ""},
		{"missing start payload", `{"type":"list_start"}`, "query_evidence_invalid"},
		{"missing start address", `{"type":"list_start","list_start":{}}`, "query_evidence_invalid"},
		{"blank start address", `{"type":"list_start","list_start":{"address":" "}}`, "query_evidence_invalid"},
		{"damaged start text", string(queryEvidenceFixture("0")) + "list_start: not JSON", "query_evidence_invalid"},
		{"start disguised as progress", string(queryEvidenceFixture("0")) + `{"type":"log","list_start":{"address":"hidden"}}`, "query_evidence_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence, err := parseQueryEvidence("qry-one", []byte(tc.data), testQueryProviders())
			if tc.code != "" {
				require.EqualError(t, err, tc.code)
				require.Nil(t, evidence)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, evidence)
		})
	}
}

func TestUnfinishedStartedListPreventsSummaryAndPreparationFinalization(t *testing.T) {
	data := string(queryEvidenceFixture("0")) + `{"type":"list_start","list_start":{"address":"list.aws_s3_buckets.unfinished"}}`
	marker, err := queryResultMarker("qry-one", testQueryProviders()["aws_s3_buckets"], "aws_s3_buckets", map[string]any{"bucket": "example"})
	require.NoError(t, err)
	for name, collector := range map[string]queryEvidenceCollector{
		"summary": newQueryDiscoverySummaryCollector(&queryDiscoveryProvenance{QueryRunID: "qry-one"}, QueryDiscoveryFilter{}),
		"prepare": newSelectedQueryEvidenceCollector([]ImportSelection{{QueryResultMarker: marker, ManagedType: "aws_s3_bucket"}}),
	} {
		t.Run(name, func(t *testing.T) {
			err := parseQueryEvidenceReaderWithCollector("qry-one", strings.NewReader(data), testQueryProviders(), collector)
			require.EqualError(t, err, "query_evidence_incomplete")
		})
	}
}

func TestQueryEvidenceAcceptsLargeValuesWithinDepthLimit(t *testing.T) {
	raw := []byte(strings.Repeat("[", 40) + `{"value":"` + strings.Repeat("x", 128*1024) + `"}` + strings.Repeat("]", 40))
	var value any
	require.NoError(t, decodeQueryEvidence(raw, &value))
}

func TestQueryEvidenceReaderHandlesFragmentedRecordsAndFinalLine(t *testing.T) {
	data := bytes.TrimSuffix(queryEvidenceFixture("0"), []byte{'\n'})
	evidence, err := parseQueryEvidenceReader("qry-one", iotest.OneByteReader(bytes.NewReader(data)), testQueryProviders())
	require.NoError(t, err)
	require.Len(t, evidence.Results, 1)
	require.Equal(t, "example", evidence.Results[0].DisplayName)
}

func TestQueryEvidenceReaderAcceptsRecordLargerThanReadBuffer(t *testing.T) {
	displayName := strings.Repeat("x", 128*1024)
	data := bytes.Replace(queryEvidenceFixture("0"), []byte(`"display_name":"example"`), []byte(`"display_name":"`+displayName+`"`), 1)
	evidence, err := parseQueryEvidenceReader("qry-one", bytes.NewReader(data), testQueryProviders())
	require.NoError(t, err)
	require.Equal(t, displayName, evidence.Results[0].DisplayName)
}

func TestQueryEvidenceReaderFailsClosedOnLateReadError(t *testing.T) {
	sentinel := errors.New("signed backend detail")
	found := bytes.Split(queryEvidenceFixture("0"), []byte{'\n'})[0]
	reader := io.MultiReader(bytes.NewReader(append(found, '\n')), iotest.ErrReader(sentinel))
	evidence, err := parseQueryEvidenceReader("qry-one", reader, testQueryProviders())
	require.Nil(t, evidence)
	require.Equal(t, "query_log_unavailable", err.Error())
	require.ErrorIs(t, err, sentinel)
	require.NotContains(t, err.Error(), "signed backend detail")
}

func TestQueryEvidenceReaderPreservesCallerCancellation(t *testing.T) {
	evidence, err := parseQueryEvidenceReader("qry-one", iotest.ErrReader(context.Canceled), testQueryProviders())
	require.Nil(t, evidence)
	require.Equal(t, "query_log_unavailable", err.Error())
	require.ErrorIs(t, err, context.Canceled)
}

func TestQueryLogLimitReaderAcceptsExactLimitAndRejectsOneByteOver(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reader  io.Reader
		limit   int64
		want    string
		wantErr string
	}{
		{name: "below", reader: strings.NewReader("ab"), limit: 3, want: "ab"},
		{name: "exact", reader: strings.NewReader("abc"), limit: 3, want: "abc"},
		{name: "exact fragmented", reader: iotest.OneByteReader(strings.NewReader("abc")), limit: 3, want: "abc"},
		{name: "one over", reader: strings.NewReader("abcd"), limit: 3, want: "abc", wantErr: "query_log_size_limit"},
		{name: "one over fragmented", reader: iotest.OneByteReader(strings.NewReader("abcd")), limit: 3, want: "abc", wantErr: "query_log_size_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := io.ReadAll(newQueryLogLimitReader(tc.reader, tc.limit))
			require.Equal(t, tc.want, string(actual))
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func TestPurposeSpecificCollectorsRetainOnlyNeededResults(t *testing.T) {
	data := []byte(strings.Join([]string{
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"filtered","identity":{"bucket":"filtered"},"identity_version":0,"configuration":"private-filtered-hcl"}}`,
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"selected","identity":{"bucket":"selected"},"identity_version":0,"configuration":"resource \"aws_s3_bucket\" \"selected\" { bucket = \"private-selected-hcl\" }"}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","total":2}}`,
	}, "\n"))

	selectedMarker, err := queryResultMarker("qry-one", testQueryProviders()["aws_s3_buckets"], "aws_s3_buckets", map[string]any{"bucket": "selected"})
	require.NoError(t, err)
	prepareCollector := newSelectedQueryEvidenceCollector([]ImportSelection{{QueryResultMarker: selectedMarker, ManagedType: "aws_s3_bucket"}})
	require.NoError(t, parseQueryEvidenceReaderWithCollector("qry-one", bytes.NewReader(data), testQueryProviders(), prepareCollector))
	require.Len(t, prepareCollector.selected, 1)
	require.Contains(t, prepareCollector.selected[selectedMarker].Configuration, "private-selected-hcl")
	for _, result := range prepareCollector.selected {
		require.NotContains(t, fmt.Sprint(result), "private-filtered-hcl")
	}

	prov := &queryDiscoveryProvenance{QueryRunID: "qry-one", Organization: "org", WorkspaceName: "ws", WorkspaceID: "ws-one", GenerateConfigOut: true}
	summaryCollector := newQueryDiscoverySummaryCollector(prov, QueryDiscoveryFilter{NameContains: "selected"})
	// The filtered fixture HCL is intentionally invalid and must not be parsed.
	require.NoError(t, parseQueryEvidenceReaderWithCollector("qry-one", bytes.NewReader(data), testQueryProviders(), summaryCollector))
	require.Equal(t, 2, summaryCollector.summary.ResourcesDiscovered)
	require.Equal(t, 1, summaryCollector.summary.TotalMatching)
	require.Len(t, summaryCollector.summary.Lists, 1)
	require.Len(t, summaryCollector.summary.Lists[0].Results, 1)
	require.Equal(t, "private-selected-hcl", summaryCollector.summary.Lists[0].Results[0].ConfigurationAttributes["bucket"])
	require.NotContains(t, fmt.Sprint(summaryCollector.summary), "private-filtered-hcl")
}

func TestSummaryCollectorDefersProjectionFailureUntilWholeLogValidation(t *testing.T) {
	data := []byte(strings.Join([]string{
		`{"type":"list_resource_found","list_resource_found":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","display_name":"selected","identity":{"bucket":"selected"},"identity_version":0,"configuration":"invalid generated HCL"}}`,
		`{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","total":2}}`,
	}, "\n"))
	prov := &queryDiscoveryProvenance{QueryRunID: "qry-one", GenerateConfigOut: true}
	collector := newQueryDiscoverySummaryCollector(prov, QueryDiscoveryFilter{})

	err := parseQueryEvidenceReaderWithCollector("qry-one", bytes.NewReader(data), testQueryProviders(), collector)
	require.EqualError(t, err, "query_evidence_incomplete")
	require.Empty(t, collector.summary.Lists)
}

func TestValidateQueryEvidenceJSONAcceptsDeepContainersAndRejectsNestedDuplicates(t *testing.T) {
	raw := []byte(strings.Repeat("[", 1000) + `0` + strings.Repeat("]", 1000))
	require.NoError(t, validateQueryEvidenceJSON(raw))

	duplicate := []byte(strings.Repeat("[", 999) + `{"key":1,"key":2}` + strings.Repeat("]", 999))
	require.EqualError(t, validateQueryEvidenceJSON(duplicate), "query_evidence_invalid")
}

func TestQueryEvidenceJSONDepthLimit(t *testing.T) {
	for _, shape := range []struct {
		name, open, close string
	}{
		{"arrays", "[", "]"},
		{"objects", `{"value":`, "}"},
		{"mixed", `[{"value":`, "}]"},
	} {
		// Include extreme input to exercise early rejection without recursively
		// constructing or decoding a million-level Go value.
		for _, depth := range []int{999, 1000, 1001, 1_000_000} {
			t.Run(fmt.Sprintf("%s/%d", shape.name, depth), func(t *testing.T) {
				levels := depth
				prefix, suffix := "", ""
				if shape.name == "mixed" {
					levels = depth / 2
					if depth%2 != 0 {
						prefix, suffix = "[", "]"
					}
				}
				raw := []byte(prefix + strings.Repeat(shape.open, levels) + `"private-value"` + strings.Repeat(shape.close, levels) + suffix)
				var decoded any
				err := decodeQueryEvidence(raw, &decoded)
				if depth > maxQueryEvidenceJSONDepth {
					require.EqualError(t, err, "query_evidence_invalid")
					require.Nil(t, decoded)
				} else {
					require.NoError(t, err)
					require.NotNil(t, decoded)
				}
			})
		}
	}
}

func TestQueryEvidenceIgnoresResourceObject(t *testing.T) {
	data := bytes.Replace(queryEvidenceFixture("0"), []byte(`"identity_version":0`), []byte(`"identity_version":0,"resource_object":{"tags":{"team":"search","nested":{"password":"secret"}},"password":"secret"}`), 1)
	evidence, err := parseQueryEvidenceReader("qry-one", bytes.NewReader(data), testQueryProviders())
	require.NoError(t, err)
	encoded, err := json.Marshal(evidence)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "resource_object")
	require.NotContains(t, string(encoded), "tags")
	require.NotContains(t, string(encoded), "password")
	require.NotContains(t, string(encoded), "secret")
}

func TestQueryEvidencePreservesGeneratedBlocksBeyondFormerLimit(t *testing.T) {
	configuration := strings.Repeat("# generated configuration\n", 2000)
	importBlock := strings.Repeat("# generated import block\n", 2000)
	found, err := json.Marshal(map[string]any{
		"type": "list_resource_found",
		"list_resource_found": map[string]any{
			"address": "list.aws_s3_buckets.all", "resource_type": "aws_s3_buckets",
			"identity": map[string]any{"bucket": "example"}, "identity_version": 0,
			"configuration": configuration, "import_configuration": importBlock,
		},
	})
	require.NoError(t, err)
	log := string(found) + "\n" + `{"type":"list_complete","list_complete":{"address":"list.aws_s3_buckets.all","resource_type":"aws_s3_buckets","total":1}}`
	evidence, err := parseQueryEvidence("qry-one", []byte(log), testQueryProviders())
	require.NoError(t, err)
	require.Equal(t, configuration, evidence.Results[0].Configuration)
	require.Equal(t, importBlock, evidence.Results[0].ImportConfig)
}

func TestQueryEvidenceRejectsUnknownProviderWithoutPartialResults(t *testing.T) {
	evidence, err := parseQueryEvidence("qry-one", queryEvidenceFixture("0"), nil)
	require.Nil(t, evidence)
	require.EqualError(t, err, "query_provider_source_unresolved")
}
