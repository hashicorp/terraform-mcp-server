// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package client

import (
	"os"
	"path/filepath"
)

func configDir() (string, error) {
	// On Windows, Terraform uses %APPDATA%/terraform.d (no leading dot)
	appData := os.Getenv("APPDATA")
	if appData == "" {
		// Fallback to UserHomeDir if APPDATA not set
		dir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "terraform.d"), nil
	}
	return filepath.Join(appData, "terraform.d"), nil
}
