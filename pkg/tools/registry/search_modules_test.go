// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalTerraformModules(t *testing.T) {
	t.Run("sorts matches by downloads", func(t *testing.T) {
		resp := []byte(`{
			"meta": {"limit": 15, "current_offset": 0},
			"modules": [
				{"id": "hashicorp/consul/aws/0.1.0", "namespace": "hashicorp", "name": "consul", "provider": "aws", "version": "0.1.0", "downloads": 50, "verified": true},
				{"id": "terraform-aws-modules/vpc/aws/3.14.0", "namespace": "terraform-aws-modules", "name": "vpc", "provider": "aws", "version": "3.14.0", "downloads": 200, "verified": true}
			]
		}`)

		out, err := unmarshalTerraformModules(resp, "aws", nil)
		require.NoError(t, err)
		assert.Contains(t, out, "Available Terraform Modules (top matches) for aws")

		vpcIndex := strings.Index(out, "terraform-aws-modules/vpc/aws/3.14.0")
		consulIndex := strings.Index(out, "hashicorp/consul/aws/0.1.0")
		require.NotEqual(t, -1, vpcIndex)
		require.NotEqual(t, -1, consulIndex)
		assert.Less(t, vpcIndex, consulIndex, "higher download module should come first")
	})

	t.Run("empty modules array is not an error", func(t *testing.T) {
		resp := []byte(`{"meta": {"limit": 15, "current_offset": 0}, "modules": []}`)

		out, err := unmarshalTerraformModules(resp, "zzqxnomatch", nil)
		require.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("missing modules field is not an error", func(t *testing.T) {
		out, err := unmarshalTerraformModules([]byte(`{}`), "zzqxnomatch", nil)
		require.NoError(t, err)
		assert.Empty(t, out)
	})

	t.Run("malformed json is still an error", func(t *testing.T) {
		_, err := unmarshalTerraformModules([]byte(`not json`), "aws", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshalling modules")
	})
}
