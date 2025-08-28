// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/hashicorp/terraform-mcp-server/version"
	log "github.com/sirupsen/logrus"
)

const DefaultPublicRegistryURL = "https://registry.terraform.io"

// createHTTPClient initializes a retryable HTTP client
func createHTTPClient(insecureSkipVerify bool, logger *log.Logger) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = logger

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
	}
	transport.Proxy = http.ProxyFromEnvironment

	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.HTTPClient.Timeout = 10 * time.Second
	retryClient.HTTPClient.Transport = transport
	retryClient.RetryMax = 3

	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			resetAfterInt, err := strconv.ParseInt(resetAfter, 10, 64)
			if err != nil {
				return 0
			}
			resetAfterTime := time.Unix(resetAfterInt, 0)
			return time.Until(resetAfterTime)
		}
		return 0
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			return resetAfter != "", nil
		}
		return false, nil
	}

	return retryClient.StandardClient()
}

func SendRegistryCall(client *http.Client, method string, uri string, logger *log.Logger, callOptions ...string) ([]byte, error) {
	ver := "v1"
	if len(callOptions) > 0 {
		ver = callOptions[0] // API version will be the first optional arg to this function
	}

	url, err := url.Parse(fmt.Sprintf("%s/%s/%s", DefaultPublicRegistryURL, ver, uri))
	if err != nil {
		return nil, fmt.Errorf("error parsing terraform registry URL: %w", err)
	}
	logger.Debugf("Requested URL: %s", url)

	req, err := http.NewRequest(method, url.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("terraform-mcp-server/%s", version.GetHumanVersion()))

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
