// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ResolveProviderDocID creates a tool to get provider details from registry.
func ResolveProviderDocID(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("resolveProviderDocID",
			mcp.WithDescription(`This tool retrieves a list of potential documents based on the serviceSlug and providerDataType provided. You MUST call this function before 'getProviderDocs' to obtain a valid tfprovider-compatible providerDocID. 
			Use the most relevant single word as the search query for serviceSlug, if unsure about the serviceSlug, use the providerName for its value.
			When selecting the best match, consider: - Title similarity to the query - Category relevance Return the selected providerDocID and explain your choice.  
			If there are multiple good matches, mention this but proceed with the most relevant one.`),
			mcp.WithTitleAnnotation("Identify the most relevant provider document ID for a Terraform service"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("providerName", mcp.Required(), mcp.Description("The name of the Terraform provider to perform the read or deployment operation")),
			mcp.WithString("providerNamespace", mcp.Required(), mcp.Description("The publisher of the Terraform provider, typically the name of the company, or their GitHub organization name that created the provider")),
			mcp.WithString("serviceSlug", mcp.Required(), mcp.Description("The slug of the service you want to deploy or read using the Terraform provider, prefer using a single word, use underscores for multiple words and if unsure about the serviceSlug, use the providerName for its value")),
			mcp.WithString("providerDataType", mcp.Description("The type of the document to retrieve, for general information use 'guides', for deploying resources use 'resources', for reading pre-deployed resources use 'data-sources', for functions use 'functions', and for overview of the provider use 'overview'"),
				mcp.Enum("resources", "data-sources", "functions", "guides", "overview"),
				mcp.DefaultString("resources"),
			),
			mcp.WithString("providerVersion", mcp.Description("The version of the Terraform provider to retrieve in the format 'x.y.z', or 'latest' to get the latest version")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

			// For typical provider and namespace hallucinations
			defaultErrorGuide := "please check the provider name, provider namespace or the provider version you're looking for, perhaps the provider is published under a different namespace or company name"
			providerDetail, err := resolveProviderDetails(request, registryClient, defaultErrorGuide, logger)
			if err != nil {
				return nil, err
			}

			serviceSlug, err := request.RequireString("serviceSlug")
			if err != nil {
				return nil, logAndReturnError(logger, "serviceSlug is required", err)
			}
			if serviceSlug == "" {
				return nil, logAndReturnError(logger, "serviceSlug cannot be empty", nil)
			}

			providerDataType := request.GetString("providerDataType", "resources")
			providerDetail.ProviderDataType = providerDataType

			// Check if we need to use v2 API for guides, functions, or overview
			if isV2ProviderDataType(providerDetail.ProviderDataType) {
				content, err := GetProviderDocsV2(registryClient, providerDetail, logger)
				if err != nil {
					errMessage := fmt.Sprintf(`No %s documentation found for provider '%s' in the '%s' namespace, %s`,
						providerDetail.ProviderDataType, providerDetail.ProviderName, providerDetail.ProviderNamespace, defaultErrorGuide)
					return nil, logAndReturnError(logger, errMessage, err)
				}

				fullContent := fmt.Sprintf("# %s provider docs\n\n%s",
					providerDetail.ProviderName, content)

				return mcp.NewToolResultText(fullContent), nil
			}

			// For resources/data-sources, use the v1 API for better performance (single response)
			uri := fmt.Sprintf("providers/%s/%s/%s", providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion)
			response, err := sendRegistryCall(registryClient, "GET", uri, logger)
			if err != nil {
				return nil, logAndReturnError(logger, fmt.Sprintf(`Error getting the "%s" provider, 
					with version "%s" in the %s namespace, %s`, providerDetail.ProviderName, providerDetail.ProviderVersion, providerDetail.ProviderNamespace, defaultErrorGuide), nil)
			}

			var providerDocs ProviderDocs
			if err := json.Unmarshal(response, &providerDocs); err != nil {
				return nil, logAndReturnError(logger, "unmarshalling provider docs", err)
			}

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("Available Documentation (top matches) for %s in Terraform provider %s/%s version: %s\n\n", providerDetail.ProviderDataType, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion))
			builder.WriteString("Each result includes:\n- providerDocID: tfprovider-compatible identifier\n- Title: Service or resource name\n- Category: Type of document\n")
			builder.WriteString("For best results, select libraries based on the serviceSlug match and category of information requested.\n\n---\n\n")

			contentAvailable := false
			for _, doc := range providerDocs.Docs {
				if doc.Language == "hcl" && doc.Category == providerDetail.ProviderDataType {
					cs, err := containsSlug(doc.Slug, serviceSlug)
					cs_pn, err_pn := containsSlug(fmt.Sprintf("%s_%s", providerDetail.ProviderName, doc.Slug), serviceSlug)
					if (cs || cs_pn) && err == nil && err_pn == nil {
						contentAvailable = true
						builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Title: %s\n- Category: %s\n---\n", doc.ID, doc.Title, doc.Category))
					}
				}
			}

			// Check if the content data is not fulfilled
			if !contentAvailable {
				errMessage := fmt.Sprintf(`No documentation found for serviceSlug %s, provide a more relevant serviceSlug if unsure, use the providerName for its value`, serviceSlug)
				return nil, logAndReturnError(logger, errMessage, err)
			}
			return mcp.NewToolResultText(builder.String()), nil
		}
}

// GetProviderDocs creates a tool to get provider docs for a specific service from registry.
func GetProviderDocs(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("getProviderDocs",
			mcp.WithDescription(`Fetches up-to-date documentation for a specific service from a Terraform provider. You must call 'resolveProviderDocID' first to obtain the exact tfprovider-compatible providerDocID required to use this tool.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform provider documentation using a document ID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("providerDocID", mcp.Required(), mcp.Description("Exact tfprovider-compatible providerDocID, (e.g., '8894603', '8906901') retrieved from 'resolveProviderDocID'")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			providerDocID, err := request.RequireString("providerDocID")
			if err != nil {
				return nil, logAndReturnError(logger, "providerDocID is required", err)
			}
			if providerDocID == "" {
				return nil, logAndReturnError(logger, "providerDocID cannot be empty", nil)
			}

			detailResp, err := sendRegistryCall(registryClient, "GET", fmt.Sprintf("provider-docs/%s", providerDocID), logger, "v2")
			if err != nil {
				return nil, logAndReturnError(logger, fmt.Sprintf("Error fetching provider-docs/%s, please make sure providerDocID is valid and the resolveProviderDocID tool has run prior", providerDocID), err)
			}

			var details ProviderResourceDetails
			if err := json.Unmarshal(detailResp, &details); err != nil {
				return nil, logAndReturnError(logger, fmt.Sprintf("error unmarshalling provider-docs/%s", providerDocID), err)
			}
			return mcp.NewToolResultText(details.Data.Attributes.Content), nil
		}
}

func SearchModules(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("searchModules",
			mcp.WithDescription(`Resolves a Terraform module name to obtain a compatible moduleID for the moduleDetails tool and returns a list of matching Terraform modules. You MUST call this function before 'moduleDetails' to obtain a valid and compatible moduleID. When selecting the best match, consider: - Name similarity to the query - Description relevance - Verification status (verified) - Download counts (popularity) Return the selected moduleID and explain your choice. If there are multiple good matches, mention this but proceed with the most relevant one. If no modules were found, reattempt the search with a new moduleName query.`),
			mcp.WithTitleAnnotation("Search and match Terraform modules based on name and relevance"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("moduleQuery",
				mcp.Required(),
				mcp.Description("The query to search for Terraform modules."),
			),
			mcp.WithNumber("currentOffset",
				mcp.Description("Current offset for pagination"),
				mcp.Min(0),
				mcp.DefaultNumber(0),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			moduleQuery, err := request.RequireString("moduleQuery")
			if err != nil {
				return nil, logAndReturnError(logger, "moduleQuery is required", err)
			}
			currentOffsetValue := request.GetInt("currentOffset", 0)

			var modulesData, errMsg string
			response, err := searchModules(registryClient, moduleQuery, currentOffsetValue, logger)
			if err != nil {
				return nil, logAndReturnError(logger, fmt.Sprintf("no module(s) found for moduleName: %s", moduleQuery), err)
			} else {
				modulesData, err = UnmarshalTFModulePlural(response, moduleQuery)
				if err != nil {
					return nil, logAndReturnError(logger, fmt.Sprintf("unmarshalling modules for moduleName: %s", moduleQuery), err)
				}
			}

			if modulesData == "" {
				errMsg = fmt.Sprintf("getting module(s), none found! query used: %s; error: %s", moduleQuery, errMsg)
				return nil, logAndReturnError(logger, errMsg, nil)
			}
			return mcp.NewToolResultText(modulesData), nil
		}
}

func ModuleDetails(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("moduleDetails",
			mcp.WithDescription(`Fetches up-to-date documentation on how to use a Terraform module. You must call 'searchModules' first to obtain the exact valid and compatible moduleID required to use this tool.`),
			mcp.WithTitleAnnotation("Retrieve documentation for a specific Terraform module"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("moduleID",
				mcp.Required(),
				mcp.Description("Exact valid and compatible moduleID retrieved from searchModules (e.g., 'squareops/terraform-kubernetes-mongodb/mongodb/2.1.1', 'GoogleCloudPlatform/vertex-ai/google/0.2.0')"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			moduleID, err := request.RequireString("moduleID")
			if err != nil {
				return nil, logAndReturnError(logger, "moduleID is required", err)
			}
			if moduleID == "" {
				return nil, logAndReturnError(logger, "moduleID cannot be empty", nil)
			}

			var errMsg string
			response, err := GetModuleDetails(registryClient, moduleID, 0, logger)
			if err != nil {
				errMsg = fmt.Sprintf("no module(s) found for %v,", moduleID)
				return nil, logAndReturnError(logger, errMsg, nil)
			}
			moduleData, err := UnmarshalModuleSingular(response)
			if err != nil {
				return nil, logAndReturnError(logger, "unmarshalling module details", err)
			}
			if moduleData == "" {
				errMsg = fmt.Sprintf("getting module(s), none found! %s please provider a different moduleProvider", errMsg)
				return nil, logAndReturnError(logger, errMsg, nil)
			}
			return mcp.NewToolResultText(moduleData), nil
		}
}

func SearchPolicies(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("searchPolicies",
			mcp.WithDescription(`Searches for Terraform policies based on a query string. This tool returns a list of matching policies, which can be used to retrieve detailed policy information using the 'policyDetails' tool. 
			You MUST call this function before 'policyDetails' to obtain a valid terraformPolicyID.
			When selecting the best match, consider: - Name similarity to the query - Title relevance - Verification status (verified) - Download counts (popularity) Return the selected policyID and explain your choice. 
			If there are multiple good matches, mention this but proceed with the most relevant one. If no policies were found, reattempt the search with a new policyQuery.`),
			mcp.WithTitleAnnotation("Search and match Terraform policies based on name and relevance"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("policyQuery",
				mcp.Required(),
				mcp.Description("The query to search for Terraform modules."),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var terraformPolicies TerraformPolicyList
			pq, err := request.RequireString("policyQuery")
			if err != nil {
				return nil, logAndReturnError(logger, "policyQuery is required", err)
			}
			if pq == "" {
				return nil, logAndReturnError(logger, "policyQuery cannot be empty", nil)
			}

			// static list of 100 is fine for now
			policyResp, err := sendRegistryCall(registryClient, "GET", "policies?page%5Bsize%5D=100&include=latest-version", logger, "v2")
			if err != nil {
				return nil, logAndReturnError(logger, "Failed to fetch policies: registry API did not return a successful response", err)
			}

			err = json.Unmarshal(policyResp, &terraformPolicies)
			if err != nil {
				return nil, logAndReturnError(logger, "Unmarshalling policy list", err)
			}

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("Matching Terraform Policies for query: %s\n\n", pq))
			builder.WriteString("Each result includes:\n- terraformPolicyID: Unique identifier to be used with policyDetails tool\n- Name: Policy name\n- Title: Policy description\n- Downloads: Policy downloads\n---\n\n")

			contentAvailable := false
			for _, policy := range terraformPolicies.Data {
				cs, err := containsSlug(strings.ToLower(policy.Attributes.Title), strings.ToLower(pq))
				cs_pn, err_pn := containsSlug(strings.ToLower(policy.Attributes.Name), strings.ToLower(pq))
				if (cs || cs_pn) && err == nil && err_pn == nil {
					contentAvailable = true
					ID := strings.ReplaceAll(policy.Relationships.LatestVersion.Links.Related, "/v2/", "")
					builder.WriteString(fmt.Sprintf(
						"- terraformPolicyID: %s\n- Name: %s\n- Title: %s\n- Downloads: %d\n---\n",
						ID,
						policy.Attributes.Name,
						policy.Attributes.Title,
						policy.Attributes.Downloads,
					))
				}
			}

			policyData := builder.String()
			if !contentAvailable {
				errMessage := fmt.Sprintf("No policies found matching the query: %s. Try a different policyQuery.", pq)
				return nil, logAndReturnError(logger, errMessage, nil)
			}

			return mcp.NewToolResultText(policyData), nil
		}
}

func PolicyDetails(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("policyDetails",
			mcp.WithDescription(`Fetches up-to-date documentation for a specific policy from the Terraform registry. You must call 'searchPolicies' first to obtain the exact terraformPolicyID required to use this tool.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform policy documentation using a terraformPolicyID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("terraformPolicyID",
				mcp.Required(),
				mcp.Description("Matching terraformPolicyID retrieved from the 'searchPolicies' tool (e.g., 'policies/hashicorp/CIS-Policy-Set-for-AWS-Terraform/1.0.1')"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			terraformPolicyID, err := request.RequireString("terraformPolicyID")
			if err != nil {
				return nil, logAndReturnError(logger, "terraformPolicyID is required and must be a string, it is fetched by running the searchPolicies tool", err)
			}
			if terraformPolicyID == "" {
				return nil, logAndReturnError(logger, "terraformPolicyID cannot be empty, it is fetched by running the searchPolicies tool", nil)
			}

			policyResp, err := sendRegistryCall(registryClient, "GET", fmt.Sprintf("%s?include=policies,policy-modules,policy-library", terraformPolicyID), logger, "v2")
			if err != nil {
				return nil, logAndReturnError(logger, "Failed to fetch policy details: registry API did not return a successful response", err)
			}

			var policyDetails TerraformPolicyDetails
			if err := json.Unmarshal(policyResp, &policyDetails); err != nil {
				return nil, logAndReturnError(logger, fmt.Sprintf("error unmarshalling policy details for %s", terraformPolicyID), err)
			}

			readme := extractReadme(policyDetails.Data.Attributes.Readme)
			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("## Policy details about %s \n\n%s", terraformPolicyID, readme))
			policyList := ""
			moduleList := ""
			for _, policy := range policyDetails.Included {
				if policy.Type == "policy-modules" {
					moduleList += fmt.Sprintf(`
module "%s" {
  source = "https://registry.terraform.io/v2%s/policy-module/%s.sentinel?checksum=sha256:%s"
}
`, policy.Attributes.Name, terraformPolicyID, policy.Attributes.Name, policy.Attributes.Shasum)
				}

				if policy.Type == "policies" {
					policyList += fmt.Sprintf("- POLICY_NAME: %s\n- POLICY_CHECKSUM: sha256:%s\n", policy.Attributes.Name, policy.Attributes.Shasum)
					policyList += "\n---\n"
				}
			}
			builder.WriteString("---\n")
			builder.WriteString("## Usage\n\n")
			builder.WriteString("Generate the content for a HashiCorp Configuration Language (HCL) file named policies.hcl. This file should define a set of policies. For each policy provided, create a distinct policy block using the following template.\n")
			builder.WriteString("\n```hcl\n")
			hclTemplate := fmt.Sprintf(`
%s
policy "<<POLICY_NAME>>" {
  source = "https://registry.terraform.io/v2%s/policy/<<POLICY_NAME>>.sentinel?checksum=<<POLICY_CHECKSUM>>"
  enforcement_level = "advisory"
}
`, moduleList, terraformPolicyID)
			builder.WriteString(hclTemplate)
			builder.WriteString("\n```\n")
			builder.WriteString(fmt.Sprintf("Available policies with SHA for %s are: \n\n", terraformPolicyID))
			builder.WriteString(policyList)

			policyData := builder.String()
			return mcp.NewToolResultText(policyData), nil
		}
}
