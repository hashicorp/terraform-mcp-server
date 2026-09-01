// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
)

// GetHttpClient returns the session-scoped HTTP client for sessionID.
func GetHttpClient(ctx context.Context, sessionID string) (*http.Client, error) {
	return tfeclient.GetHttpClientForSession(ctx, sessionID, log.StandardLogger()), nil
}
