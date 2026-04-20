// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

// credentialsFile represents the structure of credentials.tfrc.json
type credentialsFile struct {
	Credentials map[string]credentialEntry `json:"credentials"`
}

type credentialEntry struct {
	Token string `json:"token"`
}

// ReadCredentialsFile reads the Terraform CLI credentials file and returns
// the token for the specified hostname
func ReadCredentialsFile(hostname string, logger *log.Logger) (string, error) {
	if hostname == "" {
		return "", errors.New("ReadCredentialsFile: hostname is empty")
	}

	cfg := newConfig()
	dir, err := cfg.configDir(logger)
	if err != nil {
		logger.Errorf("Failed to get config directory for credentials file lookup: %v", err)
		return "", err
	}

	credPath := filepath.Join(dir, "credentials.tfrc.json")

	data, err := os.ReadFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debugf("No credentials file found at %s", credPath)
		} else if os.IsPermission(err) {
			logger.Warnf("Permission denied reading credentials file at %s", credPath)
		} else {
			logger.Warnf("Failed to read credentials file at %s: %v", credPath, err)
		}
		return "", err
	}

	var creds credentialsFile
	if err := json.Unmarshal(data, &creds); err != nil {
		logger.Warnf("Failed to parse credentials file at %s: %v", credPath, err)
		return "", err
	}

	if entry, ok := creds.Credentials[hostname]; ok {
		return entry.Token, nil
	}

	logger.Debugf("No credentials found for hostname %q in credentials file", hostname)
	return "", fmt.Errorf("No credentials found for hostname %q in credentials file", hostname)
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
