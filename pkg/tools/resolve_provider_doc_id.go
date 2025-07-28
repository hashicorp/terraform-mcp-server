// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ResolveProviderDocID creates a tool to get provider details from registry.
func ResolveProviderDocID(registryClient *http.Client, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("resolve_provider_doc_id",
			mcp.WithDescription(`This tool retrieves a list of potential documents based on the service_slug and provider_data_type provided.
You MUST call this function before 'get_provider_docs' to obtain a valid tfprovider-compatible provider_doc_id.
Use the most relevant single word as the search query for service_slug, if unsure about the service_slug, use the provider_name for its value.
When selecting the best match, consider the following:
	- Title similarity to the query
	- Category relevance
Return the selected provider_doc_id and explain your choice.
If there are multiple good matches, mention this but proceed with the most relevant one.`),
			mcp.WithTitleAnnotation("Identify the most relevant provider document ID for a Terraform service"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("provider_name",
				mcp.Required(),
				mcp.Description("The name of the Terraform provider to perform the read or deployment operation"),
			),
			mcp.WithString("provider_namespace",
				mcp.Required(),
				mcp.Description("The publisher of the Terraform provider, typically the name of the company, or their GitHub organization name that created the provider"),
			),
			mcp.WithString("service_slug",
				mcp.Required(),
				mcp.Description("The slug of the service you want to deploy or read using the Terraform provider, prefer using a single word, use underscores for multiple words and if unsure about the service_slug, use the provider_name for its value"),
			),
			mcp.WithString("provider_data_type",
				mcp.Description("The type of the document to retrieve, for general information use 'guides', for deploying resources use 'resources', for reading pre-deployed resources use 'data-sources', for functions use 'functions', and for overview of the provider use 'overview'"),
				mcp.Enum("resources", "data-sources", "functions", "guides", "overview"),
				mcp.DefaultString("resources"),
			),
			mcp.WithString("provider_version",
				mcp.Description("The version of the Terraform provider to retrieve in the format 'x.y.z', or 'latest' to get the latest version")),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return resolveProviderDocIDHandler(registryClient, request, logger)
		},
	}
}

func resolveProviderDocIDHandler(registryClient *http.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// For typical provider and namespace hallucinations
	defaultErrorGuide := "please check the provider name, provider namespace or the provider version you're looking for, perhaps the provider is published under a different namespace or company name"
	providerDetail, err := resolveProviderDetails(request, registryClient, defaultErrorGuide, logger)
	if err != nil {
		return nil, err
	}

	serviceSlug, err := request.RequireString("service_slug")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "service_slug is required", err)
	}
	if serviceSlug == "" {
		return nil, utils.LogAndReturnError(logger, "service_slug cannot be empty", nil)
	}

	providerDataType := request.GetString("provider_data_type", "resources")
	providerDetail.ProviderDataType = providerDataType

	// Check if we need to use v2 API for guides, functions, or overview
	if utils.IsV2ProviderDataType(providerDetail.ProviderDataType) {
		content, err := get_provider_docsV2(registryClient, providerDetail, logger)
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
	response, err := client.SendRegistryCall(registryClient, "GET", uri, logger)
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
	builder.WriteString("For best results, select libraries based on the service_slug match and category of information requested.\n\n---\n\n")

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
		errMessage := fmt.Sprintf(`No documentation found for service_slug %s, provide a more relevant service_slug if unsure, use the provider_name for its value`, serviceSlug)
		return nil, utils.LogAndReturnError(logger, errMessage, err)
	}
	return mcp.NewToolResultText(builder.String()), nil
}

func resolveProviderDetails(request mcp.CallToolRequest, registryClient *http.Client, defaultErrorGuide string, logger *log.Logger) (client.ProviderDetail, error) {
	providerDetail := client.ProviderDetail{}
	providerName := request.GetString("provider_name", "")
	if providerName == "" {
		return providerDetail, fmt.Errorf("provider_name is required and must be a string")
	}

	providerNamespace := request.GetString("provider_namespace", "")
	if providerNamespace == "" {
		logger.Debugf(`Error getting latest provider version in "%s" namespace, trying the hashicorp namespace`, providerNamespace)
		providerNamespace = "hashicorp"
	}

	providerVersion := request.GetString("provider_version", "latest")
	providerDataType := request.GetString("provider_data_type", "resources")

	var err error
	providerVersionValue := ""
	if utils.IsValidProviderVersionFormat(providerVersion) {
		providerVersionValue = providerVersion
	} else {
		providerVersionValue, err = client.GetLatestProviderVersion(registryClient, providerNamespace, providerName, logger)
		if err != nil {
			providerVersionValue = ""
			logger.Debugf("Error getting latest provider version in %s namespace: %v", providerNamespace, err)
		}
	}

	// If the provider version doesn't exist, try the hashicorp namespace
	if providerVersionValue == "" {
		tryProviderNamespace := "hashicorp"
		providerVersionValue, err = client.GetLatestProviderVersion(registryClient, tryProviderNamespace, providerName, logger)
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

// get_provider_docsV2 retrieves a list of documentation items for a specific provider category using v2 API with support for pagination using page numbers
func get_provider_docsV2(registryClient *http.Client, providerDetail client.ProviderDetail, logger *log.Logger) (string, error) {
	providerVersionID, err := client.GetProviderVersionID(registryClient, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion, logger)
	if err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider version ID", err)
	}
	category := providerDetail.ProviderDataType
	if category == "overview" {
		return client.GetProviderOverviewDocs(registryClient, providerVersionID, logger)
	}

	uriPrefix := fmt.Sprintf("provider-docs?filter[provider-version]=%s&filter[category]=%s&filter[language]=hcl",
		providerVersionID, category)

	docs, err := client.SendPaginatedRegistryCall(registryClient, uriPrefix, logger)
	if err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider documentation", err)
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("no %s documentation found for provider version %s", category, providerVersionID)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Documentation (top matches) for %s in Terraform provider %s/%s version: %s\n\n", providerDetail.ProviderDataType, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion))
	builder.WriteString("Each result includes:\n- providerDocID: tfprovider-compatible identifier\n- Title: Service or resource name\n- Category: Type of document\n")
	builder.WriteString("For best results, select libraries based on the service_slug match and category of information requested.\n\n---\n\n")
	for _, doc := range docs {
		builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Title: %s\n- Category: %s\n---\n", doc.ID, doc.Attributes.Title, doc.Attributes.Category))
	}

	return builder.String(), nil
}
