// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Preparation half of the source TestPurposeSpecificCollectorsRetainOnlyNeededResults.
// The summary half remains independently covered in query_evidence_test.go.
func TestPrepareCollectorRetainsOnlyNeededResults(t *testing.T) {
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
}
