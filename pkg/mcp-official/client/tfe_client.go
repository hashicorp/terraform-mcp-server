// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"

	"github.com/hashicorp/go-tfe"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
)

// GetTfeClient returns the session-scoped TFE client for sessionID. Caching,
// token-rotation handling, and the stateless bypass all live in pkg/client
// so both the mark3labs and official-SDK transports share one cache and one contextKey type
func GetTfeClient(ctx context.Context, sessionID string) (*tfe.Client, error) {
	return tfeclient.GetTfeClientForSession(ctx, sessionID, log.StandardLogger())
}
