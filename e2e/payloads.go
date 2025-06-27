// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2e

type ContentType string

const (
	CONST_TYPE_RESOURCE    ContentType = "resources"
	CONST_TYPE_DATA_SOURCE ContentType = "data-sources"
	CONST_TYPE_GUIDES      ContentType = "guides"
	CONST_TYPE_FUNCTIONS   ContentType = "functions"
	CONST_TYPE_OVERVIEW    ContentType = "overview"
)

type RegistryTestCase struct {
	TestName        string                 `json:"testName"`
	TestShouldFail  bool                   `json:"testShouldFail"`
	TestDescription string                 `json:"testDescription"`
	TestContentType ContentType            `json:"testContentType,omitempty"`
	TestPayload     map[string]interface{} `json:"testPayload,omitempty"`
}

var providerTestCases = []RegistryTestCase{
	{
		TestName:        "empty_payload",
		TestShouldFail:  true,
		TestDescription: "Testing with empty payload",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "missing_namespace_and_version",
		TestShouldFail:  true,
		TestDescription: "Testing without providerNamespace and providerVersion",
		TestPayload:     map[string]interface{}{"ProviderName": "google"},
	},
	{
		TestName:        "without_version",
		TestShouldFail:  false,
		TestDescription: "Testing without providerVersion",
		TestPayload: map[string]interface{}{
			"providerName":      "azurerm",
			"providerNamespace": "hashicorp",
			"serviceSlug":       "azurerm_iot_security_solution",
		},
	},
	{
		TestName:        "hashicorp_without_namespace",
		TestShouldFail:  false,
		TestDescription: "Testing without providerNamespace, but owned by hashicorp",
		TestPayload: map[string]interface{}{
			"providerName":    "aws",
			"providerVersion": "latest",
			"serviceSlug":     "aws_s3_bucket",
		},
	},
	{
		TestName:        "third_party_without_namespace",
		TestShouldFail:  true,
		TestDescription: "Testing without providerNamespace, but not-owned by hashicorp",
		TestPayload: map[string]interface{}{
			"providerName":    "snowflake",
			"providerVersion": "latest",
		},
	},
	{
		TestName:        "required_values_resource",
		TestShouldFail:  false,
		TestDescription: "Testing only with required values",
		TestContentType: CONST_TYPE_RESOURCE,
		TestPayload: map[string]interface{}{
			"providerName":      "dns",
			"providerNamespace": "hashicorp",
			"serviceSlug":       "ns_record_set",
		},
	},
	{
		TestName:        "data_source_with_prefix",
		TestShouldFail:  false,
		TestDescription: "Testing only with required values with the providerName prefix",
		TestContentType: CONST_TYPE_DATA_SOURCE,
		TestPayload: map[string]interface{}{
			"providerName":      "dns",
			"providerNamespace": "hashicorp",
			"providerDataType":  "data-sources",
			"serviceSlug":       "dns_ns_record_set",
		},
	},
	{
		TestName:        "third_party_resource",
		TestShouldFail:  false,
		TestDescription: "Testing resources with all values for non-hashicorp providerNamespace",
		TestContentType: CONST_TYPE_RESOURCE,
		TestPayload: map[string]interface{}{
			"providerName":      "pinecone",
			"providerNamespace": "pinecone-io",
			"providerVersion":   "latest",
			"providerDataType":  "resources",
			"serviceSlug":       "pinecone_index",
		},
	},
	{
		TestName:        "third_party_data_source",
		TestShouldFail:  false,
		TestDescription: "Testing data-sources for non-hashicorp providerNamespace",
		TestContentType: CONST_TYPE_DATA_SOURCE,
		TestPayload: map[string]interface{}{
			"providerName":      "terracurl",
			"providerNamespace": "devops-rob",
			"providerDataType":  "data-sources",
			"serviceSlug":       "terracurl",
		},
	},
	{
		TestName:        "malformed_namespace",
		TestShouldFail:  false,
		TestDescription: "Testing payload with malformed providerNamespace",
		TestPayload: map[string]interface{}{
			"providerName":      "vault",
			"providerNamespace": "hashicorp-malformed",
			"providerVersion":   "latest",
			"serviceSlug":       "vault_aws_auth_backend_role",
		},
	},
	{
		TestName:        "malformed_provider_name",
		TestShouldFail:  true,
		TestDescription: "Testing payload with malformed providerName",
		TestPayload: map[string]interface{}{
			"providerName":      "vaults",
			"providerNamespace": "hashicorp",
			"providerVersion":   "latest",
		},
	},
	{
		TestName:        "guides_documentation",
		TestShouldFail:  false,
		TestDescription: "Testing guides documentation with v2 API",
		TestContentType: CONST_TYPE_GUIDES,
		TestPayload: map[string]interface{}{
			"providerName":      "aws",
			"providerNamespace": "hashicorp",
			"providerVersion":   "latest",
			"providerDataType":  "guides",
			"serviceSlug":       "custom-service-endpoints",
		},
	},
	{
		TestName:        "functions_documentation",
		TestShouldFail:  false,
		TestDescription: "Testing functions documentation with v2 API",
		TestContentType: CONST_TYPE_FUNCTIONS,
		TestPayload: map[string]interface{}{
			"providerName":      "google",
			"providerNamespace": "hashicorp",
			"providerVersion":   "latest",
			"providerDataType":  "functions",
			"serviceSlug":       "name_from_id",
		},
	},
	{
		TestName:        "overview_documentation",
		TestShouldFail:  false,
		TestDescription: "Testing overview documentation with v2 API",
		TestContentType: CONST_TYPE_OVERVIEW,
		TestPayload: map[string]interface{}{
			"providerName":      "google",
			"providerNamespace": "hashicorp",
			"providerVersion":   "latest",
			"providerDataType":  "overview",
			"serviceSlug":       "index",
		},
	},
}

var providerDocsTestCases = []RegistryTestCase{
	{
		TestName:        "empty_payload",
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with empty payload",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "empty_doc_id",
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with empty providerDocID",
		TestPayload: map[string]interface{}{
			"providerDocID": "",
		},
	},
	{
		TestName:        "invalid_doc_id",
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with invalid providerDocID",
		TestPayload: map[string]interface{}{
			"providerDocID": "invalid-doc-id",
		},
	},
	{
		TestName:        "valid_doc_id",
		TestShouldFail:  false,
		TestDescription: "Testing providerDocs with all correct providerDocID value",
		TestPayload: map[string]interface{}{
			"providerDocID": "8894603",
		},
	}, {
		TestName:        "incorrect_numeric_doc_id",
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with incorrect numeric providerDocID value",
		TestPayload: map[string]interface{}{
			"providerDocID": "3356809",
		},
	},
}
var searchModulesTestCases = []RegistryTestCase{
	{
		TestName:        "no_parameters",
		TestShouldFail:  true,
		TestDescription: "Testing searchModules with no parameters",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "empty_query_all_modules",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with empty moduleQuery - all modules",
		TestPayload:     map[string]interface{}{"moduleQuery": ""},
	},
	{
		TestName:        "aws_query_no_offset",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery 'aws' - no offset",
		TestPayload: map[string]interface{}{
			"moduleQuery": "aws",
		},
	},
	{
		TestName:        "empty_query_with_offset",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery '' and currentOffset 10",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": 10,
		},
	},
	{
		TestName:        "offset_only",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with currentOffset 5 only - all modules",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": 5,
		},
	},
	{
		TestName:        "negative_offset",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with invalid currentOffset (negative)",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": -1,
		},
	},
	{
		TestName:        "unknown_provider",
		TestShouldFail:  true,
		TestDescription: "Testing searchModules with a moduleQuery not in the map (e.g., 'unknownprovider')",
		TestPayload: map[string]interface{}{
			"moduleQuery": "unknownprovider",
		},
	},
	{
		TestName:        "vsphere_capitalized",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with vSphere (capitalized)",
		TestPayload: map[string]interface{}{
			"moduleQuery": "vSphere",
		},
	},
	{
		TestName:        "aviatrix_provider",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with Aviatrix (handle terraform-provider-modules)",
		TestPayload: map[string]interface{}{
			"moduleQuery": "aviatrix",
		},
	},
	{
		TestName:        "oci_provider",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with oci",
		TestPayload: map[string]interface{}{
			"moduleQuery": "oci",
		},
	},
	{
		TestName:        "query_with_spaces",
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with vertex ai - query with spaces",
		TestPayload: map[string]interface{}{
			"moduleQuery": "vertex ai",
		},
	},
}

var moduleDetailsTestCases = []RegistryTestCase{
	{
		TestName:        "valid_module_id",
		TestShouldFail:  false,
		TestDescription: "Testing moduleDetails with valid moduleID",
		TestPayload: map[string]interface{}{
			"moduleID": "terraform-aws-modules/vpc/aws/2.1.0",
		},
	},
	{
		TestName:        "missing_module_id",
		TestShouldFail:  true,
		TestDescription: "Testing moduleDetails missing moduleID",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "empty_module_id",
		TestShouldFail:  true,
		TestDescription: "Testing moduleDetails with empty moduleID",
		TestPayload: map[string]interface{}{
			"moduleID": "",
		},
	},
	{
		TestName:        "nonexistent_module_id",
		TestShouldFail:  true,
		TestDescription: "Testing moduleDetails with non-existent moduleID",
		TestPayload: map[string]interface{}{
			"moduleID": "hashicorp/nonexistentmodule/aws/1.0.0",
		},
	},
	{
		TestName:        "invalid_format",
		TestShouldFail:  true, // Expecting empty or error, tool call might succeed but return no useful data
		TestDescription: "Testing moduleDetails with invalid moduleID format",
		TestPayload: map[string]interface{}{
			"moduleID": "invalid-format",
		},
	},
}
