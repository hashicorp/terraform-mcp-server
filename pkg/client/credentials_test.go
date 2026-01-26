// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected string
	}{
		{"standard https", "https://app.terraform.io", "app.terraform.io"},
		{"with port", "https://tfe.example.com:8443", "tfe.example.com"},
		{"with path", "https://app.terraform.io/api/v2", "app.terraform.io"},
		{"http scheme", "http://localhost:8080", "localhost"},
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
	t.Run("empty hostname", func(t *testing.T) {
		result := ReadCredentialsFile("")
		require.Empty(t, result)
	})

	t.Run("file not found", func(t *testing.T) {
		result := ReadCredentialsFile("nonexistent.example.com")
		require.Empty(t, result)
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
		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", originalHome)

		// Test finding a token
		token := ReadCredentialsFile("app.terraform.io")
		require.Equal(t, "test-token-123", token)

		// Test finding another token
		token = ReadCredentialsFile("tfe.example.com")
		require.Equal(t, "enterprise-token-456", token)

		// Test hostname not in file
		token = ReadCredentialsFile("other.terraform.io")
		require.Empty(t, token)
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

		originalHome := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", originalHome)

		token := ReadCredentialsFile("app.terraform.io")
		require.Empty(t, token)
	})
}
