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
		TestDescription: "Testing without provider_namespace and provider_version",
		TestPayload:     map[string]interface{}{"provider_name": "google"},
	},
	{
		TestName:        "without_version",
		TestShouldFail:  false,
		TestDescription: "Testing without provider_version",
		TestPayload: map[string]interface{}{
			"provider_name":      "azurerm",
			"provider_namespace": "hashicorp",
			"service_slug":       "azurerm_iot_security_solution",
		},
	},
	{
		TestName:        "hashicorp_without_namespace",
		TestShouldFail:  false,
		TestDescription: "Testing without provider_namespace, but owned by hashicorp",
		TestPayload: map[string]interface{}{
			"provider_name":    "aws",
			"provider_version": "latest",
			"service_slug":     "aws_s3_bucket",
		},
	},
	{
		TestName:        "third_party_without_namespace",
		TestShouldFail:  true,
		TestDescription: "Testing without provider_namespace, but not-owned by hashicorp",
		TestPayload: map[string]interface{}{
			"provider_name":    "snowflake",
			"provider_version": "latest",
		},
	},
	{
		TestName:        "required_values_resource",
		TestShouldFail:  false,
		TestDescription: "Testing only with required values",
		TestContentType: CONST_TYPE_RESOURCE,
		TestPayload: map[string]interface{}{
			"provider_name":      "dns",
			"provider_namespace": "hashicorp",
			"service_slug":       "ns_record_set",
		},
	},
	{
		TestName:        "data_source_with_prefix",
		TestShouldFail:  false,
		TestDescription: "Testing only with required values with the provider_name prefix",
		TestContentType: CONST_TYPE_DATA_SOURCE,
		TestPayload: map[string]interface{}{
			"provider_name":      "dns",
			"provider_namespace": "hashicorp",
			"provider_data_type": "data-sources",
			"service_slug":       "dns_ns_record_set",
		},
	},
	{
		TestName:        "third_party_resource",
		TestShouldFail:  false,
		TestDescription: "Testing resources with all values for non-hashicorp provider_namespace",
		TestContentType: CONST_TYPE_RESOURCE,
		TestPayload: map[string]interface{}{
			"provider_name":      "pinecone",
			"provider_namespace": "pinecone-io",
			"provider_version":   "latest",
			"provider_data_type": "resources",
			"service_slug":       "pinecone_index",
		},
	},
	{
		TestName:        "third_party_data_source",
		TestShouldFail:  false,
		TestDescription: "Testing data-sources for non-hashicorp provider_namespace",
		TestContentType: CONST_TYPE_DATA_SOURCE,
		TestPayload: map[string]interface{}{
			"provider_name":      "terracurl",
			"provider_namespace": "devops-rob",
			"provider_data_type": "data-sources",
			"service_slug":       "terracurl",
		},
	},
	{
		TestName:        "malformed_namespace",
		TestShouldFail:  false,
		TestDescription: "Testing payload with malformed provider_namespace",
		TestPayload: map[string]interface{}{
			"provider_name":      "vault",
			"provider_namespace": "hashicorp-malformed",
			"provider_version":   "latest",
			"service_slug":       "vault_aws_auth_backend_role",
		},
	},
	{
		TestName:        "malformed_provider_name",
		TestShouldFail:  true,
		TestDescription: "Testing payload with malformed provider_name",
		TestPayload: map[string]interface{}{
			"provider_name":      "vaults",
			"provider_namespace": "hashicorp",
			"provider_version":   "latest",
		},
	},
	{
		TestName:        "guides_documentation",
		TestShouldFail:  false,
		TestDescription: "Testing guides documentation with v2 API",
		TestContentType: CONST_TYPE_GUIDES,
		TestPayload: map[string]interface{}{
			"provider_name":      "aws",
			"provider_namespace": "hashicorp",
			"provider_version":   "latest",
			"provider_data_type": "guides",
			"service_slug":       "custom-service-endpoints",
		},
	},
	{
		TestName:        "functions_documentation",
		TestShouldFail:  false,
		TestDescription: "Testing functions documentation with v2 API",
		TestContentType: CONST_TYPE_FUNCTIONS,
		TestPayload: map[string]interface{}{
			"provider_name":      "google",
			"provider_namespace": "hashicorp",
			"provider_version":   "latest",
			"provider_data_type": "functions",
			"service_slug":       "name_from_id",
		},
	},
	{
		TestName:        "overview_documentation",
		TestShouldFail:  false,
		TestDescription: "Testing overview documentation with v2 API",
		TestContentType: CONST_TYPE_OVERVIEW,
		TestPayload: map[string]interface{}{
			"provider_name":      "google",
			"provider_namespace": "hashicorp",
			"provider_version":   "latest",
			"provider_data_type": "overview",
			"service_slug":       "index",
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
		TestDescription: "Testing providerDocs with empty provider_doc_id",
		TestPayload: map[string]interface{}{
			"provider_doc_id": "",
		},
	},
	{
		TestName:        "invalid_doc_id",
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with invalid provider_doc_id",
		TestPayload: map[string]interface{}{
			"provider_doc_id": "invalid-doc-id",
		},
	},
	{
		TestName:        "valid_doc_id",
		TestShouldFail:  false,
		TestDescription: "Testing providerDocs with all correct provider_doc_id value",
		TestPayload: map[string]interface{}{
			"provider_doc_id": "8894603",
		},
	}, {
		TestName:        "incorrect_numeric_doc_id",
		TestShouldFail:  true,
		TestDescription: "Testing providerDocs with incorrect numeric provider_doc_id value",
		TestPayload: map[string]interface{}{
			"provider_doc_id": "3356809",
		},
	},
}
var searchModulesTestCases = []RegistryTestCase{
	{
		TestName:        "no_parameters",
		TestShouldFail:  true,
		TestDescription: "Testing search_modules with no parameters",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "empty_query_all_modules",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with empty module_query - all modules",
		TestPayload:     map[string]interface{}{"module_query": ""},
	},
	{
		TestName:        "aws_query_no_offset",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with module_query 'aws' - no offset",
		TestPayload: map[string]interface{}{
			"module_query": "aws",
		},
	},
	{
		TestName:        "empty_query_with_offset",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with module_query '' and current_offset 10",
		TestPayload: map[string]interface{}{
			"module_query":   "",
			"current_offset": 10,
		},
	},
	{
		TestName:        "offset_only",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with current_offset 5 only - all modules",
		TestPayload: map[string]interface{}{
			"module_query":   "",
			"current_offset": 5,
		},
	},
	{
		TestName:        "negative_offset",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with invalid current_offset (negative)",
		TestPayload: map[string]interface{}{
			"module_query":   "",
			"current_offset": -1,
		},
	},
	{
		TestName:        "unknown_provider",
		TestShouldFail:  true,
		TestDescription: "Testing search_modules with a module_query not in the map (e.g., 'unknownprovider')",
		TestPayload: map[string]interface{}{
			"module_query": "unknownprovider",
		},
	},
	{
		TestName:        "vsphere_capitalized",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with vSphere (capitalized)",
		TestPayload: map[string]interface{}{
			"module_query": "vSphere",
		},
	},
	{
		TestName:        "aviatrix_provider",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with Aviatrix (handle terraform-provider-modules)",
		TestPayload: map[string]interface{}{
			"module_query": "aviatrix",
		},
	},
	{
		TestName:        "oci_provider",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with oci",
		TestPayload: map[string]interface{}{
			"module_query": "oci",
		},
	},
	{
		TestName:        "query_with_spaces",
		TestShouldFail:  false,
		TestDescription: "Testing search_modules with vertex ai - query with spaces",
		TestPayload: map[string]interface{}{
			"module_query": "vertex ai",
		},
	},
}

var moduleDetailsTestCases = []RegistryTestCase{
	{
		TestName:        "valid_module_id",
		TestShouldFail:  false,
		TestDescription: "Testing module_details with valid module_id",
		TestPayload: map[string]interface{}{
			"module_id": "terraform-aws-modules/vpc/aws/2.1.0",
		},
	},
	{
		TestName:        "missing_module_id",
		TestShouldFail:  true,
		TestDescription: "Testing module_details missing module_id",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "empty_module_id",
		TestShouldFail:  true,
		TestDescription: "Testing module_details with empty module_id",
		TestPayload: map[string]interface{}{
			"module_id": "",
		},
	},
	{
		TestName:        "nonexistent_module_id",
		TestShouldFail:  true,
		TestDescription: "Testing module_details with non-existent module_id",
		TestPayload: map[string]interface{}{
			"module_id": "hashicorp/nonexistentmodule/aws/1.0.0",
		},
	},
	{
		TestName:        "invalid_format",
		TestShouldFail:  true, // Expecting empty or error, tool call might succeed but return no useful data
		TestDescription: "Testing module_details with invalid module_id format",
		TestPayload: map[string]interface{}{
			"module_id": "invalid-format",
		},
	},
}

var searchPoliciesTestCases = []RegistryTestCase{
	{
		TestShouldFail:  true,
		TestDescription: "Testing search_policies with empty payload",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing search_policies with empty policy_query",
		TestPayload: map[string]interface{}{
			"policy_query": "",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing search_policies with a valid hashicorp policy name",
		TestPayload: map[string]interface{}{
			"policy_query": "aws",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing search_policies with a valid policy title substring",
		TestPayload: map[string]interface{}{
			"policy_query": "security",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing search_policies with an invalid/nonexistent policy name",
		TestPayload: map[string]interface{}{
			"policy_query": "nonexistentpolicyxyz123",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing search_policies with mixed case input",
		TestPayload: map[string]interface{}{
			"policy_query": "TeRrAfOrM",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing search_policies with policy name containing special characters",
		TestPayload: map[string]interface{}{
			"policy_query": "cis-policy",
		},
	},
	{
		TestShouldFail:  false,
		TestDescription: "Testing search_policies with policy name containing spaces",
		TestPayload: map[string]interface{}{
			"policy_query": "FSBP Foundations benchmark",
		},
	},
}

var policyDetailsTestCases = []RegistryTestCase{
	{
		TestShouldFail:  false,
		TestDescription: "Testing policy_details with valid terraform_policy_id",
		TestPayload: map[string]interface{}{
			"terraform_policy_id": "policies/hashicorp/azure-storage-terraform/1.0.2",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing policy_details with missing terraform_policy_id",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing policy_details with empty terraform_policy_id",
		TestPayload: map[string]interface{}{
			"terraform_policy_id": "",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing policy_details with non-existent terraform_policy_id",
		TestPayload: map[string]interface{}{
			"terraform_policy_id": "nonexistent-policy-xyz",
		},
	},
	{
		TestShouldFail:  true,
		TestDescription: "Testing policy_details with malformed terraform_policy_id",
		TestPayload: map[string]interface{}{
			"terraform_policy_id": "malformed!@#",
		},
	},
}

var getLatestModuleVersionTestCases = []RegistryTestCase{
	{
		TestName:        "valid_aws_module",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_module_version with valid AWS module",
		TestPayload: map[string]interface{}{
			"module_publisher": "terraform-aws-modules",
			"module_name":      "vpc",
			"module_provider":  "aws",
		},
	},
	{
		TestName:        "valid_aws_module_case_insensitivity",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_module_version with valid but case insensitive AWS module",
		TestPayload: map[string]interface{}{
			"module_publisher": "TerraFORM-AwS-ModuLES",
			"module_name":      "VpC",
			"module_provider":  "AWs",
		},
	},
	{
		TestName:        "valid_hashicorp_module",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_module_version with valid HashiCorp module",
		TestPayload: map[string]interface{}{
			"module_publisher": "hashicorp",
			"module_name":      "consul",
			"module_provider":  "aws",
		},
	},
	{
		TestName:        "missing_module_publisher",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_module_version with missing module_publisher",
		TestPayload: map[string]interface{}{
			"module_name":     "vpc",
			"module_provider": "aws",
		},
	},
	{
		TestName:        "missing_module_name",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_module_version with missing module_name",
		TestPayload: map[string]interface{}{
			"module_publisher": "terraform-aws-modules",
			"module_provider":  "aws",
		},
	},
	{
		TestName:        "missing_module_provider",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_module_version with missing module_provider",
		TestPayload: map[string]interface{}{
			"module_publisher": "terraform-aws-modules",
			"module_name":      "vpc",
		},
	},
	{
		TestName:        "empty_parameters",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_module_version with empty parameters",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "nonexistent_module",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_module_version with nonexistent module",
		TestPayload: map[string]interface{}{
			"module_publisher": "nonexistent-publisher",
			"module_name":      "nonexistent-module",
			"module_provider":  "nonexistent-provider",
		},
	},
	{
		TestName:        "valid_google_module",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_module_version with valid Google module",
		TestPayload: map[string]interface{}{
			"module_publisher": "terraform-google-modules",
			"module_name":      "network",
			"module_provider":  "google",
		},
	},
	{
		TestName:        "valid_azure_module",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_module_version with valid Azure module",
		TestPayload: map[string]interface{}{
			"module_publisher": "Azure",
			"module_name":      "network",
			"module_provider":  "azurerm",
		},
	},
}

var getLatestProviderVersionTestCases = []RegistryTestCase{
	{
		TestName:        "valid_aws_provider",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_provider_version with valid AWS provider",
		TestPayload: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "aws",
		},
	},
	{
		TestName:        "valid_aws_provider_case_insensitive",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_provider_version with valid AWS provider with case insensitivity",
		TestPayload: map[string]interface{}{
			"namespace": "HashiCORp",
			"name":      "AwS",
		},
	},
	{
		TestName:        "valid_google_provider",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_provider_version with valid Google provider",
		TestPayload: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "google",
		},
	},
	{
		TestName:        "valid_azurerm_provider",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_provider_version with valid Azure provider",
		TestPayload: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "azurerm",
		},
	},
	{
		TestName:        "missing_namespace",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_provider_version with missing namespace",
		TestPayload: map[string]interface{}{
			"name": "aws",
		},
	},
	{
		TestName:        "missing_name",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_provider_version with missing name",
		TestPayload: map[string]interface{}{
			"namespace": "hashicorp",
		},
	},
	{
		TestName:        "empty_parameters",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_provider_version with empty parameters",
		TestPayload:     map[string]interface{}{},
	},
	{
		TestName:        "nonexistent_provider",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_provider_version with nonexistent provider",
		TestPayload: map[string]interface{}{
			"namespace": "nonexistent-namespace",
			"name":      "nonexistent-provider",
		},
	},
	{
		TestName:        "valid_third_party_provider",
		TestShouldFail:  false,
		TestDescription: "Testing get_latest_provider_version with valid third-party provider",
		TestPayload: map[string]interface{}{
			"namespace": "datadog",
			"name":      "datadog",
		},
	},
	{
		TestName:        "empty_namespace",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_provider_version with empty namespace",
		TestPayload: map[string]interface{}{
			"namespace": "",
			"name":      "aws",
		},
	},
	{
		TestName:        "empty_name",
		TestShouldFail:  true,
		TestDescription: "Testing get_latest_provider_version with empty name",
		TestPayload: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "",
		},
	},
}
