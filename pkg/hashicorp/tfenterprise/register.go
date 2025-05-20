// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfenterprise

import (
	"fmt"
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"terraform-mcp-server/pkg/hashicorp/tfenterprise/organization"
)

func Init(hcServer *server.MCPServer, logger *log.Logger, tfeToken string, tfeAddress string) error {
	config := &tfe.Config{
		Address:           tfeAddress,
		Token:             tfeToken,
		RetryServerErrors: true,
	}
	tfeClient, err := tfe.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create TFE client: %v", err)
	}

	addTools(hcServer, tfeClient, logger)

	return nil
}

func addTools(hcServer *server.MCPServer, tfeClient *tfe.Client, logger *log.Logger) {
	organization.InitOrganizationTools(hcServer, tfeClient, logger)
}
