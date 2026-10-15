// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"fmt"
	"strings"
	"testing"

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

func TestQueryEvidenceDoesNotApplyByteOrDepthCaps(t *testing.T) {
	raw := []byte(strings.Repeat("[", 40) + `{"value":"` + strings.Repeat("x", 128*1024) + `"}` + strings.Repeat("]", 40))
	var value any
	require.NoError(t, decodeQueryEvidence(raw, &value))
}

func TestQueryEvidenceRejectsUnknownProviderWithoutPartialResults(t *testing.T) {
	evidence, err := parseQueryEvidence("qry-one", queryEvidenceFixture("0"), nil)
	require.Nil(t, evidence)
	require.EqualError(t, err, "query_provider_source_unresolved")
}
