// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolError(t *testing.T) {
	cause := errors.New("token missing")
	tests := []struct {
		name      string
		message   string
		cause     error
		wantError string
	}{
		{
			name:      "wraps cause",
			message:   "getting Terraform client",
			cause:     cause,
			wantError: "getting Terraform client: token missing",
		},
		{
			name:      "message only",
			message:   "no organizations to list",
			wantError: "no organizations to list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			logger := log.New()
			logger.SetOutput(&output)
			logger.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

			err := toolError(logger, tt.message, tt.cause)

			require.EqualError(t, err, tt.wantError)
			assert.Contains(t, output.String(), "Tool error: "+tt.wantError)
			if tt.cause != nil {
				assert.ErrorIs(t, err, tt.cause)
			}
		})
	}
}

func TestToolErrorMCPResult(t *testing.T) {
	var output bytes.Buffer
	logger := log.New()
	logger.SetOutput(&output)
	logger.SetFormatter(&log.TextFormatter{DisableTimestamp: true})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "fail"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return nil, nil, toolError(logger, "getting Terraform client", errors.New("token missing"))
		})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, serverSession.Close()) })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, clientSession.Close()) })

	result, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{Name: "fail"})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t, "getting Terraform client: token missing", text.Text)
	assert.Contains(t, output.String(), "Tool error: getting Terraform client: token missing")
}
