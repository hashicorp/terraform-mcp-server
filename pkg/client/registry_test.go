// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- sendRegistryCall ---
var logger = log.New()

func TestRedactRetryURL(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"archivist object": {
			input: "https://archivist.example/v1/object/secret-object?X-Amz-Signature=secret-sentinel",
			want:  "https://archivist.example/v1/object/___REDACTED___",
		},
		"archivist path": {
			input: "https://archivist.example/private/log?token=secret-sentinel",
			want:  "https://archivist.example/___REDACTED___",
		},
		"signed query": {
			input: "https://storage.example/private/log?X-Goog-Signature=secret-sentinel",
			want:  "https://storage.example/private/log?___REDACTED___",
		},
		"embedded credentials": {
			input: "https://user:password@example.com/private/log",
			want:  "https://redacted@example.com/private/log",
		},
		"ordinary URL": {
			input: "https://registry.terraform.io/v1/providers",
			want:  "https://registry.terraform.io/v1/providers",
		},
		"trailing punctuation": {
			input: "https://archivist.example/v1/object/secret-object?sig=secret-sentinel.",
			want:  "https://archivist.example/v1/object/___REDACTED___.",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, redactRetryURL(test.input))
		})
	}
}

func TestRetryLoggerRedactsCapabilityURLParts(t *testing.T) {
	var output bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&output)
	redactingRetryLogger{logger: testLogger}.Printf(
		"[ERR] GET %s request failed: redirect from %s",
		"https://archivist.example/v1/object/secret-object?X-Amz-Signature=secret-sentinel",
		"http://storage.example/downgrade?X-Amz-Signature=other-secret",
	)

	logged := output.String()
	require.Contains(t, logged, "https://archivist.example/v1/object/___REDACTED___")
	require.Contains(t, logged, "http://storage.example/downgrade?___REDACTED___")
	for _, secret := range []string{"secret-object", "secret-sentinel", "other-secret"} {
		require.NotContains(t, logged, secret)
	}
}

func TestCreateHTTPClientUsesRedactingRetryLogger(t *testing.T) {
	var output bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&output)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	secretURL := server.URL + "/private/log?X-Amz-Signature=secret-sentinel"
	resp, err := CreateHTTPClient(false, testLogger).Get(secretURL)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, output.String(), server.URL+"/private/log?___REDACTED___")
	require.NotContains(t, output.String(), "secret-sentinel")
}

func TestCreateHTTPClientRedactsRetryLogs(t *testing.T) {
	var output bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&output)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-ratelimit-reset", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	secretURL := server.URL + "/private/log?X-Amz-Signature=secret-sentinel"
	resp, err := CreateHTTPClient(false, testLogger).Get(secretURL)
	require.Error(t, err)
	require.Nil(t, resp)

	logged := output.String()
	require.Contains(t, logged, "retrying")
	require.Contains(t, logged, server.URL+"/private/log?___REDACTED___")
	require.NotContains(t, logged, "secret-sentinel")
}

func TestCreateHTTPClientAllowsNilLogger(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := CreateHTTPClient(false, nil).Get(server.URL)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func TestSendRegistryCall(t *testing.T) {
	tests := []struct {
		name             string
		uri              string
		apiVersion       string
		httpMethod       string
		mockStatusCode   int
		mockResponse     string
		expectErrContent string
	}{
		{
			name:             "Success_v1_GET",
			uri:              "providers/hashicorp/aws",
			apiVersion:       "v1",
			httpMethod:       "GET",
			mockStatusCode:   http.StatusOK,
			mockResponse:     `{"data": "success_v1"}`,
			expectErrContent: "",
		},
		{
			name:             "Success_v2_GET_WithQuery",
			uri:              "provider-docs?filter[provider-version]=6221",
			apiVersion:       "v2",
			httpMethod:       "GET",
			mockStatusCode:   http.StatusOK,
			mockResponse:     `{"data": "success_v2"}`,
			expectErrContent: "",
		},
		{
			name:             "404NotFound_v1_GET",
			uri:              "test-uri-v1",
			apiVersion:       "v1",
			httpMethod:       "GET",
			mockStatusCode:   http.StatusNotFound,
			mockResponse:     `{"error": "not_found_v1"}`,
			expectErrContent: "404 Not Found",
		},
		{
			name:             "404NotFound_v2_GET",
			uri:              "test-uri-v2",
			apiVersion:       "v2",
			httpMethod:       "GET",
			mockStatusCode:   http.StatusNotFound,
			mockResponse:     `{"error": "not_found_v2"}`,
			expectErrContent: "404 Not Found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.httpMethod {
					t.Errorf("Handler: Expected method %s, got %s", tc.httpMethod, r.Method)
					http.Error(w, "Bad method", http.StatusBadRequest)
					return
				}

				expectedPathPart := strings.Split(tc.uri, "?")[0]
				var expectedFullPrefix string
				if tc.apiVersion == "v1" {
					expectedFullPrefix = "/v1/" + strings.TrimPrefix(expectedPathPart, "/")
				} else {
					expectedFullPrefix = "/v2/" + strings.TrimPrefix(expectedPathPart, "/")
				}

				if !strings.HasPrefix(r.URL.Path, expectedFullPrefix) {
					t.Errorf("Handler: Expected path prefix %s, got %s", expectedFullPrefix, r.URL.Path)
					http.Error(w, "Bad path", http.StatusBadRequest)
					return
				}

				if tc.name == "Success_v2_GET_WithQuery" {
					if r.URL.Query().Get("filter[provider-version]") != "6221" {
						t.Errorf("Handler: Expected query 'filter[provider-version]=6221', got query: '%s'", r.URL.RawQuery)
						http.Error(w, "Bad query params", http.StatusBadRequest)
						return
					}
				}

				w.WriteHeader(tc.mockStatusCode)
				fmt.Fprint(w, tc.mockResponse)
			}))
			defer server.Close()

			_, err := SendRegistryCall(context.Background(), server.Client(), tc.httpMethod, tc.uri, logger, tc.apiVersion, server.URL)

			if tc.expectErrContent == "" {
				require.NoError(t, err, "TestSendRegistryCall (%s)", tc.name)
			} else {
				require.Error(t, err, "TestSendRegistryCall (%s): expected an error but got nil", tc.name)
				assert.Contains(t, err.Error(), tc.expectErrContent, "TestSendRegistryCall (%s): error message mismatch", tc.name)
			}
		})
	}
}
