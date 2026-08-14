package terraform

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

const (
	toolCallTimeout  = 30 * time.Second
	alphaNum         = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomNameLength = 8
)

var (
	mcpEndpoint  string
	tfeToken     string
	tfeAddress   string
	tfeOrgName   string
	tfeUsername  string
	tfeUserEmail string

	testingClient      *mcp.Client
	enableTfOperations string
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
	tfeAddress = os.Getenv("TFE_ADDRESS")
	tfeOrgName = os.Getenv("TFE_ORG_NAME")
	tfeUsername = os.Getenv("TFE_USERNAME")
	tfeUserEmail = os.Getenv("TFE_USER_EMAIL")
	enableTfOperations = os.Getenv("ENABLE_TF_OPERATIONS")

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

	if tfeOrgName == "" {
		tfeOrgName = "terraform-ai-ecosystem-testing"
		t.Logf("TFE_ORG_NAME was not specified, using: %q", tfeOrgName)
	}

	if tfeUsername == "" {
		t.Skip("You need to supply TFE_USERNAME to run these tests")
	}

	if tfeUserEmail == "" {
		t.Skip("You need to supply TFE_USER_EMAIL to run these tests")
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

// newTFEClient creates a client that bypasses the MCP server and communicates
// directly with the TFE API. It can verify create, update, and delete tool calls
// against the actual API.
func tfeClient(t *testing.T) *tfe.Client {
	if tfeToken == "" {
		t.Skip("You need to supply TFE_TOKEN to run these tests")
	}

	address := tfeAddress
	if address == "" {
		address = "https://app.terraform.io"
	}

	client, err := tfe.NewClient(&tfe.Config{
		Address: address,
		Token:   tfeToken,
	})
	if err != nil {
		t.Fatalf("Failed to create direct TFE client: %v", err)
	}
	return client
}

func requireTfOperations(t *testing.T) {
	t.Helper()
	if enableTfOperations != "true" {
		t.Skip("skipping: ENABLE_TF_OPERATIONS is not set to true")
	}
}

func requireTeamsEntitlement(t *testing.T, client *tfe.Client) {
	t.Helper()
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Teams {
		t.Skipf("Organization %q does not have the Teams entitlement", tfeOrgName)
	}
}

func requireStacksEntitlement(t *testing.T, client *tfe.Client) {
	t.Helper()
	org, err := client.Organizations.Read(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read organization %q", tfeOrgName)
	if !org.Permissions.CanEnableStacks {
		t.Skipf("Organization %q does not have the Stacks entitlement", tfeOrgName)
	}
}

func randomName(prefix string) string {
	suffix := make([]byte, randomNameLength)
	for i := range suffix {
		suffix[i] = alphaNum[rand.Intn(len(alphaNum))]
	}

	return prefix + string(suffix)
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
		t.Logf("Tool call success: %q: %v", toolName, arguments)
	}
	return result, textContent
}

// waitFor polls until poll returns a non-nil result. Poll errors are retried
// and included in the timeout failure.
func waitFor[T any](t *testing.T, timeout time.Duration, description string, poll func(context.Context) (*T, error)) *T {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		result, err := poll(ctx)
		if err == nil && result != nil && ctx.Err() == nil {
			return result
		}
		if err != nil && ctx.Err() == nil {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				t.Fatalf("timed out after %s waiting for %s: last error: %v", timeout, description, lastErr)
			}
			t.Fatalf("timed out after %s waiting for %s", timeout, description)
		case <-ticker.C:
		}
	}
}
