// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"context"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	officialclient "github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// WithSessionLifecycle runs once per session: it creates the session's
// TFE/HTTP clients, then waits in the background for the client to
// disconnect so it can clean up. It is the go-sdk equivalent of AddOnRegisterSession +
// AddOnUnregisterSession, shared by the HTTP bridge and the stdio runner
func WithSessionLifecycle(rateLimiter *client.RateLimitMiddleware, logger *log.Logger) Option {
	return WithOnSession(func(ctx context.Context, session *mcp.ServerSession) {
		if session == nil {
			return
		}
		sessionID := officialclient.SessionID(session)
		client.NewSessionHandler(ctx, sessionID, logger)
		go func() {
			_ = session.Wait()
			client.EndSessionHandler(context.Background(), sessionID, rateLimiter, logger)
		}()
	})
}
