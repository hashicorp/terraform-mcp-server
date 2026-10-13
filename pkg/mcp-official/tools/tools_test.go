// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterToolsSelectsCreateRunVariant(t *testing.T) {
	tests := []struct {
		name            string
		tfOpsEnabled    string
		wantDestructive bool
		wantRunTypes    []any
	}{
		{
			name:            "safe",
			tfOpsEnabled:    "false",
			wantDestructive: false,
			wantRunTypes:    []any{"plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply"},
		},
		{
			name:            "all run types",
			tfOpsEnabled:    "true",
			wantDestructive: true,
			wantRunTypes:    []any{"plan_and_apply", "refresh_state", "plan_only", "allow_empty_apply", "auto_approve", "is_destroy"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ENABLE_TF_OPERATIONS", test.tfOpsEnabled)

			implementation := &mcp.Implementation{Name: "test", Version: "test"}
			server := mcp.NewServer(implementation, nil)
			RegisterTools(
				server,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				toolsets.NewIndividualToolFilter([]string{"create_run"}),
			)

			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(t.Context(), serverTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, serverSession.Close()) })

			clientSession, err := mcp.NewClient(implementation, nil).Connect(t.Context(), clientTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, clientSession.Close()) })

			result, err := clientSession.ListTools(t.Context(), nil)
			require.NoError(t, err)
			require.Len(t, result.Tools, 1)

			tool := result.Tools[0]
			assert.Equal(t, "create_run", tool.Name)
			require.NotNil(t, tool.Annotations)
			require.NotNil(t, tool.Annotations.DestructiveHint)
			assert.Equal(t, test.wantDestructive, *tool.Annotations.DestructiveHint)

			data, err := json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			var schema jsonschema.Schema
			require.NoError(t, json.Unmarshal(data, &schema))
			assert.Equal(t, test.wantRunTypes, schema.Properties["run_type"].Enum)
		})
	}
}
