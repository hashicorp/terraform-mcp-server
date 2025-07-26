// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !integration

package utils

import (
	"fmt"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Others---
var logger = log.New()

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
	ok, err := ContainsSlug("aws_s3_bucket", "s3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Errorf("expected true, got false")
	}
	ok, err = ContainsSlug("aws_s3_bucket", "ec2")
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
		if !IsValidProviderVersionFormat(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if IsValidProviderVersionFormat(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestIsValidProviderDataType(t *testing.T) {
	valid := []string{"resources", "data-sources", "functions", "guides", "overview"}
	invalid := []string{"foo", "bar", ""}
	for _, v := range valid {
		if !IsValidProviderDataType(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if IsValidProviderDataType(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestLogAndReturnError_NilLogger(t *testing.T) {
	err := LogAndReturnError(nil, "context", fmt.Errorf("fail"))
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Errorf("expected error to contain context, got %v", err)
	}
}

func TestIsV2ProviderDataType(t *testing.T) {
	valid := []string{"guides", "functions", "overview"}
	invalid := []string{"resources", "data-sources", "foo"}
	for _, v := range valid {
		if !IsV2ProviderDataType(v) {
			t.Errorf("expected %q to be valid v2 data type", v)
		}
	}
	for _, v := range invalid {
		if IsV2ProviderDataType(v) {
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
			err := LogAndReturnError(tc.logger, tc.context, tc.inputErr)
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
			result := ExtractReadme(tc.readme)
			if result != tc.expected {
				t.Errorf("extractReadme(%q) = %q; want %q", tc.readme, result, tc.expected)
			}
		})
	}
}
