// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// connectTestServer registers the resources on a fresh server and returns a
// connected client session.
func connectTestServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	svr := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	RegisterResources(svr, newTestLogger())
	RegisterResourceTemplates(svr, newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := svr.Connect(ctx, serverT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, clientT, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestRegisteredResources(t *testing.T) {
	cs := connectTestServer(t)

	res, err := cs.ListResources(context.Background(), nil)
	require.NoError(t, err)

	got := map[string]*mcp.Resource{}
	for _, r := range res.Resources {
		got[r.URI] = r
	}
	require.Len(t, got, 2)

	style := got["/terraform/style-guide"]
	require.NotNil(t, style)
	require.Equal(t, "Terraform Style Guide", style.Name)
	require.Equal(t, "text/markdown", style.MIMEType)

	modules := got["/terraform/module-development"]
	require.NotNil(t, modules)
	require.Equal(t, "Terraform Module Development Guide", modules.Name)
	require.Equal(t, "text/markdown", modules.MIMEType)
}

func TestRegisteredResourceTemplates(t *testing.T) {
	cs := connectTestServer(t)

	res, err := cs.ListResourceTemplates(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, res.ResourceTemplates, 1)

	tmpl := res.ResourceTemplates[0]
	require.Equal(t, "registry:/providers/{namespace}/name/{name}/version/{version}", tmpl.URITemplate)
	require.Equal(t, "Provider details", tmpl.Name)
	require.Equal(t, "text/markdown", tmpl.MIMEType)
}

func TestServerAdvertisesResourcesCapability(t *testing.T) {
	cs := connectTestServer(t)
	require.NotNil(t, cs.InitializeResult().Capabilities.Resources)
}

func TestReadUnknownResourceFails(t *testing.T) {
	cs := connectTestServer(t)

	_, err := cs.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "/terraform/does-not-exist"})
	require.Error(t, err)
}

func TestProviderResourceTemplateHelperRejectsBadURI(t *testing.T) {
	_, err := providerResourceTemplateHelper(context.Background(), http.DefaultClient, "bad-uri", nil)
	require.ErrorContains(t, err, "invalid provider URI format")
}

func TestProviderResourceResultUsesRequestedURI(t *testing.T) {
	const requested = "registry:/providers/hashicorp/name/random/version/3.6.0"

	res := providerResourceResult(requested, "# docs")

	require.Len(t, res.Contents, 1)
	require.Equal(t, requested, res.Contents[0].URI, "must be the requested URI, not the template")
	require.NotContains(t, res.Contents[0].URI, "{namespace}")
	require.Equal(t, "text/markdown", res.Contents[0].MIMEType)
	require.Equal(t, "# docs", res.Contents[0].Text)
}
