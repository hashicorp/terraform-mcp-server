// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// GetSentinelMock creates a tool to export and download Sentinel mock data for a plan.
func GetSentinelMock(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_sentinel_mock",
			mcp.WithDescription(`Exports and downloads Sentinel mock bundle data for a Terraform plan. This data can be used to test Sentinel policies against plan output. The export is asynchronous - this tool handles polling until the export is ready.`),
			mcp.WithTitleAnnotation("Get Sentinel mock data for a Terraform plan"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("plan_id",
				mcp.Required(),
				mcp.Description("The ID of the plan to export Sentinel mock data for (e.g., plan-8F5JFydVYAmtTjET)"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getSentinelMockHandler(ctx, request, logger)
		},
	}
}

func getSentinelMockHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	planID, err := request.RequireString("plan_id")
	if err != nil {
		return ToolError(logger, "missing required input: plan_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	// Create the plan export
	dataType := tfe.PlanExportSentinelMockBundleV0
	planExport, err := tfeClient.PlanExports.Create(ctx, tfe.PlanExportCreateOptions{
		Plan:     &tfe.Plan{ID: planID},
		DataType: &dataType,
	})
	if err != nil {
		return ToolErrorf(logger, "failed to create plan export for plan %s: %v", planID, err)
	}

	logger.WithFields(log.Fields{
		"plan_id":        planID,
		"plan_export_id": planExport.ID,
	}).Debug("Created plan export, polling for completion")

	// Poll until the export is finished
	maxAttempts := 30
	pollInterval := 2 * time.Second

	for i := 0; i < maxAttempts; i++ {
		planExport, err = tfeClient.PlanExports.Read(ctx, planExport.ID)
		if err != nil {
			return ToolErrorf(logger, "failed to read plan export status: %v", err)
		}

		switch planExport.Status {
		case tfe.PlanExportFinished:
			// Export is ready, download it
			data, err := tfeClient.PlanExports.Download(ctx, planExport.ID)
			if err != nil {
				return ToolErrorf(logger, "failed to download plan export: %v", err)
			}

			// Return as base64-encoded tar.gz
			encoded := base64.StdEncoding.EncodeToString(data)
			result := fmt.Sprintf(`{"plan_id": "%s", "plan_export_id": "%s", "data_type": "sentinel-mock-bundle-v0", "format": "base64-tar-gz", "data": "%s"}`, planID, planExport.ID, encoded)
			return mcp.NewToolResultText(result), nil

		case tfe.PlanExportErrored:
			return ToolErrorf(logger, "plan export failed with error status for plan %s", planID)

		case tfe.PlanExportCanceled:
			return ToolErrorf(logger, "plan export was canceled for plan %s", planID)

		case tfe.PlanExportExpired:
			return ToolErrorf(logger, "plan export expired for plan %s", planID)

		case tfe.PlanExportPending, tfe.PlanExportQueued:
			// Still processing, wait and retry
			logger.WithFields(log.Fields{
				"status":  planExport.Status,
				"attempt": i + 1,
			}).Debug("Plan export still processing, waiting...")

			select {
			case <-ctx.Done():
				return ToolError(logger, "context canceled while waiting for plan export", ctx.Err())
			case <-time.After(pollInterval):
				continue
			}

		default:
			return ToolErrorf(logger, "unexpected plan export status: %s", planExport.Status)
		}
	}

	return ToolErrorf(logger, "plan export timed out after %d attempts for plan %s", maxAttempts, planID)
}
