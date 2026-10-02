// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func downloadRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func TestImportConfigurationDownloadToolReturnsURLWithoutReadingLogOrArchive(t *testing.T) {
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	result, err := HandleGetImportConfigurationDownload(context.Background(), downloadRequest(map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "configuration_version_id": "cv-current"}), silentLogger())
	require.NoError(t, err)
	require.False(t, result.IsError, result.StructuredContent)
	out, ok := result.StructuredContent.(importDownload)
	require.True(t, ok)
	assert.Equal(t, "available", out.Status)
	assert.Equal(t, f.url+"/cv-archive?signed=fixture", out.DownloadURL)
	assert.Contains(t, strings.Join(out.Instructions, " "), "no Authorization header")
	assert.Contains(t, strings.Join(out.Instructions, " "), "secret")
	assert.NotContains(t, out.NextAction, out.DownloadURL)

	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.requests["GET /api/v2/configuration-versions/cv-current/download"])
	assert.Zero(t, f.requests["GET /cv-archive"], "the archive is never fetched")
	assert.Zero(t, f.requests["GET /logs"], "no QueryRun log read")
	assert.Zero(t, f.requests["GET /api/v2/queries/qry-fixture"])
	for request := range f.requests {
		assert.True(t, strings.HasPrefix(request, http.MethodGet+" "), request)
	}
}

func TestImportConfigurationDownloadToolRejectsNonCurrentConfigurationVersion(t *testing.T) {
	f := importBackendFixture(t)
	t.Setenv(client.TerraformAddress, f.url)
	t.Setenv(client.TerraformToken, "fixture-token")
	result, err := HandleGetImportConfigurationDownload(context.Background(), downloadRequest(map[string]any{"organization_name": "fixture-org", "workspace_name": "import-root", "configuration_version_id": "cv-speculative"}), silentLogger())
	require.NoError(t, err)
	assert.True(t, result.IsError)
	out := result.StructuredContent.(importDownload)
	assert.Contains(t, out.Diagnostics, "configuration_version_not_current")
	assert.Empty(t, out.DownloadURL)
}

func TestImportConfigurationDownloadToolRejectsBadInput(t *testing.T) {
	for _, args := range []map[string]any{
		{"organization_name": "fixture-org", "workspace_name": "import-root"},
		{"organization_name": "fixture-org", "workspace_name": "import-root", "configuration_version_id": "cv-current", "extra": true},
		{"organization_name": "../x", "workspace_name": "import-root", "configuration_version_id": "cv-current"},
	} {
		result, err := HandleGetImportConfigurationDownload(context.Background(), downloadRequest(args), silentLogger())
		require.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.StructuredContent.(importDownload).Diagnostics, "import_input_invalid")
	}
}
