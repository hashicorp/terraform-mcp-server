// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected string
	}{
		{"empty string", "", ""},
		{"invalid url", "not-a-url", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHostname(tt.address)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestReadCredentialsFile(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)

	t.Run("empty hostname", func(t *testing.T) {
		result, err := ReadCredentialsFile("", logger)
		require.Empty(t, result)
		require.Error(t, err)
	})

	t.Run("file not found", func(t *testing.T) {
		result, err := ReadCredentialsFile("nonexistent.example.com", logger)
		require.Empty(t, result)
		require.Error(t, err)
	})

	t.Run("valid credentials file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "terraform-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		terraformDir := filepath.Join(tmpDir, ".terraform.d")
		err = os.MkdirAll(terraformDir, 0755)
		require.NoError(t, err)

		credContent := `{
  "credentials": {
    "app.terraform.io": {
      "token": "test-token-123"
    },
    "tfe.example.com": {
      "token": "enterprise-token-456"
    }
  }
}`
		credPath := filepath.Join(terraformDir, "credentials.tfrc.json")
		err = os.WriteFile(credPath, []byte(credContent), 0600)
		require.NoError(t, err)

		// Override HOME for this test
		t.Setenv("HOME", tmpDir)

		// Test finding a token
		token, err := ReadCredentialsFile("app.terraform.io", logger)
		require.Equal(t, "test-token-123", token)
		require.Empty(t, err)

		// Test finding another token
		token, err = ReadCredentialsFile("tfe.example.com", logger)
		require.Equal(t, "enterprise-token-456", token)
		require.NoError(t, err)

		// Test hostname not in file
		token, err = ReadCredentialsFile("other.terraform.io", logger)
		require.Empty(t, token)
		require.EqualError(t, err, "No credentials found for hostname \"other.terraform.io\" in credentials file")
	})

	t.Run("malformed json", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "terraform-test")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		terraformDir := filepath.Join(tmpDir, ".terraform.d")
		err = os.MkdirAll(terraformDir, 0755)
		require.NoError(t, err)

		credPath := filepath.Join(terraformDir, "credentials.tfrc.json")
		err = os.WriteFile(credPath, []byte("not valid json"), 0600)
		require.NoError(t, err)

		t.Setenv("HOME", tmpDir)

		token, err := ReadCredentialsFile("app.terraform.io", logger)
		require.Empty(t, token)
		require.Error(t, err)
	})
}
