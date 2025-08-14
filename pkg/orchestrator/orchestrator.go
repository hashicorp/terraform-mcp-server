// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package orchestrator

import (
	log "github.com/sirupsen/logrus"
)

// Orchestrator handles workspace replication orchestration
type Orchestrator struct {
	logger *log.Logger
}

// NewOrchestrator creates a new orchestrator instance
func NewOrchestrator(logger *log.Logger) *Orchestrator {
	return &Orchestrator{
		logger: logger,
	}
}
