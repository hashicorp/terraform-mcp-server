// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"

	importworkflow "github.com/hashicorp/terraform-mcp-server/pkg/tools/search/import"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

type importHandler func(context.Context, mcp.CallToolRequest, *log.Logger) (*mcp.CallToolResult, error)

func importTool(tool mcp.Tool, handler importHandler, logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: tool,
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handler(ctx, request, logger)
		},
	}
}

// The focused Search-to-import tools share one import package. Only the two
// create tools mutate, and each needs explicit confirmation.

// PrepareImport validates a selection and returns the schema, baseline and carry block.
func PrepareImport(logger *log.Logger) server.ServerTool {
	return importTool(importworkflow.PrepareImportDefinition(), importworkflow.HandlePrepareImport, logger)
}

// GetImportConfigurationDownload returns the short-lived current-configuration archive URL.
func GetImportConfigurationDownload(logger *log.Logger) server.ServerTool {
	return importTool(importworkflow.GetImportConfigurationDownloadDefinition(), importworkflow.HandleGetImportConfigurationDownload, logger)
}

// CreateImportCV creates the speculative configuration version after confirmation.
func CreateImportCV(logger *log.Logger) server.ServerTool {
	return importTool(importworkflow.CreateImportCVDefinition(), importworkflow.HandleCreateImportCV, logger)
}

// CreateImportRun creates the CV-bound plan-only run after confirmation.
func CreateImportRun(logger *log.Logger) server.ServerTool {
	return importTool(importworkflow.CreateImportRunDefinition(), importworkflow.HandleCreateImportRun, logger)
}

// VerifyImportPlan summarizes what a finished plan-only run showed.
func VerifyImportPlan(logger *log.Logger) server.ServerTool {
	return importTool(importworkflow.VerifyImportPlanDefinition(), importworkflow.HandleVerifyImportPlan, logger)
}
