package acceptance

import (
	"context"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/client"

	"github.com/mark3labs/mcp-go/mcp"

	tfmcpserver "github.com/hashicorp/terraform-mcp-server/pkg/server"
	"github.com/sirupsen/logrus"
)

func TestAcceptance(t *testing.T) {
	ctx := context.Background()

	// TFE_TOKEN is required to run the tests
	if os.Getenv("TFE_TOKEN") == "" {
		t.Fatal("You must set the TFE_TOKEN` environment variable to run the acceptance tests")
	}

	logger := logrus.New()
	logger.SetOutput(&logInterceptor{t: t, suppress: true})
	srv := tfmcpserver.NewServer("acc-test", logger)

	sess := &TestSession{
		id:           srv.GenerateInProcessSessionID(),
		notifChannel: make(chan mcp.JSONRPCNotification, 10),
	}
	if err := srv.RegisterSession(ctx, sess); err != nil {
		t.Fatalf("failed to register session: %v", err)
	}
	sessionCtx := srv.WithContext(ctx, sess)

	mcpclient, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("Failed to start MCP client + server: %v", err)
	}
	defer mcpclient.Close()

	if _, err = mcpclient.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Failed to initialize the MCP client: %v", err)
	}

	for _, at := range TerraformToolTests {
		t.Run("terraform", func(t *testing.T) {
			runAcceptanceTest(t, sessionCtx, at, mcpclient)
		})
	}

	for _, at := range RegistryToolTests {
		t.Run("registry", func(t *testing.T) {
			runAcceptanceTest(t, sessionCtx, at, mcpclient)
		})
	}
}
