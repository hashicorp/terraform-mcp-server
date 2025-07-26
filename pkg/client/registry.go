// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"
)

func SendRegistryCall(client *http.Client, method string, uri string, logger *log.Logger, callOptions ...string) ([]byte, error) {
	version := "v1"
	if len(callOptions) > 0 {
		version = callOptions[0] // API version will be the first optional arg to this function
	}

	url, err := url.Parse(fmt.Sprintf("https://registry.terraform.io/%s/%s", version, uri))
	if err != nil {
		return nil, fmt.Errorf("error parsing terraform registry URL: %w", err)
	}
	logger.Debugf("Requested URL: %s", url)

	req, err := http.NewRequest(method, url.String(), nil)
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

func SendPaginatedRegistryCall(client *http.Client, uriPrefix string, logger *log.Logger) ([]ProviderDocData, error) {
	var results []ProviderDocData
	page := 1

	for {
		uri := fmt.Sprintf("%s&page[number]=%d", uriPrefix, page)
		resp, err := SendRegistryCall(client, "GET", uri, logger, "v2")
		if err != nil {
			return nil, utils.LogAndReturnError(logger, fmt.Sprintf("calling paginated registry API (page %d)", page), err)
		}

		var wrapper struct {
			Data []ProviderDocData `json:"data"`
		}
		if err := json.Unmarshal(resp, &wrapper); err != nil {
			return nil, utils.LogAndReturnError(logger, fmt.Sprintf("unmarshalling page %d", page), err)
		}

		if len(wrapper.Data) == 0 {
			break
		}

		results = append(results, wrapper.Data...)
		page++
	}

	return results, nil
}
