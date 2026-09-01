// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AccountDetails is the response shape returned by the whoami tool.
type AccountDetails struct {
	Username         string `json:"username"`
	Email            string `json:"email"`
	IsServiceAccount bool   `json:"is_service_account"`
}

// WhoAmIArguments holds the (empty) input for the whoami tool.
type WhoAmIArguments struct{}

func WhoAmITool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "whoami",
		Description: "Returns the identity of the currently authenticated Terraform token. Use this to determine which user or service account the active token belongs to.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get current Terraform identity",
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func WhoAmIFunc(ctx context.Context, request *mcp.CallToolRequest, _ WhoAmIArguments) (*mcp.CallToolResult, *AccountDetails, error) {
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	user, err := tfeClient.Users.ReadCurrent(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read account details: %w", err)
	}

	result := &AccountDetails{
		Username:         user.Username,
		Email:            user.Email,
		IsServiceAccount: user.IsServiceAccount,
	}

	return nil, result, nil
}
