// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	officialclient "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type SearchProvidersArguments struct {
	ProviderName         string `json:"provider_name"`
	ProviderNamespace    string `json:"provider_namespace,omitempty"`
	ServiceSlug          string `json:"service_slug"`
	ProviderDocumentType string `json:"provider_document_type,omitempty"`
	ProviderVersion      string `json:"provider_version,omitempty"`
}

func SearchProvidersTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "search_providers",
		Description: `This tool retrieves a list of potential documents based on the 'service_slug' and 'provider_document_type' provided.
You MUST call this function before 'get_provider_details' to obtain a valid tfprovider-compatible 'provider_doc_id'.
Use the most relevant single word as the search query for 'service_slug', if unsure about the 'service_slug', use the 'provider_name' for its value.
When selecting the best match, consider the following:
	- Title similarity to the query
	- Category relevance
Return the selected 'provider_doc_id' and explain your choice.
If there are multiple good matches, mention this but proceed with the most relevant one.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Identify the most relevant provider document ID for a Terraform service",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"provider_name": {
					Type:        "string",
					Description: "The name of the Terraform provider to perform the read or deployment operation",
				},
				"provider_namespace": {
					Type:        "string",
					Description: "The publisher of the Terraform provider, typically the name of the company, or their GitHub organization name that created the provider (defaults to hashicorp)",
					Default:     json.RawMessage(`"hashicorp"`),
				},
				"service_slug": {
					Type:        "string",
					Description: "The slug of the service you want to deploy or read using the Terraform provider, prefer using a single word, use underscores for multiple words and if unsure about the service_slug, use the provider_name for its value",
				},
				"provider_document_type": {
					Type:        "string",
					Description: "Document category: resources (default), data-sources, functions, guides, overview, actions, or list-resources",
					Enum:        []any{"resources", "data-sources", "functions", "guides", "overview", "actions", "list-resources"},
					Default:     json.RawMessage(`"resources"`),
				},
				"provider_version": {
					Type:        "string",
					Description: "Provider version in x.y.z format, or latest (default)",
					Default:     json.RawMessage(`"latest"`),
				},
			},
			Required: []string{"provider_name", "service_slug"},
		},
	}
}

func SearchProvidersFunc(ctx context.Context, request *mcp.CallToolRequest, input SearchProvidersArguments) (*mcp.CallToolResult, any, error) {
	// TODO: Replace with structured slog logging
	logger := log.StandardLogger()

	defaultErrorGuide := "please check the provider name, provider namespace or the provider version you're looking for, perhaps the provider is published under a different namespace or company name"

	input.ProviderName = strings.ToLower(strings.TrimSpace(input.ProviderName))
	if input.ProviderName == "" {
		return nil, nil, fmt.Errorf("provider_name is required")
	}

	input.ServiceSlug = strings.ToLower(strings.TrimSpace(input.ServiceSlug))
	if input.ServiceSlug == "" {
		return nil, nil, fmt.Errorf("service_slug cannot be empty")
	}

	input.ProviderNamespace = strings.ToLower(strings.TrimSpace(input.ProviderNamespace))
	input.ProviderVersion = strings.ToLower(strings.TrimSpace(input.ProviderVersion))
	input.ProviderDocumentType = strings.ToLower(strings.TrimSpace(input.ProviderDocumentType))

	httpClient, err := officialclient.GetHttpClient(ctx, officialclient.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	providerDetail, err := resolveProviderDetails(ctx, input, httpClient, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve provider: %v - %s", err, defaultErrorGuide)
	}

	serviceSlug := input.ServiceSlug

	// Check if we need to use v2 API for guides, functions, or overview
	if utils.IsV2ProviderDocumentType(providerDetail.ProviderDocumentType) {
		content, err := providerDetailsV2(ctx, httpClient, providerDetail, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to find %s documentation for provider '%s' in the '%s' namespace - %s",
				providerDetail.ProviderDocumentType, providerDetail.ProviderName, providerDetail.ProviderNamespace, defaultErrorGuide)
		}

		fullContent := fmt.Sprintf("# %s provider docs\n\n%s",
			providerDetail.ProviderName, content)

		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fullContent}}}, nil, nil
	}

	// For resources/data-sources, use the v1 API for better performance (single response)
	uri := path.Join("providers", providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion)
	response, err := client.SendRegistryCall(ctx, httpClient, "GET", uri, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get provider '%s' version '%s' in namespace '%s' - %s",
			providerDetail.ProviderName, providerDetail.ProviderVersion, providerDetail.ProviderNamespace, defaultErrorGuide)
	}

	var providerDocs client.ProviderDocs
	if err := json.Unmarshal(response, &providerDocs); err != nil {
		return nil, nil, fmt.Errorf("failed to parse provider docs: %w", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Documentation (top matches) for %s in Terraform provider %s/%s version: %s\n\n", providerDetail.ProviderDocumentType, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion))
	builder.WriteString("Each result includes:\n- providerDocID: tfprovider-compatible identifier\n- Title: Service or resource name\n- Category: Type of document\n- Description: Brief summary of the document\n")
	builder.WriteString("For best results, select libraries based on the service_slug match and category of information requested.\n\n---\n\n")

	contentAvailable := false
	for _, doc := range providerDocs.Docs {
		if doc.Language == "hcl" && doc.Category == providerDetail.ProviderDocumentType {
			cs, err := utils.ContainsSlug(doc.Slug, serviceSlug)
			cs_pn, err_pn := utils.ContainsSlug(fmt.Sprintf("%s_%s", providerDetail.ProviderName, doc.Slug), serviceSlug)
			if (cs || cs_pn) && err == nil && err_pn == nil {
				contentAvailable = true
				descriptionSnippet, err := getContentSnippet(ctx, httpClient, doc.ID, logger)
				if err != nil {
					logger.WithField("provider_doc_id", doc.ID).WithError(err).Warn("error fetching content snippet")
				}
				builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Title: %s\n- Category: %s\n- Description: %s\n---\n", doc.ID, doc.Title, doc.Category, descriptionSnippet))
			}
		}
	}

	if !contentAvailable {
		return nil, nil, fmt.Errorf("no documentation found for service_slug '%s' - try a more relevant service_slug, or use the provider_name as the value", serviceSlug)
	}

	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: builder.String()}}}, nil, nil
}

func resolveProviderDetails(ctx context.Context, input SearchProvidersArguments, httpClient *http.Client, logger *log.Logger) (client.ProviderDetail, error) {
	providerDetail := client.ProviderDetail{}
	providerName := input.ProviderName
	providerNamespace := input.ProviderNamespace
	providerVersion := input.ProviderVersion
	providerDocumentType := input.ProviderDocumentType

	var err error
	providerVersionValue := ""
	if utils.IsValidProviderVersionFormat(providerVersion) {
		providerVersionValue = providerVersion
	} else {
		providerVersionValue, err = client.GetLatestProviderVersion(ctx, httpClient, providerNamespace, providerName, logger)
		if err != nil {
			providerVersionValue = ""
			logger.WithField("provider_namespace", providerNamespace).WithError(err).Debug("error getting latest provider version")
		}
	}

	// If the provider version doesn't exist, try the hashicorp namespace
	if providerVersionValue == "" {
		tryProviderNamespace := "hashicorp"
		providerVersionValue, err = client.GetLatestProviderVersion(ctx, httpClient, tryProviderNamespace, providerName, logger)
		if err != nil {
			namespaceTried := providerNamespace
			if providerNamespace != tryProviderNamespace {
				namespaceTried = fmt.Sprintf("'%s' or '%s'", providerNamespace, tryProviderNamespace)
			}
			return providerDetail, fmt.Errorf("provider '%s' version '%s' not found in namespace %s", providerName, providerVersion, namespaceTried)
		}
		providerNamespace = tryProviderNamespace
	}

	providerDocumentTypeValue := ""
	if utils.IsValidProviderDocumentType(providerDocumentType) {
		providerDocumentTypeValue = providerDocumentType
	}

	return client.ProviderDetail{
		ProviderName:         providerName,
		ProviderNamespace:    providerNamespace,
		ProviderVersion:      providerVersionValue,
		ProviderDocumentType: providerDocumentTypeValue,
	}, nil
}

// providerDetailsV2 retrieves a list of documentation items for a specific provider category using v2 API
func providerDetailsV2(ctx context.Context, httpClient *http.Client, providerDetail client.ProviderDetail, logger *log.Logger) (string, error) {
	providerVersionID, err := client.GetProviderVersionID(ctx, httpClient, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion, logger)
	if err != nil {
		return "", fmt.Errorf("getting provider version ID: %w", err)
	}

	category := providerDetail.ProviderDocumentType
	if category == "overview" {
		return client.GetProviderOverviewDocs(ctx, httpClient, providerVersionID, logger)
	}

	uriPrefix := fmt.Sprintf("provider-docs?filter[provider-version]=%s&filter[category]=%s&filter[language]=hcl",
		providerVersionID, category)

	docs, err := client.SendPaginatedRegistryCall(ctx, httpClient, uriPrefix, logger)
	if err != nil {
		return "", fmt.Errorf("getting provider documentation: %w", err)
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("no %s documentation found for provider version %s", category, providerVersionID)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Documentation (top matches) for %s in Terraform provider %s/%s version: %s\n\n", providerDetail.ProviderDocumentType, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion))
	builder.WriteString("Each result includes:\n- providerDocID: tfprovider-compatible identifier\n- Title: Service or resource name\n- Category: Type of document\n- Description: Brief summary of the document\n")
	builder.WriteString("For best results, select libraries based on the service_slug match and category of information requested.\n\n---\n\n")
	for _, doc := range docs {
		descriptionSnippet, err := getContentSnippet(ctx, httpClient, doc.ID, logger)
		if err != nil {
			logger.WithField("provider_doc_id", doc.ID).WithError(err).Warn("error fetching content snippet")
		}
		builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Title: %s\n- Category: %s\n- Description: %s\n---\n", doc.ID, doc.Attributes.Title, doc.Attributes.Category, descriptionSnippet))
	}

	return builder.String(), nil
}

func getContentSnippet(ctx context.Context, httpClient *http.Client, docID string, logger *log.Logger) (string, error) {
	docContent, err := client.SendRegistryCall(ctx, httpClient, "GET", fmt.Sprintf("provider-docs/%s", docID), logger, "v2")
	if err != nil {
		return "", fmt.Errorf("fetching provider-docs/%s: %w", docID, err)
	}

	var docDescription client.ProviderResourceDetails
	if err := json.Unmarshal(docContent, &docDescription); err != nil {
		return "", fmt.Errorf("unmarshalling provider-docs/%s: %w", docID, err)
	}

	content := docDescription.Data.Attributes.Content
	desc := ""
	if start := strings.Index(content, "description: |-"); start != -1 {
		substring := ""
		if end := strings.Index(content[start:], "\n---"); end != -1 {
			substring = content[start+len("description: |-") : start+end]
		} else {
			substring = content[start+len("description: |-"):]
		}
		trimmed := strings.TrimSpace(substring)
		desc = strings.ReplaceAll(trimmed, "\n", " ")
	}

	if len(desc) > 300 {
		return desc[:300] + "...", nil
	}
	return desc, nil
}
