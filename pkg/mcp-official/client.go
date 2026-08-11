package mcpofficial

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/hashicorp/go-tfe"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/hashicorp/terraform-mcp-server/version"
	log "github.com/sirupsen/logrus"
)

type contextKey string

const (
	TerraformAddress        = "TFE_ADDRESS"
	TerraformToken          = "TFE_TOKEN"
	TerraformSkipTLSVerify  = "TFE_SKIP_TLS_VERIFY"
	DefaultTerraformAddress = "https://app.terraform.io"
)

var (
	tfeClient     *tfe.Client
	tfeClientOnce sync.Once // To make the client singleton since credentials are read from env.
	tfeClientErr  error
)

func NewTfeClient(terraformAddress string, terraformSkipTLSVerify bool, terraformToken string) (*tfe.Client, error) {
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
	config.HTTPClient = tfeclient.CreateHTTPClient(terraformSkipTLSVerify, log.StandardLogger())

	client, err := tfe.NewClient(config)
	if err != nil {
		log.Printf("Failed to create a Terraform Cloud/Enterprise client: %v", err)
		return nil, err
	}

	log.Print("Created new TFE client...")
	return client, nil
}

// GetTfeClient returns the singleton TFE client, creating it on first call.
func GetTfeClient(ctx context.Context) (*tfe.Client, error) {
	tfeClientOnce.Do(func() {
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
			tfeClientErr = fmt.Errorf("terraform token is required but not found in context or environment variables")
			return
		}
		tfeClient, tfeClientErr = NewTfeClient(terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken)
	})
	return tfeClient, tfeClientErr
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
