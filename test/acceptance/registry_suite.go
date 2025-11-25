package acceptance

import "regexp"

var (
	MatchContentResource      = regexp.MustCompile(`Category: resources`)
	MatchContentDataSource    = regexp.MustCompile(`Category: data-sources`)
	MatchContentGuides        = regexp.MustCompile(`Category: guides`)
	MatchContentFunctions     = regexp.MustCompile(`Category: functions`)
	MatchContentActions       = regexp.MustCompile(`Category: actions`)
	MatchContentListResources = regexp.MustCompile(`Category: list-resources`)
)

var RegistryToolTests = []ToolAcceptanceTest{
	{
		Name:        "get_latest_provider_version_nonexistent",
		Description: "Get latest provider version for a provider that doesn't exist",
		ToolName:    "get_latest_provider_version",
		Arguments: map[string]any{
			"namespace": "initech",
			"name":      "y2kbanksoftware",
		},
		ExpectError: regexp.MustCompile(`404 Not Found`),
	},
	{
		Name:        "get_latest_provider_version",
		Description: "Get latest provider version for a well known provider",
		ToolName:    "get_latest_provider_version",
		Arguments: map[string]any{
			"namespace": "hashicorp",
			"name":      "aws",
		},
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
	},

	// search_providers
	{
		Name:        "search_providers_empty_payload",
		ToolName:    "search_providers",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_providers with empty payload",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "search_providers_missing_namespace_and_version",
		ToolName:    "search_providers",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_providers without provider_namespace and provider_version",
		Arguments:   map[string]interface{}{"provider_name": "google"},
	},
	{
		Name:        "search_providers_without_version",
		ToolName:    "search_providers",
		Description: "Testing search_providers without provider_version",
		Arguments: map[string]interface{}{
			"provider_name":      "azurerm",
			"provider_namespace": "hashicorp",
			"service_slug":       "azurerm_iot_security_solution",
		},
	},
	{
		Name:        "search_providers_hashicorp_without_namespace",
		ToolName:    "search_providers",
		Description: "Testing search_providers without provider_namespace, but owned by hashicorp",
		Arguments: map[string]interface{}{
			"provider_name":    "aws",
			"provider_version": "latest",
			"service_slug":     "aws_s3_bucket",
		},
	},
	{
		Name:        "search_providers_third_party_without_namespace",
		ToolName:    "search_providers",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_providers without provider_namespace, but not-owned by hashicorp",
		Arguments: map[string]interface{}{
			"provider_name":    "snowflake",
			"provider_version": "latest",
		},
	},
	{
		Name:              "search_providers_required_values_resource",
		ToolName:          "search_providers",
		Description:       "Testing search_providers only with required values",
		ExpectTextContent: MatchContentResource,
		Arguments: map[string]interface{}{
			"provider_name":      "dns",
			"provider_namespace": "hashicorp",
			"service_slug":       "ns_record_set",
		},
	},
	{
		Name:              "search_providers_data_source_with_prefix",
		ToolName:          "search_providers",
		Description:       "Testing search_providers only with required values with the provider_name prefix",
		ExpectTextContent: MatchContentDataSource,
		Arguments: map[string]interface{}{
			"provider_name":          "dns",
			"provider_namespace":     "hashicorp",
			"provider_document_type": "data-sources",
			"service_slug":           "dns_ns_record_set",
		},
	},
	{
		Name:              "search_providers_third_party_resource",
		ToolName:          "search_providers",
		Description:       "Testing search_providers resources with all values for non-hashicorp provider_namespace",
		ExpectTextContent: MatchContentResource,
		Arguments: map[string]interface{}{
			"provider_name":          "pinecone",
			"provider_namespace":     "pinecone-io",
			"provider_version":       "latest",
			"provider_document_type": "resources",
			"service_slug":           "pinecone_index",
		},
	},
	{
		Name:              "search_providers_third_party_data_source",
		ToolName:          "search_providers",
		Description:       "Testing search_providers data-sources for non-hashicorp provider_namespace",
		ExpectTextContent: MatchContentDataSource,
		Arguments: map[string]interface{}{
			"provider_name":          "terracurl",
			"provider_namespace":     "devops-rob",
			"provider_document_type": "data-sources",
			"service_slug":           "terracurl",
		},
	},
	{
		Name:        "search_providers_malformed_namespace",
		ToolName:    "search_providers",
		Description: "Testing search_providers payload with malformed provider_namespace",
		Arguments: map[string]interface{}{
			"provider_name":      "vault",
			"provider_namespace": "hashicorp-malformed",
			"provider_version":   "latest",
			"service_slug":       "vault_aws_auth_backend_role",
		},
	},
	{
		Name:        "search_providers_malformed_provider_name",
		ToolName:    "search_providers",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_providers payload with malformed provider_name",
		Arguments: map[string]interface{}{
			"provider_name":      "vaults",
			"provider_namespace": "hashicorp",
			"provider_version":   "latest",
		},
	},
	{
		Name:              "search_providers_guides_documentation",
		ToolName:          "search_providers",
		Description:       "Testing search_providers guides documentation with v2 API",
		ExpectTextContent: MatchContentGuides,
		Arguments: map[string]interface{}{
			"provider_name":          "aws",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "guides",
			"service_slug":           "custom-service-endpoints",
		},
	},
	{
		Name:              "search_providers_functions_documentation",
		ToolName:          "search_providers",
		Description:       "Testing search_providers functions documentation with v2 API",
		ExpectTextContent: MatchContentFunctions,
		Arguments: map[string]interface{}{
			"provider_name":          "google",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "functions",
			"service_slug":           "name_from_id",
		},
	},
	{
		Name:              "search_providers_overview_documentation",
		ToolName:          "search_providers",
		Description:       "Testing search_providers overview documentation with v2 API",
		ExpectTextContent: regexp.MustCompile(`google provider docs`),
		Arguments: map[string]interface{}{
			"provider_name":          "google",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "overview",
			"service_slug":           "index",
		},
	},
	{
		Name:              "search_providers_actions_documentation",
		ToolName:          "search_providers",
		Description:       "Testing search_providers actions documentation with v2 API",
		ExpectTextContent: MatchContentActions,
		Arguments: map[string]interface{}{
			"provider_name":          "aws",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "actions",
			"service_slug":           "ec2",
		},
	},
	{
		Name:              "search_providers_list_resources_documentation",
		ToolName:          "search_providers",
		Description:       "Testing search_providers list-resources documentation with v2 API",
		ExpectTextContent: MatchContentListResources,
		Arguments: map[string]interface{}{
			"provider_name":          "aws",
			"provider_namespace":     "hashicorp",
			"provider_version":       "latest",
			"provider_document_type": "list-resources",
			"service_slug":           "instance",
		},
	},

	// get_provider_details
	{
		Name:        "get_provider_details_empty_payload",
		ToolName:    "get_provider_details",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_provider_details with empty payload",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "get_provider_details_empty_doc_id",
		ToolName:    "get_provider_details",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_provider_details with empty provider_doc_id",
		Arguments: map[string]interface{}{
			"provider_doc_id": "",
		},
	},
	{
		Name:        "get_provider_details_invalid_doc_id",
		ToolName:    "get_provider_details",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_provider_details with invalid provider_doc_id",
		Arguments: map[string]interface{}{
			"provider_doc_id": "invalid-doc-id",
		},
	},
	{
		Name:        "get_provider_details_valid_doc_id",
		ToolName:    "get_provider_details",
		Description: "Testing get_provider_details with all correct provider_doc_id value",
		Arguments: map[string]interface{}{
			"provider_doc_id": "8894603",
		},
		ExpectTextContent: regexp.MustCompile(`aws_ec2_transit_gateway_default_route_table_propagation`),
	},
	{
		Name:        "get_provider_details_incorrect_numeric_doc_id",
		ToolName:    "get_provider_details",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_provider_details with incorrect numeric provider_doc_id value",
		Arguments: map[string]interface{}{
			"provider_doc_id": "3356809",
		},
	},

	// search_modules
	{
		Name:        "search_module_no_parameters",
		ToolName:    "search_modules",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_modules with no parameters",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:              "search_module_empty_query_all_modules",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with empty module_query - all modules",
		Arguments:         map[string]interface{}{"module_query": ""},
	},
	{
		Name:              "search_module_aws_query_no_offset",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with module_query 'aws' - no offset",
		Arguments: map[string]interface{}{
			"module_query": "aws",
		},
	},
	{
		Name:              "search_module_empty_query_with_offset",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with module_query '' and current_offset 10",
		Arguments: map[string]interface{}{
			"module_query":   "",
			"current_offset": 10,
		},
	},
	{
		Name:              "search_module_offset_only",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with current_offset 5 only - all modules",
		Arguments: map[string]interface{}{
			"module_query":   "",
			"current_offset": 5,
		},
	},
	{
		Name:              "search_module_negative_offset",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with invalid current_offset (negative)",
		Arguments: map[string]interface{}{
			"module_query":   "",
			"current_offset": -1,
		},
	},
	{
		Name:        "search_module_unknown_provider",
		ToolName:    "search_modules",
		ExpectError: regexp.MustCompile(`no modules found for query`),
		Description: "Testing search_modules with a module_query not in the map (e.g., 'unknownprovider')",
		Arguments: map[string]interface{}{
			"module_query": "unknownprovider",
		},
	},
	{
		Name:              "search_module_vsphere_capitalized",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with vSphere (capitalized)",
		Arguments: map[string]interface{}{
			"module_query": "vSphere",
		},
	},
	{
		Name:              "search_module_aviatrix_provider",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with Aviatrix (handle terraform-provider-modules)",
		Arguments: map[string]interface{}{
			"module_query": "aviatrix",
		},
	},
	{
		Name:              "search_module_oci_provider",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with oci",
		Arguments: map[string]interface{}{
			"module_query": "oci",
		},
	},
	{
		Name:              "search_module_query_with_spaces",
		ToolName:          "search_modules",
		ExpectTextContent: regexp.MustCompile(`Available Terraform Modules`),
		Description:       "Testing search_modules with vertex ai - query with spaces",
		Arguments: map[string]interface{}{
			"module_query": "vertex ai",
		},
	},

	// get_module_details
	{
		Name:              "get_module_details_valid_module_id",
		ToolName:          "get_module_details",
		ExpectTextContent: regexp.MustCompile(`Terraform module to create AWS VPC resources`),
		Description:       "Testing get_module_details with valid module_id",
		Arguments: map[string]interface{}{
			"module_id": "terraform-aws-modules/vpc/aws/2.1.0",
		},
	},
	{
		Name:        "get_module_details_missing_module_id",
		ToolName:    "get_module_details",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing get_module_details missing module_id",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "get_module_details_empty_module_id",
		ToolName:    "get_module_details",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing get_module_details with empty module_id",
		Arguments: map[string]interface{}{
			"module_id": "",
		},
	},
	{
		Name:        "get_module_details_nonexistent_module_id",
		ToolName:    "get_module_details",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing get_module_details with non-existent module_id",
		Arguments: map[string]interface{}{
			"module_id": "hashicorp/nonexistentmodule/aws/1.0.0",
		},
	},
	{
		Name:        "get_module_details_invalid_format",
		ToolName:    "get_module_details",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing get_module_details with invalid module_id format",
		Arguments: map[string]interface{}{
			"module_id": "invalid-format",
		},
	},

	// search_policies
	{
		Name:        "search_policies_empty_payload",
		ToolName:    "search_policies",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_policies with empty payload",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "search_policies_empty_policy_query",
		ToolName:    "search_policies",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_policies with empty policy_query",
		Arguments: map[string]interface{}{
			"policy_query": "",
		},
	},
	{
		Name:     "search_policies_valid_hashicorp_policy_name",
		ToolName: "search_policies",
		Checks: []ToolTestCheck{
			CheckTextContentContains("terraform_policy_id"),
			CheckTextContentContains("Name:"),
			CheckTextContentContains("Title:"),
			CheckTextContentContains("Downloads:"),
		},
		Description: "Testing search_policies with a valid hashicorp policy name",
		Arguments: map[string]interface{}{
			"policy_query": "aws",
		},
	},
	{
		Name:     "search_policies_valid_policy_title_substring",
		ToolName: "search_policies",
		Checks: []ToolTestCheck{
			CheckTextContentContains("terraform_policy_id"),
			CheckTextContentContains("Name:"),
			CheckTextContentContains("Title:"),
			CheckTextContentContains("Downloads:"),
		},
		Description: "Testing search_policies with a valid policy title substring",
		Arguments: map[string]interface{}{
			"policy_query": "security",
		},
	},
	{
		Name:        "search_policies_invalid_nonexistent_policy_name",
		ToolName:    "search_policies",
		ExpectError: regexp.MustCompile(`internal error`),
		Description: "Testing search_policies with an invalid/nonexistent policy name",
		Arguments: map[string]interface{}{
			"policy_query": "nonexistentpolicyxyz123",
		},
	},
	{
		Name:     "search_policies_mixed_case_input",
		ToolName: "search_policies",
		Checks: []ToolTestCheck{
			CheckTextContentContains("terraform_policy_id"),
			CheckTextContentContains("Name:"),
			CheckTextContentContains("Title:"),
			CheckTextContentContains("Downloads:"),
		},
		Description: "Testing search_policies with mixed case input",
		Arguments: map[string]interface{}{
			"policy_query": "TeRrAfOrM",
		},
	},
	{
		Name:     "search_policies_special_characters",
		ToolName: "search_policies",
		Checks: []ToolTestCheck{
			CheckTextContentContains("terraform_policy_id"),
			CheckTextContentContains("Name:"),
			CheckTextContentContains("Title:"),
			CheckTextContentContains("Downloads:"),
		},
		Description: "Testing search_policies with policy name containing special characters",
		Arguments: map[string]interface{}{
			"policy_query": "cis-policy",
		},
	},
	{
		Name:     "search_policies_spaces",
		ToolName: "search_policies",
		Checks: []ToolTestCheck{
			CheckTextContentContains("terraform_policy_id"),
			CheckTextContentContains("Name:"),
			CheckTextContentContains("Title:"),
			CheckTextContentContains("Downloads:"),
		},
		Description: "Testing search_policies with policy name containing spaces",
		Arguments: map[string]interface{}{
			"policy_query": "FSBP Foundations benchmark",
		},
	},

	// get_policy_details
	{
		Name:     "get_policy_details_valid",
		ToolName: "get_policy_details",
		Checks: []ToolTestCheck{
			CheckTextContentContains("POLICY_NAME"),
			CheckTextContentContains("POLICY_CHECKSUM"),
		},
		Description: "Testing get_policy_details with valid terraform_policy_id",
		Arguments: map[string]interface{}{
			"terraform_policy_id": "policies/hashicorp/azure-storage-terraform/1.0.2",
		},
	},
	{
		Name:        "get_policy_details_missing_id",
		ToolName:    "get_policy_details",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_policy_details with missing terraform_policy_id",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "get_policy_details_empty_id",
		ToolName:    "get_policy_details",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_policy_details with empty terraform_policy_id",
		Arguments: map[string]interface{}{
			"terraform_policy_id": "",
		},
	},
	{
		Name:        "get_policy_details_nonexistent_id",
		ToolName:    "get_policy_details",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_policy_details with non-existent terraform_policy_id",
		Arguments: map[string]interface{}{
			"terraform_policy_id": "nonexistent-policy-xyz",
		},
	},
	{
		Name:        "get_policy_details_malformed_id",
		ToolName:    "get_policy_details",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_policy_details with malformed terraform_policy_id",
		Arguments: map[string]interface{}{
			"terraform_policy_id": "malformed!@#",
		},
	},

	// get_latest_module_version
	{
		Name:              "get_latest_module_version_valid_aws_module",
		ToolName:          "get_latest_module_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_module_version with valid AWS module",
		Arguments: map[string]interface{}{
			"module_publisher": "terraform-aws-modules",
			"module_name":      "vpc",
			"module_provider":  "aws",
		},
	},
	{
		Name:              "get_latest_module_version_valid_aws_module_case_insensitivity",
		ToolName:          "get_latest_module_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_module_version with valid but case insensitive AWS module",
		Arguments: map[string]interface{}{
			"module_publisher": "TerraFORM-AwS-ModuLES",
			"module_name":      "VpC",
			"module_provider":  "AWs",
		},
	},
	{
		Name:              "get_latest_module_version_valid_hashicorp_module",
		ToolName:          "get_latest_module_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_module_version with valid HashiCorp module",
		Arguments: map[string]interface{}{
			"module_publisher": "hashicorp",
			"module_name":      "consul",
			"module_provider":  "aws",
		},
	},
	{
		Name:        "get_latest_module_version_missing_module_publisher",
		ToolName:    "get_latest_module_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_module_version with missing module_publisher",
		Arguments: map[string]interface{}{
			"module_name":     "vpc",
			"module_provider": "aws",
		},
	},
	{
		Name:        "get_latest_module_version_missing_module_name",
		ToolName:    "get_latest_module_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_module_version with missing module_name",
		Arguments: map[string]interface{}{
			"module_publisher": "terraform-aws-modules",
			"module_provider":  "aws",
		},
	},
	{
		Name:        "get_latest_module_version_missing_module_provider",
		ToolName:    "get_latest_module_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_module_version with missing module_provider",
		Arguments: map[string]interface{}{
			"module_publisher": "terraform-aws-modules",
			"module_name":      "vpc",
		},
	},
	{
		Name:        "get_latest_module_version_empty_parameters",
		ToolName:    "get_latest_module_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_module_version with empty parameters",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "get_latest_module_version_nonexistent_module",
		ToolName:    "get_latest_module_version",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_latest_module_version with nonexistent module",
		Arguments: map[string]interface{}{
			"module_publisher": "nonexistent-publisher",
			"module_name":      "nonexistent-module",
			"module_provider":  "nonexistent-provider",
		},
	},
	{
		Name:              "get_latest_module_version_valid_google_module",
		ToolName:          "get_latest_module_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_module_version with valid Google module",
		Arguments: map[string]interface{}{
			"module_publisher": "terraform-google-modules",
			"module_name":      "network",
			"module_provider":  "google",
		},
	},
	{
		Name:              "get_latest_module_version_valid_azure_module",
		ToolName:          "get_latest_module_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_module_version with valid Azure module",
		Arguments: map[string]interface{}{
			"module_publisher": "Azure",
			"module_name":      "network",
			"module_provider":  "azurerm",
		},
	},

	// get_latest_provider_version
	{
		Name:              "get_latest_provider_version_valid_aws_provider",
		ToolName:          "get_latest_provider_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_provider_version with valid AWS provider",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "aws",
		},
	},
	{
		Name:              "get_latest_provider_version_valid_aws_provider_case_insensitive",
		ToolName:          "get_latest_provider_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_provider_version with valid AWS provider with case insensitivity",
		Arguments: map[string]interface{}{
			"namespace": "HashiCORp",
			"name":      "AwS",
		},
	},
	{
		Name:              "get_latest_provider_version_valid_google_provider",
		ToolName:          "get_latest_provider_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_provider_version with valid Google provider",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "google",
		},
	},
	{
		Name:              "get_latest_provider_version_valid_azurerm_provider",
		ToolName:          "get_latest_provider_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_provider_version with valid Azure provider",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "azurerm",
		},
	},
	{
		Name:        "get_latest_provider_version_missing_namespace",
		ToolName:    "get_latest_provider_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_provider_version with missing namespace",
		Arguments: map[string]interface{}{
			"name": "aws",
		},
	},
	{
		Name:        "get_latest_provider_version_missing_name",
		ToolName:    "get_latest_provider_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_provider_version with missing name",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
		},
	},
	{
		Name:        "get_latest_provider_version_empty_parameters",
		ToolName:    "get_latest_provider_version",
		ExpectError: regexp.MustCompile(`required input`),
		Description: "Testing get_latest_provider_version with empty parameters",
		Arguments:   map[string]interface{}{},
	},
	{
		Name:        "get_latest_provider_version_nonexistent_provider",
		ToolName:    "get_latest_provider_version",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_latest_provider_version with nonexistent provider",
		Arguments: map[string]interface{}{
			"namespace": "nonexistent-namespace",
			"name":      "nonexistent-provider",
		},
	},
	{
		Name:              "get_latest_provider_version_valid_third_party_provider",
		ToolName:          "get_latest_provider_version",
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
		Description:       "Testing get_latest_provider_version with valid third-party provider",
		Arguments: map[string]interface{}{
			"namespace": "datadog",
			"name":      "datadog",
		},
	},
	{
		Name:        "get_latest_provider_version_empty_namespace",
		ToolName:    "get_latest_provider_version",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_latest_provider_version with empty namespace",
		Arguments: map[string]interface{}{
			"namespace": "",
			"name":      "aws",
		},
	},
	{
		Name:        "get_latest_provider_version_empty_name",
		ToolName:    "get_latest_provider_version",
		ExpectError: regexp.MustCompile(`404 Not Found`),
		Description: "Testing get_latest_provider_version with empty name",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "",
		},
	},
}
