// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
)

// credentialsFile represents the structure of ~/.terraform.d/credentials.tfrc.json
type credentialsFile struct {
	Credentials map[string]credentialEntry `json:"credentials"`
}

type credentialEntry struct {
	Token string `json:"token"`
}

// ReadCredentialsFile reads the Terraform CLI credentials file and returns
// the token for the specified hostname. Returns empty string if not found.
func ReadCredentialsFile(hostname string) string {
	if hostname == "" {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	credPath := filepath.Join(homeDir, ".terraform.d", "credentials.tfrc.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		return ""
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return ""
	}

	if entry, ok := creds.Credentials[hostname]; ok {
		return entry.Token
	}

	return ""
}

// extractHostname extracts the hostname from a Terraform address URL.
// e.g., "https://app.terraform.io" -> "app.terraform.io"
func extractHostname(address string) string {
	if address == "" {
		return ""
	}

	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}

	return parsed.Hostname()
}
