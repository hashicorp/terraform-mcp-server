// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetTokenPermissionsArguments holds the input parameters for fetching token permissions.
type GetTokenPermissionsArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
}

// TokenPermissionsResult holds the list of permissions the current token has in an organization.
type TokenPermissionsResult struct {
	Permissions []string `json:"permissions"`
}

func GetTokenPermissionsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_token_permissions",
		Description: "Fetches the permissions the current token has for the specified terraform organization.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get permissions for current token",
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetTokenPermissionsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetTokenPermissionsArguments) (*mcp.CallToolResult, *TokenPermissionsResult, error) {
	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	org, err := tfeClient.Organizations.Read(ctx, input.TerraformOrgName)
	if err != nil {
		return nil, nil, fmt.Errorf("organization not found: %q", input.TerraformOrgName)
	}

	return nil, &TokenPermissionsResult{
		Permissions: tfeclient.HumanReadableTokenPermissions(org.Permissions),
	}, nil
}
