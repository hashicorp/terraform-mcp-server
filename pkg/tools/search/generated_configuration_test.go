// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectGeneratedConfigurationScalarsAndFlatMaps(t *testing.T) {
	attributes, err := projectGeneratedConfiguration(`
resource "aws_instance" "result" {
  provider      = aws
  ami           = "ami-123"
  instance_type = "m7i.large"
  enabled       = true
  capacity      = 9007199254740993
  ratio         = 1.25
  negative      = -7
  tags = {
    Team    = "Search"
    Enabled = true
    Count   = 2
  }
  tags_all = {
    Team = "Search"
  }
}`)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"ami":           "ami-123",
		"instance_type": "m7i.large",
		"enabled":       true,
		"capacity":      json.Number("9007199254740993"),
		"ratio":         json.Number("1.25"),
		"negative":      json.Number("-7"),
		"tags": map[string]any{
			"Team": "Search", "Enabled": true, "Count": json.Number("2"),
		},
		"tags_all": map[string]any{"Team": "Search"},
	}, attributes)
}

func TestProjectGeneratedConfigurationOmitsUnsupportedValues(t *testing.T) {
	attributes, err := projectGeneratedConfiguration(`
resource "example_resource" "result" {
  keep             = "value"
  null_value       = null
  empty_object     = {}
  empty_list       = []
  list_value       = ["omit"]
  nested_object    = { outer = { inner = "omit" } }
  map_with_null    = { one = "value", two = null }
  function_value   = jsonencode({ secret = "omit" })
  reference_value  = var.value
  conditional      = true ? "one" : "two"
  interpolated     = "prefix-${var.value}"

  nested_block {
    secret = "omit"
  }
}`)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"keep": "value"}, attributes)
}

func TestProjectGeneratedConfigurationRejectsNonliteralMapKeys(t *testing.T) {
	attributes, err := projectGeneratedConfiguration(`
resource "example_resource" "result" {
  accepted = {
    identifier = "one"
    "quoted"   = "two"
  }
  conditional_key = {
    (true ? "one" : "two") = "omit"
  }
  interpolated_key = {
    "prefix-${"value"}" = "omit"
  }
}`)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"accepted": map[string]any{"identifier": "one", "quoted": "two"},
	}, attributes)
}

func TestProjectGeneratedConfigurationHandlesAdditionalTerraformShapes(t *testing.T) {
	attributes, err := projectGeneratedConfiguration(`
resource "example_resource" "result" {
  literal_heredoc = <<-EOT
    hello
  EOT
  escaped_template = "literal $${value}"
  special_keys = {
    "key.with.dot" = "value"
    "key with space" = 3
  }

  sensitive_scalar = null # sensitive
  sensitive_map    = {}   # sensitive
  sensitive_list   = []   # sensitive

  arithmetic   = 1 + 2
  static_interpolation = "prefix-${1}"
  indexed      = local.items[0]
  splat        = local.items[*].name
  comprehension = { for key, value in local.items : key => value }
}`)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"literal_heredoc":  "hello\n",
		"escaped_template": "literal ${value}",
		"special_keys": map[string]any{
			"key.with.dot":   "value",
			"key with space": json.Number("3"),
		},
	}, attributes)
}

func TestProjectGeneratedConfigurationCanReturnNoConvenienceAttributes(t *testing.T) {
	attributes, err := projectGeneratedConfiguration(`resource "example_resource" "result" {
  provider = example
  policy = jsonencode({ key = "value" })
  nested_block { value = "ignored" }
}`)
	require.NoError(t, err)
	require.Nil(t, attributes)
}

func TestProjectGeneratedConfigurationRejectsInvalidStructureWithoutDetails(t *testing.T) {
	tests := map[string]string{
		"malformed":           `resource "private_secret`,
		"zero blocks":         `value = "secret"`,
		"multiple blocks":     `resource "a" "one" {} resource "a" "two" {}`,
		"import block":        `import { to = example_resource.result id = "secret" }`,
		"wrong labels":        `resource "example_resource" {}`,
		"top-level attribute": `private = "secret" resource "example_resource" "result" {}`,
	}
	for name, configuration := range tests {
		t.Run(name, func(t *testing.T) {
			attributes, err := projectGeneratedConfiguration(configuration)
			require.Nil(t, attributes)
			require.EqualError(t, err, "query_generated_configuration_invalid")
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "generated.tf")
		})
	}
}

func FuzzProjectGeneratedConfiguration(f *testing.F) {
	f.Add(`resource "example_resource" "result" { name = "example" }`)
	f.Add(`resource "private_secret`)
	f.Fuzz(func(t *testing.T, configuration string) {
		_, _ = projectGeneratedConfiguration(configuration)
	})
}

func BenchmarkProjectGeneratedConfiguration(b *testing.B) {
	var tags strings.Builder
	for i := range 1000 {
		tags.WriteString("\n    key_")
		tags.WriteString(strconv.Itoa(i))
		tags.WriteString(` = "value"`)
	}
	configuration := `resource "aws_instance" "result" {
  ami = "ami-123"
  instance_type = "m7i.large"
  tags = {` + tags.String() + `
  }
}`
	b.ReportAllocs()
	b.SetBytes(int64(len(configuration)))
	for b.Loop() {
		_, err := projectGeneratedConfiguration(configuration)
		if err != nil {
			b.Fatal(err)
		}
	}
}
