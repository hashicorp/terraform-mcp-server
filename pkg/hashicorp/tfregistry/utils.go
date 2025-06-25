// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfregistry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

const PROVIDER_BASE_PATH = "registry://providers"

func GetProviderList(providerClient *http.Client, providerType string, logger *log.Logger) ([]map[string]string, error) {
	uri := fmt.Sprintf("providers?filter[tier]=%s", providerType)
	jsonData, err := sendRegistryCall(providerClient, "GET", uri, logger, "v2")
	if err != nil {
		return nil, logAndReturnError(logger, fmt.Sprintf("%s provider API request", providerType), err)
	}

	var providerListJson ProviderList
	if err := json.Unmarshal(jsonData, &providerListJson); err != nil {
		return nil, logAndReturnError(logger, fmt.Sprintf("%s providers request unmarshalling", providerType), err)
	}

	providerDetails := make([]map[string]string, len(providerListJson.Data))

	for i, provider := range providerListJson.Data {
		providerDetails[i] = map[string]string{
			"name":        provider.Attributes.Name,
			"namespace":   provider.Attributes.Namespace,
			"description": provider.Attributes.Namespace,
			"downloads":   provider.Attributes.Namespace,
		}
	}
	return providerDetails, nil
}

// Every provider version has a unique ID, which is used to identify the provider version in the registry and its specific documentation
// https://registry.terraform.io/v2/providers/hashicorp/aws?include=provider-versions
func GetProviderVersionID(registryClient *http.Client, namespace string, name string, version string, logger *log.Logger) (string, error) {
	uri := fmt.Sprintf("providers/%s/%s?include=provider-versions", namespace, name)
	response, err := sendRegistryCall(registryClient, "GET", uri, logger, "v2")
	if err != nil {
		return "", logAndReturnError(logger, "provider version ID request", err)
	}
	var providerVersionList ProviderVersionList
	if err := json.Unmarshal(response, &providerVersionList); err != nil {
		return "", logAndReturnError(logger, "provider version ID request unmarshalling", err)
	}
	for _, providerVersion := range providerVersionList.Included {
		if providerVersion.Attributes.Version == version {
			return providerVersion.ID, nil
		}
	}
	return "", fmt.Errorf("provider version %s not found", version)
}

func GetProviderOverviewDocs(registryClient *http.Client, providerVersionID string, logger *log.Logger) (string, error) {
	// https://registry.terraform.io/v2/provider-docs?filter[provider-version]=21818&filter[category]=overview&filter[slug]=index
	uri := fmt.Sprintf("provider-docs?filter[provider-version]=%s&filter[category]=overview&filter[slug]=index", providerVersionID)
	response, err := sendRegistryCall(registryClient, "GET", uri, logger, "v2")
	if err != nil {
		return "", logAndReturnError(logger, "getting provider docs overview", err)
	}
	var providerOverview ProviderOverviewStruct
	if err := json.Unmarshal(response, &providerOverview); err != nil {
		return "", logAndReturnError(logger, "getting provider docs request unmarshalling", err)
	}

	resourceContent := ""
	for _, providerOverviewPage := range providerOverview.Data {
		resourceContentNew, err := GetProviderResourceDocs(registryClient, providerOverviewPage.ID, logger)
		resourceContent += resourceContentNew
		if err != nil {
			return "", logAndReturnError(logger, "getting provider resource docs looping", err)
		}
	}

	return resourceContent, nil
}

func GetProviderResourceDocs(registryClient *http.Client, providerDocsID string, logger *log.Logger) (string, error) {
	// https://registry.terraform.io/v2/provider-docs/8862001
	uri := fmt.Sprintf("provider-docs/%s", providerDocsID)
	response, err := sendRegistryCall(registryClient, "GET", uri, logger, "v2")
	if err != nil {
		return "", logAndReturnError(logger, "Error getting provider resource docs ", err)
	}
	var providerServiceDetails ProviderResourceDetails
	if err := json.Unmarshal(response, &providerServiceDetails); err != nil {
		return "", logAndReturnError(logger, "Error unmarshalling provider resource docs", err)
	}
	return providerServiceDetails.Data.Attributes.Content, nil
}

func ExtractProviderNameAndVersion(uri string) (string, string, string) {
	uri = strings.TrimPrefix(uri, fmt.Sprintf("%s/", PROVIDER_BASE_PATH))
	parts := strings.Split(uri, "/")
	return parts[0], parts[2], parts[4]
}

func ConstructProviderVersionURI(providerNamespace interface{}, providerName string, providerVersion interface{}) string {
	return fmt.Sprintf("%s/%s/providers/%s/versions/%s", PROVIDER_BASE_PATH, providerNamespace, providerName, providerVersion)
}

func GetLatestProviderVersion(providerClient *http.Client, providerNamespace, providerName interface{}, logger *log.Logger) (string, error) {
	uri := fmt.Sprintf("providers/%s/%s", providerNamespace, providerName)
	jsonData, err := sendRegistryCall(providerClient, "GET", uri, logger, "v1")
	if err != nil {
		return "", logAndReturnError(logger, "latest provider version API request", err)
	}

	var providerVersionLatest ProviderVersionLatest
	if err := json.Unmarshal(jsonData, &providerVersionLatest); err != nil {
		return "", logAndReturnError(logger, "provider versions request unmarshalling", err)
	}

	logger.Debugf("Fetched latest provider version: %s", providerVersionLatest.Version)

	return providerVersionLatest.Version, nil
}

// GetProviderResourceDetailsV2 fetches the provider resource details using v2 API with support for pagination using page numbers
func GetProviderResourceDetailsV2(client *http.Client, providerDetail ProviderDetail, serviceSlug string, logger *log.Logger) (string, error) {
	providerVersionID, err := GetProviderVersionID(client, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion, logger)
	if err != nil {
		return "", logAndReturnError(logger, "getting provider version ID", err)
	}

	uriPrefix := fmt.Sprintf("provider-docs?filter[provider-version]=%s&filter[category]=%s&filter[slug]=%s&filter[language]=hcl",
		providerVersionID, providerDetail.ProviderDataType, serviceSlug)
	docs, err := sendPaginatedRegistryCall[ProviderDocData](client, uriPrefix, logger)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for _, doc := range docs {
		detailResp, err := sendRegistryCall(client, "GET", fmt.Sprintf("provider-docs/%s", doc.ID), logger, "v2")
		if err != nil {
			logger.Errorf("Error fetching provider-docs/%s: %v", doc.ID, err)
			continue
		}

		var details ProviderResourceDetails
		if err := json.Unmarshal(detailResp, &details); err != nil {
			logger.Errorf("Error unmarshalling provider-docs/%s: %v", doc.ID, err)
			continue
		}
		builder.WriteString(details.Data.Attributes.Content)
	}

	return builder.String(), nil
}

// containsSlug checks if the sourceName string contains the slug string anywhere within it.
// It safely handles potential regex metacharacters in the slug.
// TODO: include a unit test for this
func containsSlug(sourceName string, slug string) (bool, error) {
	// Use regexp.QuoteMeta to escape any special regex characters in the slug.
	// This ensures the slug is treated as a literal string in the pattern.
	escapedSlug := regexp.QuoteMeta(slug)

	// Construct the regex pattern dynamically: ".*" + escapedSlug + ".*"
	// This pattern means "match any characters, then the escaped slug, then any characters".
	pattern := ".*" + escapedSlug + ".*"

	// regexp.MatchString compiles and runs the regex against the sourceName.
	// It returns true if a match is found, false otherwise.
	// It also returns an error if the pattern is invalid (unlikely here due to QuoteMeta).
	matched, err := regexp.MatchString(pattern, sourceName)
	if err != nil {
		fmt.Printf("Error compiling or matching regex pattern '%s': %v\n", pattern, err)
		return false, err // Propagate the error
	}

	return matched, nil
}

// isValidProviderVersionFormat checks if the provider version format is valid.
func isValidProviderVersionFormat(version string) bool {
	// Example regex for semantic versioning (e.g., "1.0.0", "1.0.0-beta").
	semverRegex := `^v?(\d+\.\d+\.\d+(-[a-zA-Z0-9]+)?)$`
	matched, _ := regexp.MatchString(semverRegex, version)
	return matched
}

func isValidProviderDataType(providerDataType string) bool {
	validTypes := []string{"resources", "data-sources", "functions", "guides", "overview"}
	return slices.Contains(validTypes, providerDataType)
}

func resolveProviderDetails(request mcp.CallToolRequest, registryClient *http.Client, defaultErrorGuide string, logger *log.Logger) (ProviderDetail, error) {
	providerDetail := ProviderDetail{}
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
	if isValidProviderVersionFormat(providerVersion) {
		providerVersionValue = providerVersion
	} else {
		providerVersionValue, err = GetLatestProviderVersion(registryClient, providerNamespace, providerName, logger)
		if err != nil {
			providerVersionValue = ""
			logger.Debugf("Error getting latest provider version in %s namespace: %v", providerNamespace, err)
		}
	}

	// If the provider version doesn't exist, try the hashicorp namespace
	if providerVersionValue == "" {
		tryProviderNamespace := "hashicorp"
		providerVersionValue, err = GetLatestProviderVersion(registryClient, tryProviderNamespace, providerName, logger)
		if err != nil {
			// Just so we don't print the same namespace twice if they are the same
			if providerNamespace != tryProviderNamespace {
				tryProviderNamespace = fmt.Sprintf(`"%s" or the "%s"`, providerNamespace, tryProviderNamespace)
			}
			return providerDetail, logAndReturnError(logger, fmt.Sprintf(`Error getting the "%s" provider, 
			with version "%s" in the %s namespace, %s`, providerName, providerVersion, tryProviderNamespace, defaultErrorGuide), nil)
		}
		providerNamespace = tryProviderNamespace // Update the namespace to hashicorp, if successful
	}

	providerDataTypeValue := ""
	if isValidProviderDataType(providerDataType) {
		providerDataTypeValue = providerDataType
	}

	providerDetail.ProviderName = providerName
	providerDetail.ProviderNamespace = providerNamespace
	providerDetail.ProviderVersion = providerVersionValue
	providerDetail.ProviderDataType = providerDataTypeValue
	return providerDetail, nil
}

const MODULE_BASE_PATH = "registry://modules"

func searchModules(providerClient *http.Client, moduleQuery string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleQuery != "" {
		uri = fmt.Sprintf("%s/search?q='%s'&offset=%v", uri, url.PathEscape(moduleQuery), currentOffset)
	} else {
		uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	}

	response, err := sendRegistryCall(providerClient, "GET", uri, logger)
	if err != nil {
		// We shouldn't log the error here because we might hit a namespace that doesn't exist, it's better to let the caller handle it.
		return nil, fmt.Errorf("getting module(s) for: %v, call error: %v", moduleQuery, err)
	}

	// Return the filtered JSON as a string
	return response, nil
}

func GetModuleDetails(providerClient *http.Client, moduleID string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleID != "" {
		uri = fmt.Sprintf("modules/%s", moduleID)
	}

	uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	response, err := sendRegistryCall(providerClient, "GET", uri, logger)
	if err != nil {
		// We shouldn't log the error here because we might hit a namespace that doesn't exist, it's better to let the caller handle it.
		return nil, fmt.Errorf("getting module(s) for: %v, please provide a different provider name like aws, azurerm or google etc", moduleID)
	}

	// Return the filtered JSON as a string
	return response, nil
}

func UnmarshalTFModulePlural(response []byte, moduleQuery string) (string, error) {
	// Get the list of modules
	var terraformModules TerraformModules
	err := json.Unmarshal(response, &terraformModules)
	if err != nil {
		return "", logAndReturnError(nil, "unmarshalling modules", err)
	}

	if len(terraformModules.Data) == 0 {
		return "", fmt.Errorf("no modules found for query: %s", moduleQuery)
	}

	// Sort by most downloaded
	sort.Slice(terraformModules.Data, func(i, j int) bool {
		return terraformModules.Data[i].Downloads > terraformModules.Data[j].Downloads
	})

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Terraform Modules (top matches) for %s\n\n Each result includes:\n", moduleQuery))
	builder.WriteString("- moduleID: The module ID (format: namespace/name/provider-name/module-version)\n")
	builder.WriteString("- Name: The name of the module\n")
	builder.WriteString("- Description: A short description of the module\n")
	builder.WriteString("- Downloads: The total number of times the module has been downloaded\n")
	builder.WriteString("- Verified: Verification status of the module\n")
	builder.WriteString("- Published: The date and time when the module was published\n")
	builder.WriteString("\n\n---\n\n")
	for _, module := range terraformModules.Data {
		builder.WriteString(fmt.Sprintf("- moduleID: %s\n", module.ID))
		builder.WriteString(fmt.Sprintf("- Name: %s\n", module.Name))
		builder.WriteString(fmt.Sprintf("- Description: %s\n", module.Description))
		builder.WriteString(fmt.Sprintf("- Downloads: %d\n", module.Downloads))
		builder.WriteString(fmt.Sprintf("- Verified: %t\n", module.Verified))
		builder.WriteString(fmt.Sprintf("- Published: %s\n", module.PublishedAt))
		builder.WriteString("---\n\n")
	}
	return builder.String(), nil
}

func UnmarshalModuleSingular(response []byte) (string, error) {
	// Handles one module
	var terraformModules TerraformModuleVersionDetails
	err := json.Unmarshal(response, &terraformModules)
	if err != nil {
		return "", logAndReturnError(nil, "unmarshalling module details", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# %s/%s/%s\n\n", MODULE_BASE_PATH, terraformModules.Namespace, terraformModules.Name))
	builder.WriteString(fmt.Sprintf("**Description:** %s\n\n", terraformModules.Description))
	builder.WriteString(fmt.Sprintf("**Module Version:** %s\n\n", terraformModules.Version))
	builder.WriteString(fmt.Sprintf("**Namespace:** %s\n\n", terraformModules.Namespace))
	builder.WriteString(fmt.Sprintf("**Source:** %s\n\n", terraformModules.Source))

	// Format Inputs
	if len(terraformModules.Root.Inputs) > 0 {
		builder.WriteString("### Inputs\n\n")
		builder.WriteString("| Name | Type | Description | Default | Required |\n")
		builder.WriteString("|---|---|---|---|---|\n")
		for _, input := range terraformModules.Root.Inputs {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | `%v` | %t |\n",
				input.Name,
				input.Type,
				input.Description, // Consider cleaning potential newlines/markdown
				input.Default,
				input.Required,
			))
		}
		builder.WriteString("\n")
	}

	// Format Outputs
	if len(terraformModules.Root.Outputs) > 0 {
		builder.WriteString("### Outputs\n\n")
		builder.WriteString("| Name | Description |\n")
		builder.WriteString("|---|---|\n")
		for _, output := range terraformModules.Root.Outputs {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n",
				output.Name,
				output.Description, // Consider cleaning potential newlines/markdown
			))
		}
		builder.WriteString("\n")
	}

	// Format Provider Dependencies
	if len(terraformModules.Root.ProviderDependencies) > 0 {
		builder.WriteString("### Provider Dependencies\n\n")
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|---|---|---|---|\n")
		for _, dep := range terraformModules.Root.ProviderDependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				dep.Name,
				dep.Namespace,
				dep.Source,
				dep.Version,
			))
		}
		builder.WriteString("\n")
	}

	// Format Examples
	if len(terraformModules.Examples) > 0 {
		builder.WriteString("### Examples\n\n")
		for _, example := range terraformModules.Examples {
			builder.WriteString(fmt.Sprintf("#### %s\n\n", example.Name))
			// Optionally, include more details from example if needed, like inputs/outputs
			// For now, just listing the name.
			if example.Readme != "" {
				builder.WriteString("**Readme:**\n\n")
				// Append readme content, potentially needs markdown escaping/sanitization depending on source
				builder.WriteString(example.Readme)
				builder.WriteString("\n\n")
			}
		}
		builder.WriteString("\n")
	}

	content := builder.String()
	return content, nil
}

func sendRegistryCall(client *http.Client, method string, uri string, logger *log.Logger, callOptions ...string) ([]byte, error) {
	version := "v1"
	if len(callOptions) > 0 {
		version = callOptions[0] // API version will be the first optional arg to this function
	}

	url := fmt.Sprintf("https://registry.terraform.io/%s/%s", version, uri)
	logger.Debugf("Requested URL: %s", url)

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error: %s", "404 Not Found")
	}

	defer resp.Body.Close()
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	logger.Debugf("Response status: %s", resp.Status)
	logger.Tracef("Response body: %s", string(body))
	return body, nil
}

func sendPaginatedRegistryCall[T any](client *http.Client, uriPrefix string, logger *log.Logger) ([]T, error) {
	var results []T
	page := 1

	for {
		uri := fmt.Sprintf("%s&page[number]=%d", uriPrefix, page)
		resp, err := sendRegistryCall(client, "GET", uri, logger, "v2")
		if err != nil {
			return nil, logAndReturnError(logger, fmt.Sprintf("calling paginated registry API (page %d)", page), err)
		}

		var wrapper struct {
			Data []T `json:"data"`
		}
		if err := json.Unmarshal(resp, &wrapper); err != nil {
			return nil, logAndReturnError(logger, fmt.Sprintf("unmarshalling page %d", page), err)
		}

		if len(wrapper.Data) == 0 {
			break
		}

		results = append(results, wrapper.Data...)
		page++
	}

	return results, nil
}

func logAndReturnError(logger *log.Logger, context string, err error) error {
	err = fmt.Errorf("%s, %w", context, err)
	if logger != nil {
		logger.Errorf("Error in %s, %v", context, err)
	}
	return err
}

// GetProviderDocsV2 retrieves a list of documentation items for a specific provider category using v2 API with support for pagination using page numbers
func GetProviderDocsV2(client *http.Client, providerDetail ProviderDetail, logger *log.Logger) (string, error) {
	providerVersionID, err := GetProviderVersionID(client, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion, logger)
	if err != nil {
		return "", logAndReturnError(logger, "getting provider version ID", err)
	}
	category := providerDetail.ProviderDataType
	if category == "overview" {
		return GetProviderOverviewDocs(client, providerVersionID, logger)
	}

	uriPrefix := fmt.Sprintf("provider-docs?filter[provider-version]=%s&filter[category]=%s&filter[language]=hcl",
		providerVersionID, category)

	docs, err := sendPaginatedRegistryCall[ProviderDocData](client, uriPrefix, logger)
	if err != nil {
		return "", err
	}

	if len(docs) == 0 {
		return "", fmt.Errorf("no %s documentation found for provider version %s", category, providerVersionID)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Documentation (top matches) for %s in Terraform provider %s/%s version: %s\n\n", providerDetail.ProviderDataType, providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion))
	builder.WriteString("Each result includes:\n- providerDocID: tfprovider-compatible identifier\n- Title: Service or resource name\n- Category: Type of document\n")
	builder.WriteString("For best results, select libraries based on the serviceSlug match and category of information requested.\n\n---\n\n")
	for _, doc := range docs {
		builder.WriteString(fmt.Sprintf("- providerDocID: %s\n- Title: %s\n- Category: %s\n---\n", doc.ID, doc.Attributes.Title, doc.Attributes.Category))
	}

	return builder.String(), nil
}

func isV2ProviderDataType(dataType string) bool {
	v2Categories := []string{"guides", "functions", "overview"}
	return slices.Contains(v2Categories, dataType)
}
