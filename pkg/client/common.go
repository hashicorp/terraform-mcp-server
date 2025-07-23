// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"
)

func GetLatestProviderVersion(providerClient *http.Client, providerNamespace, providerName interface{}, logger *log.Logger) (string, error) {
	uri := fmt.Sprintf("providers/%s/%s", providerNamespace, providerName)
	jsonData, err := SendRegistryCall(providerClient, "GET", uri, logger, "v1")
	if err != nil {
		return "", utils.LogAndReturnError(logger, "latest provider version API request", err)
	}

	var providerVersionLatest ProviderVersionLatest
	if err := json.Unmarshal(jsonData, &providerVersionLatest); err != nil {
		return "", utils.LogAndReturnError(logger, "provider versions request unmarshalling", err)
	}

	logger.Debugf("Fetched latest provider version: %s", providerVersionLatest.Version)
	return providerVersionLatest.Version, nil
}

// Every provider version has a unique ID, which is used to identify the provider version in the registry and its specific documentation
// https://registry.terraform.io/v2/providers/hashicorp/aws?include=provider-versions
func GetProviderVersionID(registryClient *http.Client, namespace string, name string, version string, logger *log.Logger) (string, error) {
	uri := fmt.Sprintf("providers/%s/%s?include=provider-versions", namespace, name)
	response, err := SendRegistryCall(registryClient, "GET", uri, logger, "v2")
	if err != nil {
		return "", utils.LogAndReturnError(logger, "provider version ID request", err)
	}
	var providerVersionList ProviderVersionList
	if err := json.Unmarshal(response, &providerVersionList); err != nil {
		return "", utils.LogAndReturnError(logger, "provider version ID request unmarshalling", err)
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
	response, err := SendRegistryCall(registryClient, "GET", uri, logger, "v2")
	if err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider docs overview", err)
	}
	var providerOverview ProviderOverviewStruct
	if err := json.Unmarshal(response, &providerOverview); err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider docs request unmarshalling", err)
	}

	resourceContent := ""
	for _, providerOverviewPage := range providerOverview.Data {
		resourceContentNew, err := GetProviderResourceDocs(registryClient, providerOverviewPage.ID, logger)
		resourceContent += resourceContentNew
		if err != nil {
			return "", utils.LogAndReturnError(logger, "getting provider resource docs looping", err)
		}
	}

	return resourceContent, nil
}

func GetProviderResourceDocs(registryClient *http.Client, providerDocsID string, logger *log.Logger) (string, error) {
	// https://registry.terraform.io/v2/provider-docs/8862001
	uri := fmt.Sprintf("provider-docs/%s", providerDocsID)
	response, err := SendRegistryCall(registryClient, "GET", uri, logger, "v2")
	if err != nil {
		return "", utils.LogAndReturnError(logger, "Error getting provider resource docs ", err)
	}
	var providerServiceDetails ProviderResourceDetails
	if err := json.Unmarshal(response, &providerServiceDetails); err != nil {
		return "", utils.LogAndReturnError(logger, "Error unmarshalling provider resource docs", err)
	}
	return providerServiceDetails.Data.Attributes.Content, nil
}
