// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTerraformOperationsEnabled(t *testing.T) {
	// Save original env var
	originalValue := os.Getenv("ENABLE_TF_OPERATIONS")
	defer os.Setenv("ENABLE_TF_OPERATIONS", originalValue)

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"unset", "", false},
		{"false", "false", false},
		{"true", "true", true},
		{"TRUE", "TRUE", true},
		{"True", "True", true},
		{"invalid", "invalid", false},
		{"1", "1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("ENABLE_TF_OPERATIONS")
			} else {
				os.Setenv("ENABLE_TF_OPERATIONS", tt.envValue)
			}
			assert.Equal(t, tt.expected, isTerraformOperationsEnabled())
		})
	}
}

func TestDynamicToolStatelessRequestCredentials(t *testing.T) {
	logger := log.New()
	logger.SetOutput(io.Discard)
	r := &DynamicToolRegistry{logger: logger, sessionsWithTFE: map[string]bool{}}
	s := server.NewMCPServer("stateless-test", "1", server.WithToolCapabilities(true))
	s.AddTool(mcp.NewTool("credential_test"), r.wrapWithAvailabilityCheck("credential_test", func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("handler invoked"), nil
	}))
	h := server.NewTestStreamableHTTPServer(s, server.WithStateLess(true))
	defer h.Close()
	c, err := mcpclient.NewStreamableHttpClient(h.URL)
	require.NoError(t, err)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c.Start(ctx))
	_, err = c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "test", Version: "1"}}})
	require.NoError(t, err)
	for _, token := range []string{"fixture-token", ""} {
		t.Setenv(client.TerraformToken, token)
		result, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "credential_test", Arguments: map[string]any{}}})
		require.NoError(t, err)
		assert.Equal(t, token == "", result.IsError)
	}
}
