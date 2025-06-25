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
	TestShouldFail  bool                   `json:"testShouldFail"`
	TestDescription string                 `json:"testDescription"`
	TestContentType ContentType            `json:"testContentType,omitempty"`
	TestPayload     map[string]interface{} `json:"testPayload,omitempty"`
}

var providerTestCases = []RegistryTestCase{
	{
		TestShouldFail:  true,
		TestDescription: "Testing with empty payload",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing without providerNamespace and providerVersion",
		TestPayload:     map[string]interface{}{"ProviderName": "google"},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing without providerVersion",
		TestPayload: map[string]interface{}{
			"providerName":      "azurerm",
			"providerNamespace": "hashicorp",
			"serviceSlug":       "azurerm_iot_security_solution",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing without providerNamespace, but owned by hashicorp",
		TestPayload: map[string]interface{}{
			"providerName":    "aws",
			"providerVersion": "latest",
			"serviceSlug":     "aws_s3_bucket",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing without providerNamespace, but not-owned by hashicorp",
		TestPayload: map[string]interface{}{
			"providerName":    "snowflake",
			"providerVersion": "latest",
		},
	},
	{
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
		TestShouldFail:  true,
		TestDescription: "Testing payload with malformed providerName",
		TestPayload: map[string]interface{}{
			"providerName":      "vaults",
			"providerNamespace": "hashicorp",
			"providerVersion":   "latest",
		},
	},
	{
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
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with empty payload",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with empty providerDocID",
		TestPayload: map[string]interface{}{
			"providerDocID": "",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with invalid providerDocID",
		TestPayload: map[string]interface{}{
			"providerDocID": "invalid-doc-id",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing providerDocs with all correct providerDocID value",
		TestPayload: map[string]interface{}{
			"providerDocID": "8894603",
		},
	}, {
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with incorrect numeric providerDocID value",
		TestPayload: map[string]interface{}{
			"providerDocID": "3356809",
		},
	},
}
var searchModulesTestCases = []RegistryTestCase{
	{
		TestShouldFail:  true,
		TestDescription: "Testing searchModules with no parameters",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with empty moduleQuery - all modules",
		TestPayload:     map[string]interface{}{"moduleQuery": ""},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery 'aws' - no offset",
		TestPayload: map[string]interface{}{
			"moduleQuery": "aws",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery '' and currentOffset 10",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": 10,
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with currentOffset 5 only - all modules",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": 5,
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with invalid currentOffset (negative)",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": -1,
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing searchModules with a moduleQuery not in the map (e.g., 'unknownprovider')",
		TestPayload: map[string]interface{}{
			"moduleQuery": "unknownprovider",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with vSphere (capitalized)",
		TestPayload: map[string]interface{}{
			"moduleQuery": "vSphere",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with Aviatrix (handle terraform-provider-modules)",
		TestPayload: map[string]interface{}{
			"moduleQuery": "aviatrix",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with oci",
		TestPayload: map[string]interface{}{
			"moduleQuery": "oci",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with vertex ai - query with spaces",
		TestPayload: map[string]interface{}{
			"moduleQuery": "vertex ai",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery and namespace",
		TestPayload: map[string]interface{}{
			"moduleQuery": "resourcegroup",
			"namespace":   "Azure",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery and provider",
		TestPayload: map[string]interface{}{
			"moduleQuery": "resourcegroup",
			"provider":    "azurerm",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery and verified true",
		TestPayload: map[string]interface{}{
			"moduleQuery": "resourcegroup",
			"verified":    true,
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery, provider, namespace",
		TestPayload: map[string]interface{}{
			"moduleQuery": "resourcegroup",
			"provider":    "azurerm",
			"namespace":   "azure",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery, provider, namespace, verified",
		TestPayload: map[string]interface{}{
			"moduleQuery": "resourcegroup",
			"provider":    "azurerm",
			"namespace":   "azure",
			"verified":    false,
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing searchModules with moduleQuery = '', currentOffset = 10 and provider",
		TestPayload: map[string]interface{}{
			"moduleQuery":   "",
			"currentOffset": 10,
			"provider":      "azurerm",
		},
	},
}

var moduleDetailsTestCases = []RegistryTestCase{
	{
		TestShouldFail:  false,
		TestDescription: "Testing moduleDetails with valid moduleID",
		TestPayload: map[string]interface{}{
			"moduleID": "terraform-aws-modules/vpc/aws/2.1.0",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing moduleDetails missing moduleID",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing moduleDetails with empty moduleID",
		TestPayload: map[string]interface{}{
			"moduleID": "",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing moduleDetails with non-existent moduleID",
		TestPayload: map[string]interface{}{
			"moduleID": "hashicorp/nonexistentmodule/aws/1.0.0",
		},
	},
	{
		TestShouldFail:  true, // Expecting empty or error, tool call might succeed but return no useful data
		TestDescription: "Testing moduleDetails with invalid moduleID format",
		TestPayload: map[string]interface{}{
			"moduleID": "invalid-format",
		},
	},
}
