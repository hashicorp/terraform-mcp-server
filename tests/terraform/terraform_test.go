package terraform

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	mcpEndpoint string
	tfeToken    string

	testingClient *mcp.Client
)

type authTransport struct {
	token        string
	roundtripper http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.roundtripper.RoundTrip(req)
}

func init() {
	mcpEndpoint = os.Getenv("TF_MCP_ENDPOINT")
	if mcpEndpoint == "" {
		mcpEndpoint = "https://dev.mcp.terraform.io/mcp"
	}

	tfeToken = os.Getenv("TFE_TOKEN")

	testingClient = mcp.NewClient(&mcp.Implementation{
		Name:    "terraform-mcp-server-test-harness",
		Version: "v0.0.0",
	}, nil)
}

func newTestingSession(t *testing.T) *mcp.ClientSession {
	if mcpEndpoint == "" {
		mcpEndpoint = "https://dev.mcp.terraform.io/mcp"
		t.Logf("TF_MCP_ENDPOINT was not specified, using: %q", mcpEndpoint)
	}

	if tfeToken == "" {
		t.Skip("You need to supply TFE_TOKEN to run these tests")
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &authTransport{
			token:        tfeToken,
			roundtripper: http.DefaultTransport,
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, err := testingClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   mcpEndpoint,
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to create MCP client session: %v", err)
	}

	return session
}

func getTextContent(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}

	var b strings.Builder
	for _, c := range result.Content {
		if cc, ok := c.(*mcp.TextContent); ok && cc != nil {
			b.WriteString(cc.Text)
		}
	}

	return b.String()
}
