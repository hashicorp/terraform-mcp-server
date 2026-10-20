// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package middleware

import (
	"context"

	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// TFEToolVisibility hides the tools that need a Terraform token from tools/list
// when the request has none. Hiding is only about the list. A hidden tool that
// is called anyway still returns its normal "no token" error.
func TFEToolVisibility(logger *log.Logger) mcp.Middleware {
	requiresTFE := make(map[string]bool)
	for _, td := range toolsets.AllTools {
		if td.RequiresTFE {
			requiresTFE[td.Name] = true
		}
	}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if method != "tools/list" || err != nil {
				return res, err
			}
			list, ok := res.(*mcp.ListToolsResult)
			if !ok || tfeclient.HasTerraformToken(ctx, logger) {
				return res, nil
			}
			visible := make([]*mcp.Tool, 0, len(list.Tools))
			for _, tool := range list.Tools {
				if !requiresTFE[tool.Name] {
					visible = append(visible, tool)
				}
			}
			list.Tools = visible
			return list, nil
		}
	}
}
