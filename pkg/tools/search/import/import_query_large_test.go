// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func largeDiscoveryLog(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `{"type":"list_resource_found","list_resource_found":{"address":"list.aws_iam_role.roles","resource_type":"aws_iam_role","display_name":"role-%d","identity_version":0,"identity":{"account_id":"123456789012","name":"role-%d"}}}`+"\n", i, i)
	}
	fmt.Fprintf(&b, `{"type":"list_complete","list_complete":{"address":"list.aws_iam_role.roles","resource_type":"aws_iam_role","total":%d}}`+"\n", n)
	return []byte(b.String())
}

func TestImportDiscoveryParsesMoreThanOneHundredResults(t *testing.T) {
	providers := map[string]workspaceProvider{"aws_iam_role": {Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.62.0"}}
	for _, n := range []int{100, 101, 1000, 5000} {
		items, err := parseImportDiscovery(largeDiscoveryLog(n), "qry-large", providers)
		require.NoError(t, err, n)
		assert.Len(t, items, n)
	}
}

func TestImportDiscoveryLargeStillFailsClosed(t *testing.T) {
	providers := map[string]workspaceProvider{"aws_iam_role": {Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.62.0"}}
	data := largeDiscoveryLog(1000)

	_, err := parseImportDiscovery([]byte(strings.Replace(string(data), `"total":1000`, `"total":999`, 1)), "qry-large", providers)
	assert.ErrorContains(t, err, "query_evidence_incomplete")

	dup := strings.SplitN(string(data), "\n", 2)[0] + "\n" + string(data)
	_, err = parseImportDiscovery([]byte(dup), "qry-large", providers)
	assert.ErrorContains(t, err, "query_duplicate_identity")
}
