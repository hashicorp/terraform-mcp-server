// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/internal/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/internal/utils"
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
				return nil, utils.LogAndReturnError(logger, "serviceSlug is required", err)
			}
			if serviceSlug == "" {
				return nil, utils.LogAndReturnError(logger, "serviceSlug cannot be empty", nil)
			}

			providerDataType := request.GetString("providerDataType", "resources")
			providerDetail.ProviderDataType = providerDataType

			// Check if we need to use v2 API for guides, functions, or overview
			if utils.IsV2ProviderDataType(providerDetail.ProviderDataType) {
				content, err := utils.GetProviderDocsV2(registryClient, providerDetail, logger)
				if err != nil {
					errMessage := fmt.Sprintf(`No %s documentation found for provider '%s' in the '%s' namespace, %s`,
						providerDetail.ProviderDataType, providerDetail.ProviderName, providerDetail.ProviderNamespace, defaultErrorGuide)
					return nil, utils.LogAndReturnError(logger, errMessage, err)
				}

				fullContent := fmt.Sprintf("# %s provider docs\n\n%s",
					providerDetail.ProviderName, content)

				return mcp.NewToolResultText(fullContent), nil
			}

			// For resources/data-sources, use the v1 API for better performance (single response)
			uri := fmt.Sprintf("providers/%s/%s/%s", providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion)
			response, err := utils.SendRegistryCall(registryClient, "GET", uri, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf(`Error getting the "%s" provider, 
					with version "%s" in the %s namespace, %s`, providerDetail.ProviderName, providerDetail.ProviderVersion, providerDetail.ProviderNamespace, defaultErrorGuide), nil)
			}

			var providerDocs client.ProviderDocs
			if err := json.Unmarshal(response, &providerDocs); err != nil {
				return nil, utils.LogAndReturnError(logger, "unmarshalling provider docs", err)
			}

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("Available Documentation (top matches) for %s in Terraform provider %s/%s version: %s\n\n", providerDetail.ProviderDataType, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion))
			builder.WriteString("Each result includes:\n- providerDocID: tfprovider-compatible identifier\n- Title: Service or resource name\n- Category: Type of document\n")
			builder.WriteString("For best results, select libraries based on the serviceSlug match and category of information requested.\n\n---\n\n")

			contentAvailable := false
			for _, doc := range providerDocs.Docs {
				if doc.Language == "hcl" && doc.Category == providerDetail.ProviderDataType {
					cs, err := utils.ContainsSlug(doc.Slug, serviceSlug)
					cs_pn, err_pn := utils.ContainsSlug(fmt.Sprintf("%s_%s", providerDetail.ProviderName, doc.Slug), serviceSlug)
					if (cs || cs_pn) && err == nil && err_pn == nil {
						contentAvailable = true
						builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Title: %s\n- Category: %s\n---\n", doc.ID, doc.Title, doc.Category))
					}
				}
			}

			// Check if the content data is not fulfilled
			if !contentAvailable {
				errMessage := fmt.Sprintf(`No documentation found for serviceSlug %s, provide a more relevant serviceSlug if unsure, use the providerName for its value`, serviceSlug)
				return nil, utils.LogAndReturnError(logger, errMessage, err)
			}
			return mcp.NewToolResultText(builder.String()), nil
		}
}

func resolveProviderDetails(request mcp.CallToolRequest, registryClient *http.Client, defaultErrorGuide string, logger *log.Logger) (client.ProviderDetail, error) {
	providerDetail := client.ProviderDetail{}
	providerName := request.GetString("providerName", "")
	if providerName == "" {
		return providerDetail, fmt.Errorf("providerName is required and must be a string")
	}

	providerNamespace := request.GetString("providerNamespace", "")
	if providerNamespace == "" {
		logger.Debugf(`Error getting latest provider version in "%s" namespace, trying the hashicorp namespace`, providerNamespace)
		providerNamespace = "hashicorp"
	}

	providerVersion := request.GetString("providerVersion", "latest")
	providerDataType := request.GetString("providerDataType", "resources")

	var err error
	providerVersionValue := ""
	if utils.IsValidProviderVersionFormat(providerVersion) {
		providerVersionValue = providerVersion
	} else {
		providerVersionValue, err = utils.GetLatestProviderVersion(registryClient, providerNamespace, providerName, logger)
		if err != nil {
			providerVersionValue = ""
			logger.Debugf("Error getting latest provider version in %s namespace: %v", providerNamespace, err)
		}
	}

	// If the provider version doesn't exist, try the hashicorp namespace
	if providerVersionValue == "" {
		tryProviderNamespace := "hashicorp"
		providerVersionValue, err = utils.GetLatestProviderVersion(registryClient, tryProviderNamespace, providerName, logger)
		if err != nil {
			// Just so we don't print the same namespace twice if they are the same
			if providerNamespace != tryProviderNamespace {
				tryProviderNamespace = fmt.Sprintf(`"%s" or the "%s"`, providerNamespace, tryProviderNamespace)
			}
			return providerDetail, utils.LogAndReturnError(logger, fmt.Sprintf(`Error getting the "%s" provider, 
			with version "%s" in the %s namespace, %s`, providerName, providerVersion, tryProviderNamespace, defaultErrorGuide), nil)
		}
		providerNamespace = tryProviderNamespace // Update the namespace to hashicorp, if successful
	}

	providerDataTypeValue := ""
	if utils.IsValidProviderDataType(providerDataType) {
		providerDataTypeValue = providerDataType
	}

	providerDetail.ProviderName = providerName
	providerDetail.ProviderNamespace = providerNamespace
	providerDetail.ProviderVersion = providerVersionValue
	providerDetail.ProviderDataType = providerDataTypeValue
	return providerDetail, nil
}
