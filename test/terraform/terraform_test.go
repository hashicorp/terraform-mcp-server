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

const toolCallTimeout = 30 * time.Second

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
	tfeToken = os.Getenv("TFE_TOKEN")

	testingClient = mcp.NewClient(&mcp.Implementation{
		Name:    "terraform-mcp-server-test-harness",
		Version: "v0.0.0",
	}, nil)
}

func newTestingSession(t *testing.T) *mcp.ClientSession {
	if mcpEndpoint == "" {
		mcpEndpoint = "http://localhost:8080/mcp"
		t.Logf("TF_MCP_ENDPOINT was not specified, using: %q", mcpEndpoint)
	}

	if tfeToken == "" {
		t.Skip("You need to supply TFE_TOKEN to run these tests")
	}

	httpClient := &http.Client{
		Timeout: toolCallTimeout,
		Transport: &authTransport{
			token:        tfeToken,
			roundtripper: http.DefaultTransport,
		},
	}

	ctx, cancel := context.WithTimeout(t.Context(), toolCallTimeout)
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

func callTool(t *testing.T, s *mcp.ClientSession, toolName string, arguments map[string]any) (*mcp.CallToolResult, string) {
	result, err := s.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("Failed to call tool %q: %v", toolName, err)
	}

	textContent := getTextContent(result)
	if result.IsError {
		t.Logf("Tool call %q was an error: %v", toolName, textContent)
	} else {
		t.Logf("Tool call success: %q: %#v", toolName, arguments)
	}
	return result, textContent
}
