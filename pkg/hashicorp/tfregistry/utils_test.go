// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !integration

package tfregistry

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

			_, err := sendRegistryCall(server.Client(), tc.httpMethod, tc.uri, logger, tc.apiVersion, server.URL)

			if tc.expectErrContent == "" {
				require.NoError(t, err, "TestSendRegistryCall (%s)", tc.name)
			} else {
				require.Error(t, err, "TestSendRegistryCall (%s): expected an error but got nil", tc.name)
				assert.Contains(t, err.Error(), tc.expectErrContent, "TestSendRegistryCall (%s): error message mismatch", tc.name)
			}
		})
	}
}

// --- UnmarshalTFModulePlural ---

func TestUnmarshalTFModulePlural(t *testing.T) {
	tests := []struct {
		name               string
		responseBody       []byte
		query              string
		expectErrSubstring string
	}{
		{
			name:               "NoModulesFound",
			responseBody:       []byte(`{"meta": {}, "modules": []}`),
			query:              "test-query",
			expectErrSubstring: "no modules found",
		},
		{
			name:               "InvalidJSON",
			responseBody:       []byte(`not a json`),
			query:              "test-query",
			expectErrSubstring: "unmarshalling modules",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UnmarshalTFModulePlural(tc.responseBody, tc.query)

			if tc.expectErrSubstring == "" && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.expectErrSubstring != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectErrSubstring)
				}
				if !strings.Contains(err.Error(), tc.expectErrSubstring) {
					t.Errorf("expected error string %q to contain %q", err.Error(), tc.expectErrSubstring)
				}
			}
		})
	}
}

// --- UnmarshalModuleSingular ---

func TestUnmarshalModuleSingular_ValidAllFields(t *testing.T) {
	resp := []byte(`{
		"id": "namespace/name/provider/1.0.0",
		"owner": "owner",
		"namespace": "namespace",
		"name": "name",
		"version": "1.0.0",
		"provider": "provider",
		"provider_logo_url": "",
		"description": "A test module",
		"source": "source",
		"tag": "",
		"published_at": "2023-01-01T00:00:00Z",
		"downloads": 1,
		"verified": true,
		"root": {
			"path": "",
			"name": "root",
			"readme": "",
			"empty": false,
			"inputs": [
				{"name": "input1", "type": "string", "description": "desc", "default": "val", "required": true}
			],
			"outputs": [
				{"name": "output1", "description": "desc"}
			],
			"dependencies": [],
			"provider_dependencies": [
				{"name": "prov1", "namespace": "ns", "source": "src", "version": "1.0.0"}
			],
			"resources": []
		},
		"submodules": [],
		"examples": [
			{"path": "", "name": "example1", "readme": "example readme", "empty": false, "inputs": [], "outputs": [], "dependencies": [], "provider_dependencies": [], "resources": []}
		],
		"providers": ["provider"],
		"versions": ["1.0.0"],
		"deprecation": null
	}`)
	out, err := UnmarshalModuleSingular(resp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "A test module") {
		t.Errorf("expected output to contain module description, got %q", out)
	}
	if !strings.Contains(out, "input1") {
		t.Errorf("expected output to contain input variable, got %q", out)
	}
	if !strings.Contains(out, "example1") {
		t.Errorf("expected output to contain example name, got %q", out)
	}
}

func TestUnmarshalModuleSingular_EmptySections(t *testing.T) {
	resp := []byte(`{
		"id": "namespace/name/provider/1.0.0",
		"owner": "owner",
		"namespace": "namespace",
		"name": "name",
		"version": "1.0.0",
		"provider": "provider",
		"provider_logo_url": "",
		"description": "A test module",
		"source": "source",
		"tag": "",
		"published_at": "2023-01-01T00:00:00Z",
		"downloads": 1,
		"verified": true,
		"root": {
			"path": "",
			"name": "root",
			"readme": "",
			"empty": false,
			"inputs": [],
			"outputs": [],
			"dependencies": [],
			"provider_dependencies": [],
			"resources": []
		},
		"submodules": [],
		"examples": [],
		"providers": ["provider"],
		"versions": ["1.0.0"],
		"deprecation": null
	}`)
	out, err := UnmarshalModuleSingular(resp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "A test module") {
		t.Errorf("expected output to contain description, got %q", out)
	}
}

func TestUnmarshalModuleSingular_InvalidJSON(t *testing.T) {
	resp := []byte(`not a json`)
	_, err := UnmarshalModuleSingular(resp)
	if err == nil || !strings.Contains(err.Error(), "unmarshalling module details") {
		t.Errorf("expected unmarshalling error, got %v", err)
	}
}

// --- Others---

func TestExtractProviderNameAndVersion(t *testing.T) {
	uri := "registry://providers/hashicorp/namespace/aws/version/3.0.0"
	ns, name, version := ExtractProviderNameAndVersion(uri)
	if ns != "hashicorp" || name != "aws" || version != "3.0.0" {
		t.Errorf("expected (hashicorp, aws, 3.0.0), got (%s, %s, %s)", ns, name, version)
	}
}

func TestConstructProviderVersionURI(t *testing.T) {
	uri := ConstructProviderVersionURI("hashicorp", "aws", "3.0.0")
	expected := "registry://providers/hashicorp/providers/aws/versions/3.0.0"
	if uri != expected {
		t.Errorf("expected %q, got %q", expected, uri)
	}
}

func TestContainsSlug(t *testing.T) {
	ok, err := containsSlug("aws_s3_bucket", "s3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Errorf("expected true, got false")
	}
	ok, err = containsSlug("aws_s3_bucket", "ec2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected false, got true")
	}
}

func TestIsValidProviderVersionFormat(t *testing.T) {
	valid := []string{"1.0.0", "v1.2.3", "1.0.0-beta"}
	invalid := []string{"1.0", "v1", "foo", ""}
	for _, v := range valid {
		if !isValidProviderVersionFormat(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if isValidProviderVersionFormat(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestIsValidProviderDataType(t *testing.T) {
	valid := []string{"resources", "data-sources", "functions", "guides", "overview"}
	invalid := []string{"foo", "bar", ""}
	for _, v := range valid {
		if !isValidProviderDataType(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if isValidProviderDataType(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestLogAndReturnError_NilLogger(t *testing.T) {
	err := logAndReturnError(nil, "context", fmt.Errorf("fail"))
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Errorf("expected error to contain context, got %v", err)
	}
}

func TestIsV2ProviderDataType(t *testing.T) {
	valid := []string{"guides", "functions", "overview"}
	invalid := []string{"resources", "data-sources", "foo"}
	for _, v := range valid {
		if !isV2ProviderDataType(v) {
			t.Errorf("expected %q to be valid v2 data type", v)
		}
	}
	for _, v := range invalid {
		if isV2ProviderDataType(v) {
			t.Errorf("expected %q to be invalid v2 data type", v)
		}
	}
}

func TestLogAndReturnError(t *testing.T) {
	tests := []struct {
		name                string
		logger              *log.Logger
		context             string
		inputErr            error
		expectedErrContains []string
	}{
		{
			name:                "NilError_WithLogger",
			logger:              logger,
			context:             "test context nil error",
			inputErr:            nil,
			expectedErrContains: []string{"test context nil error"},
		},
		{
			name:                "NonNilError_WithLogger",
			logger:              logger,
			context:             "test context with error",
			inputErr:            fmt.Errorf("original error"),
			expectedErrContains: []string{"test context with error", "original error"},
		},
		{
			name:                "NilError_NilLogger",
			logger:              nil,
			context:             "nil logger context",
			inputErr:            nil,
			expectedErrContains: []string{"nil logger context"},
		},
		{
			name:                "NonNilError_NilLogger",
			logger:              nil,
			context:             "nil logger with error",
			inputErr:            fmt.Errorf("original nil logger error"),
			expectedErrContains: []string{"nil logger with error", "original nil logger error"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := logAndReturnError(tc.logger, tc.context, tc.inputErr)
			require.Error(t, err, "Expected an error to be returned")
			for _, expected := range tc.expectedErrContains {
				assert.Contains(t, err.Error(), expected, "Error message mismatch")
			}
		})
	}
}

func TestExtractReadme(t *testing.T) {
	tests := []struct {
		name     string
		readme   string
		expected string
	}{
		{
			name:     "NoHash",
			readme:   "No hash at all",
			expected: "No hash at all",
		},
		{
			name:     "SingleSection",
			readme:   "# Title\nSome content here.",
			expected: "# Title\nSome content here.",
		},
		{
			name:     "TwoSections",
			readme:   "# Title\nSome content here.\n\n# Section2\nMore content.",
			expected: "# Title\nSome content here.\n",
		},
		{
			name:     "ThreeSections",
			readme:   "# First\nContent1\n# Second\nContent2\n# Third\nContent3",
			expected: "# First\nContent1",
		},
		{
			name:     "HashAtEnd",
			readme:   "Some intro\n# OnlySection",
			expected: "Some intro\n# OnlySection",
		},
		{
			name:     "HashWithoutNextLine",
			readme:   "Some intro\n# OnlySection ## More Content",
			expected: "Some intro\n# OnlySection ## More Content",
		},
		{
			name:     "EmptyString",
			readme:   "",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractReadme(tc.readme)
			if result != tc.expected {
				t.Errorf("extractReadme(%q) = %q; want %q", tc.readme, result, tc.expected)
			}
		})
	}
}
