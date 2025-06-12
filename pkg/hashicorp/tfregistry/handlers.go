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

			serviceSlug, ok := request.Params.Arguments["serviceSlug"].(string)
			if !ok || serviceSlug == "" {
				return nil, logAndReturnError(logger, "serviceSlug is required and must be a string", nil)
			}

			providerDataType, ok := request.Params.Arguments["providerDataType"].(string)
			if !ok || providerDataType == "" {
				providerDataType = "resources"
			}
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
						contentSnippet, err := getContentSnippet(registryClient, doc.ID, logger)
						if err != nil {
							return nil, logAndReturnError(logger, fmt.Sprintf("error fetching provider-docs/%s within ResolveProviderDocID", doc.ID), err)
						}
						builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Content: %s\n---\n", doc.ID, contentSnippet))
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

func getContentSnippet(registryClient *http.Client, docID string, logger *log.Logger) (string, error) {

	docContent, err := sendRegistryCall(registryClient, "GET", fmt.Sprintf("provider-docs/%s", docID), logger, "v2")
	if err != nil {
		return "", logAndReturnError(logger, fmt.Sprintf("error fetching provider-docs/%s within getContentSnippet", docID), err)
	}
	var docDescription ProviderResourceDetails
	if err := json.Unmarshal(docContent, &docDescription); err != nil {
		return "", logAndReturnError(logger, fmt.Sprintf("error unmarshalling provider-docs/%s within getContentSnippet", docID), err)
	}
	contentSnippet := strings.SplitN(docDescription.Data.Attributes.Content, "\n", 10)
	return strings.Join(contentSnippet[:9], "\n"), nil
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
			providerDocID, ok := request.Params.Arguments["providerDocID"].(string)
			if !ok || providerDocID == "" {
				return nil, fmt.Errorf("providerDocID is required and must be a string")
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
			moduleQuery := request.Params.Arguments["moduleQuery"]
			currentOffset := request.Params.Arguments["currentOffset"]
			currentOffsetValue := 0
			if val, ok := currentOffset.(float64); ok {
				currentOffsetValue = int(val)
			}

			if mq, ok := moduleQuery.(string); !ok {
				return nil, logAndReturnError(logger, "error finding the module name;", nil)
			} else {
				var modulesData, errMsg string
				response, err := searchModules(registryClient, mq, currentOffsetValue, logger)
				if err != nil {
					return nil, logAndReturnError(logger, fmt.Sprintf("no module(s) found for moduleName: %s", mq), err)
				} else {
					modulesData, err = UnmarshalTFModulePlural(response, mq)
					if err != nil {
						return nil, logAndReturnError(logger, fmt.Sprintf("unmarshalling modules for moduleName: %s", mq), err)
					}
				}

				if modulesData == "" {
					errMsg = fmt.Sprintf("getting module(s), none found! query used: %s; error: %s", mq, errMsg)
					return nil, logAndReturnError(logger, errMsg, nil)
				}
				return mcp.NewToolResultText(modulesData), nil
			}
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
			moduleID := request.Params.Arguments["moduleID"]

			if mn, ok := moduleID.(string); !ok || mn == "" {
				return nil, logAndReturnError(logger, "moduleID is required and must be a valid string. It represents the ID of the module to retrieve detailed information about", nil)
			} else {
				var errMsg string
				response, err := GetModuleDetails(registryClient, mn, 0, logger)
				if err != nil {
					errMsg = fmt.Sprintf("no module(s) found for %v,", mn)
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
}
