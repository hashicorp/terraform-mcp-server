// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetTokenPermissionsArguments holds the input parameters for fetching token permissions.
type GetTokenPermissionsArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The name of the Terraform Cloud/Enterprise organization"`
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

func GetTokenPermissionsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetTokenPermissionsArguments) (*mcp.CallToolResult, []string, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	org, err := tfeClient.Organizations.Read(ctx, terraformOrgName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read organization %q: %w", terraformOrgName, err)
	}

	return nil, tfeclient.HumanReadableTokenPermissions(org.Permissions), nil
}
