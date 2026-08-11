// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// WhoAmI creates a tool to get the identity of the currently authenticated token
func WhoAmI(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("whoami",
			mcp.WithDescription(`Returns the identity of the currently authenticated Terraform token. Use this to determine which user or service account the active token belongs to.`),
			mcp.WithTitleAnnotation("Get current Terraform identity"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return whoAmIHandler(ctx, req, logger)
		},
	}
}

// accountDetails is the response shape returned by the whoami tool.
type accountDetails struct {
	Username         string `json:"username"`
	Email            string `json:"email"`
	IsServiceAccount bool   `json:"is_service_account"`
}

func whoAmIHandler(ctx context.Context, _ mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	user, err := tfeClient.Users.ReadCurrent(ctx)
	if err != nil {
		return ToolError(logger, "failed to read account details", err)
	}

	result := accountDetails{
		Username:         user.Username,
		Email:            user.Email,
		IsServiceAccount: user.IsServiceAccount,
	}

	buf, err := json.Marshal(result)
	if err != nil {
		return ToolError(logger, "failed to marshal account details", err)
	}

	return mcp.NewToolResultText(string(buf)), nil
}
