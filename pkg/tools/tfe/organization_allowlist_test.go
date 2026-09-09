// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckOrganizationAllowed(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	ctxWithAllowlist := client.WithOrganizationAllowlist(context.Background(),
		client.BuildAllowedOrganizationsMap([]string{"allowed-org"}))

	t.Run("allowed organization returns no error result", func(t *testing.T) {
		res, err := checkOrganizationAllowed(ctxWithAllowlist, logger, "workspace", "ws-1", &tfe.Organization{Name: "allowed-org"})
		require.NoError(t, err)
		assert.Nil(t, res)
	})

	t.Run("disallowed organization returns an error result", func(t *testing.T) {
		res, err := checkOrganizationAllowed(ctxWithAllowlist, logger, "workspace", "ws-1", &tfe.Organization{Name: "blocked-org"})
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.True(t, res.IsError)
	})

	t.Run("nil organization fails closed when an allowlist is configured", func(t *testing.T) {
		res, err := checkOrganizationAllowed(ctxWithAllowlist, logger, "team", "team-1", nil)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.True(t, res.IsError)
	})
}
