package terraform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"log"
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
	toolCallTimeout  = 90 * time.Second
	alphaNum         = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomNameLength = 8

	defaultTfeOrgName  = "terraform-ai-ecosystem-testing"
	defaultMCPEndpoint = "http://localhost:8080/mcp"
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
	tfeOrgName = os.Getenv("TFE_ORG_NAME")
	tfeToken = os.Getenv("TFE_TOKEN")
	tfeAddress = os.Getenv("TFE_ADDRESS")
	tfeUsername = os.Getenv("TFE_USERNAME")
	tfeUserEmail = os.Getenv("TFE_USER_EMAIL")
	enableTfOperations = os.Getenv("ENABLE_TF_OPERATIONS")

	if mcpEndpoint == "" {
		mcpEndpoint = defaultMCPEndpoint
		log.Printf("TF_MCP_ENDPOINT was not specified, using: %q", mcpEndpoint)
	}
	if tfeOrgName == "" {
		tfeOrgName = defaultTfeOrgName
		log.Printf("TFE_ORG_NAME was not specified, using: %q", tfeOrgName)
	}

	// ElicitationHandler is used to provide default values for required parameters during no_code workspace creation.
	testingClient = mcp.NewClient(&mcp.Implementation{
		Name:    "terraform-mcp-server-test-harness",
		Version: "v0.0.0",
	}, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"name": "integration-test"},
			}, nil
		},
	})
}

func newTestingSession(t *testing.T) *mcp.ClientSession {
	requireTestConfig(t)

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
	requireTestConfig(t)

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

func requireTestConfig(t *testing.T) {
	t.Helper()
	if tfeToken == "" {
		t.Skip("You need to supply TFE_TOKEN to run these tests")
	}
	if tfeUsername == "" {
		t.Skip("You need to supply TFE_USERNAME to run these tests")
	}
	if tfeUserEmail == "" {
		t.Skip("You need to supply TFE_USER_EMAIL to run these tests")
	}
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

func requirePolicySetsEntitlement(t *testing.T, client *tfe.Client) {
	t.Helper()
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Sentinel {
		t.Skipf("Organization %q does not have the Sentinel/policy-sets entitlement", tfeOrgName)
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

func requireSentinelEntitlement(t *testing.T, client *tfe.Client) {
	t.Helper()
	entitlements, err := client.Organizations.ReadEntitlements(t.Context(), tfeOrgName)
	require.NoError(t, err, "Failed to read entitlements for organization %q", tfeOrgName)
	if !entitlements.Sentinel {
		t.Skipf("Organization %q does not have the Sentinel entitlement", tfeOrgName)
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

// packages the HCL as a gzipped tar archive and uploads it to the workspace.
func uploadConfiguration(t *testing.T, client *tfe.Client, workspaceID string, config string) {
	t.Helper()

	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	configuration := []byte(config)

	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name: "main.tf",
		Mode: 0o600,
		Size: int64(len(configuration)),
	}))
	_, err := tarWriter.Write(configuration)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	// Create the configuration-version record without auto-queuing a run.
	configurationVersion, err := client.ConfigurationVersions.Create(t.Context(), workspaceID, tfe.ConfigurationVersionCreateOptions{
		AutoQueueRuns: tfe.Bool(false),
	})
	require.NoError(t, err, "failed to create a configuration version")
	require.NoError(t, client.ConfigurationVersions.UploadTarGzip(t.Context(), configurationVersion.UploadURL, &archive), "failed to upload the test configuration")

	waitFor(t, toolCallTimeout, "configuration version to finish processing", func(ctx context.Context) (*tfe.ConfigurationVersion, error) {
		configurationVersion, err := client.ConfigurationVersions.Read(ctx, configurationVersion.ID)
		if err != nil || configurationVersion.Status != tfe.ConfigurationUploaded {
			return nil, err
		}
		return configurationVersion, nil
	})
}
