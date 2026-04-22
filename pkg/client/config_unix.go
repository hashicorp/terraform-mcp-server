// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package client

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

type platformConfig struct{}

func (*platformConfig) configDir(logger *log.Logger) (string, error) {
	logger.Debug("OS type is UNIX")
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ".terraform.d"), nil
}
