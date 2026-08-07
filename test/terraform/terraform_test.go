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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	toolCallTimeout  = 30 * time.Second
	alphaNum         = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomNameLength = 8
)

var (
	mcpEndpoint string
	tfeToken    string
	tfeAddress  string
	tfeOrgName  string

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
	tfeAddress = os.Getenv("TFE_ADDRESS")
	tfeOrgName = os.Getenv("TFE_ORG_NAME")

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

func TestGetTeam(t *testing.T) {
	client := tfeClient(t)
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Teams {
		t.Skipf("Organization %q does not have the Teams entitlement", tfeOrgName)
	}

	// Get a real team ID to look up (the owners team always exists)
	teams, err := client.Teams.List(t.Context(), tfeOrgName, nil)
	require.NoError(t, err)
	require.NotEmpty(t, teams.Items, "Expected at least one team in the organization")
	teamID := teams.Items[0].ID

	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "get_team", map[string]any{
		"team_id": teamID,
	})

	require.False(t, result.IsError, "Tool call result should not be an error")
	require.NotEmpty(t, resultText, "Tool call result must not be empty")

	// MarshalPayloadWithoutIncluded returns a JSON:API envelope, not a flat object.
	assert.Equal(t, teamID, gjson.Get(resultText, "data.id").String(), "Response should contain the requested team ID")
	assert.NotEmpty(t, gjson.Get(resultText, "data.attributes.name").String(), "Response should contain the team name")
	assert.NotEmpty(t, gjson.Get(resultText, "data.attributes.visibility").String(), "Response should contain the team visibility")
	assert.True(t, gjson.Get(resultText, "data.attributes.users-count").Exists(), "Response should contain the user count field")
}
