// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
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

			_, err := SendRegistryCall(server.Client(), tc.httpMethod, tc.uri, logger, tc.apiVersion, server.URL)

			if tc.expectErrContent == "" {
				require.NoError(t, err, "TestSendRegistryCall (%s)", tc.name)
			} else {
				require.Error(t, err, "TestSendRegistryCall (%s): expected an error but got nil", tc.name)
				assert.Contains(t, err.Error(), tc.expectErrContent, "TestSendRegistryCall (%s): error message mismatch", tc.name)
			}
		})
	}
}
