// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package client

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

type platformConfig struct{}

func (*platformConfig) configDir(logger *log.Logger) (string, error) {
	// On Windows, Terraform uses %APPDATA%/terraform.d (no leading dot)
	logger.Info("OS type is WINDOWS")
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
