// Copyright IBM Corp. 2025
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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/hashicorp/terraform-mcp-server/version"
	log "github.com/sirupsen/logrus"
)

const DefaultPublicRegistryURL = "https://registry.terraform.io"

var retryLogURLPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`)

// Signed query parameters contain temporary authorization material and must not
// be written to retry logs.
var signedURLQueryKeys = map[string]struct{}{
	"signature":        {},
	"sig":              {},
	"x-amz-signature":  {},
	"x-goog-signature": {},
}

type redactingRetryLogger struct {
	logger *log.Logger
}

func (l redactingRetryLogger) Printf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.logger.Print(retryLogURLPattern.ReplaceAllStringFunc(message, redactRetryURL))
}

// redactRetryURL preserves useful URL context while removing sensitive
// Archivist object paths, signed query values, and embedded credentials.
func redactRetryURL(raw string) string {
	trailing := ""
	for len(raw) > 0 && strings.ContainsRune(".,;:!?)]}", rune(raw[len(raw)-1])) {
		trailing = string(raw[len(raw)-1]) + trailing
		raw = raw[:len(raw)-1]
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[redacted-url]" + trailing
	}

	if u.User != nil {
		u.User = url.User("redacted")
	}

	hostIsArchivist := strings.Contains(strings.ToLower(u.Hostname()), "archivist")
	objectPath := "/v1/object/"
	if idx := strings.Index(strings.ToLower(u.Path), objectPath); idx >= 0 {
		u.Path = u.Path[:idx+len(objectPath)] + "___REDACTED___"
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""
		return u.String() + trailing
	}

	if hostIsArchivist {
		u.Path = "/___REDACTED___"
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""
		return u.String() + trailing
	}

	for parameter := range strings.SplitSeq(u.RawQuery, "&") {
		key := parameter
		if idx := strings.IndexByte(key, '='); idx >= 0 {
			key = key[:idx]
		}
		key, err = url.QueryUnescape(key)
		if err != nil {
			continue
		}
		if _, ok := signedURLQueryKeys[strings.ToLower(key)]; ok {
			u.RawQuery = "___REDACTED___"
			u.Fragment = ""
			return u.String() + trailing
		}
	}

	return u.String() + trailing
}

// CreateHTTPClient initializes a retryable HTTP client
func CreateHTTPClient(insecureSkipVerify bool, logger *log.Logger) *http.Client {
	retryClient := retryablehttp.NewClient()
	if logger == nil {
		// Assign an untyped nil interface: a typed nil *logrus.Logger would
		// still satisfy retryablehttp.Logger and panic on the first request.
		retryClient.Logger = nil
	} else {
		retryClient.Logger = redactingRetryLogger{logger: logger}
	}

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

func SendRegistryCall(ctx context.Context, client *http.Client, method string, uri string, logger *log.Logger, callOptions ...string) ([]byte, error) {
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
	// Set X-Forwarded-For if client IP is in context
	if clientIP, ok := ctx.Value(contextKey(ClientIPKey)).(string); ok && clientIP != "" {
		req.Header.Set("X-Forwarded-For", clientIP)
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

func SendPaginatedRegistryCall(ctx context.Context, client *http.Client, uriPrefix string, logger *log.Logger) ([]ProviderDocData, error) {
	var results []ProviderDocData
	page := 1

	for {
		uri := fmt.Sprintf("%s&page[number]=%d", uriPrefix, page)
		resp, err := SendRegistryCall(ctx, client, "GET", uri, logger, "v2")
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
