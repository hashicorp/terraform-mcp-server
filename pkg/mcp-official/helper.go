package mcpofficial

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/hashicorp/terraform-mcp-server/version"
	log "github.com/sirupsen/logrus"
)

func CreateTfeClientForSession(ctx context.Context, sessionID string) (*tfe.Client, error) {
	var err error
	terraformAddress, ok := ctx.Value(contextKey(TerraformAddress)).(string)
	if !ok || terraformAddress == "" {
		terraformAddress = utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
	}

	terraformToken, ok := ctx.Value(contextKey(TerraformToken)).(string)
	if !ok || terraformToken == "" {
		terraformToken = utils.GetEnv(TerraformToken, "")
	}
	if terraformToken == "" {
		log.Print("Terraform token is empty")
		return nil, fmt.Errorf("terraform token is required but not found in context or environment variables")
	}

	client, err := NewTfeClient(sessionID, terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken)
	return client, err
}

func NewTfeClient(sessionId string, terraformAddress string, terraformSkipTLSVerify bool, terraformToken string) (*tfe.Client, error) {
	if terraformToken == "" {
		log.Print("No Terraform token provided, TFE client will not be available")
		return nil, fmt.Errorf("required input: no Terraform token provided")
	}

	config := &tfe.Config{
		Address:           terraformAddress,
		Token:             terraformToken,
		RetryServerErrors: true,
		Headers:           make(http.Header),
	}

	config.Headers.Set("User-Agent", fmt.Sprintf("terraform-mcp-server/%s", version.GetHumanVersion()))
	config.HTTPClient = createHTTPClient(terraformSkipTLSVerify)

	client, err := tfe.NewClient(config)
	if err != nil {
		log.Printf("Failed to create a Terraform Cloud/Enterprise client: %v", err)
		return nil, err
	}

	activeTfeClients.Store(sessionId, client)
	log.Printf("Created TFE client for session_id: %s", sessionId)
	return client, nil
}

func parseTerraformSkipTLSVerify(ctx context.Context) bool {
	terraformSkipTLSVerifyStr, ok := ctx.Value(contextKey(TerraformSkipTLSVerify)).(string)
	if !ok || terraformSkipTLSVerifyStr == "" {
		terraformSkipTLSVerifyStr = utils.GetEnv(TerraformSkipTLSVerify, "")
	}
	if terraformSkipTLSVerifyStr != "" {
		terraformSkipTLSVerify, err := strconv.ParseBool(terraformSkipTLSVerifyStr)
		if err == nil {
			return terraformSkipTLSVerify
		}
	}
	return false
}

// createHTTPClient initializes a retryable HTTP client
func createHTTPClient(insecureSkipVerify bool) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = log.New()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
	}
	transport.Proxy = http.ProxyFromEnvironment

	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.HTTPClient.Timeout = 10 * time.Second
	retryClient.HTTPClient.Transport = transport
	retryClient.RetryMax = 3

	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			resetAfterInt, err := strconv.ParseInt(resetAfter, 10, 64)
			if err != nil {
				return 0
			}
			resetAfterTime := time.Unix(resetAfterInt, 0)
			return time.Until(resetAfterTime)
		}
		return 0
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			return resetAfter != "", nil
		}
		return false, nil
	}

	return retryClient.StandardClient()
}
