package tfenterprise

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-tfe"
)

func Init(token string, address string, enabled []string, cfg runConfig) (func(_ context.Context) (*tfe.Client, error), error) {
	config := &tfe.Config{
		Address:           address,
		Token:             token,
		RetryServerErrors: true, // Example configuration, adjust as needed
	}

	tfeClient, err := tfe.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TFE client: %v", err)
	}

	InitToolsets(enabled, cfg.readOnly, tfeClient, t)

	return getClient, nil
}
