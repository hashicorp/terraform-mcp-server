// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetSentinelMockArguments struct {
	PlanID string `json:"plan_id" jsonschema:"The ID of the plan to export Sentinel mock data for (e.g., plan-8F5JFydVYAmtTjET)"`
}

type sentinelMockResult struct {
	PlanID       string `json:"plan_id"`
	PlanExportID string `json:"plan_export_id"`
	DataType     string `json:"data_type"`
	Format       string `json:"format"`
	Data         string `json:"data"`
}

func GetSentinelMockTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_sentinel_mock",
		Description: "Exports and downloads Sentinel mock bundle data for a Terraform plan. This data can be used to test Sentinel policies against plan output. The export is asynchronous - this tool handles polling until the export is ready.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get the Sentinel mock for a Terraform plan",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

func GetSentinelMockFunc(logger *slog.Logger) mcp.ToolHandlerFor[GetSentinelMockArguments, any] {
	return func(ctx context.Context, request *mcp.CallToolRequest, input GetSentinelMockArguments) (*mcp.CallToolResult, any, error) {
		return getSentinelMock(ctx, request, input, logger)
	}
}

func getSentinelMock(ctx context.Context, request *mcp.CallToolRequest, input GetSentinelMockArguments, logger *slog.Logger) (*mcp.CallToolResult, any, error) {
	planID := strings.TrimSpace(input.PlanID)
	if planID == "" {
		return nil, nil, fmt.Errorf("missing required input: plan_id")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get Terraform client: %w", err)
	}

	// Create the plan export
	dataType := tfe.PlanExportSentinelMockBundleV0
	planExport, err := tfeClient.PlanExports.Create(ctx, tfe.PlanExportCreateOptions{
		Plan:     &tfe.Plan{ID: planID},
		DataType: &dataType,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create plan export for Sentinel mock: %s: %w", planID, err)
	}

	planExportID := planExport.ID
	logger.Debug("created plan export, polling for completion",
		"plan_id", planID,
		"plan_export_id", planExportID,
	)

	// Poll until the export is finished
	maxAttempts := 30
	pollInterval := 2 * time.Second

	for i := 0; i < maxAttempts; i++ {
		planExport, err = tfeClient.PlanExports.Read(ctx, planExportID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read plan export status: %s: %w", planExportID, err)
		}

		switch planExport.Status {
		case tfe.PlanExportFinished:
			// Export is ready, download it
			data, err := tfeClient.PlanExports.Download(ctx, planExportID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to download plan export: %s: %w", planExportID, err)
			}

			// Return as base64-encoded tar.gz
			result, err := json.Marshal(sentinelMockResult{
				PlanID:       planID,
				PlanExportID: planExportID,
				DataType:     string(tfe.PlanExportSentinelMockBundleV0),
				Format:       "base64-tar-gz",
				Data:         base64.StdEncoding.EncodeToString(data),
			})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal Sentinel mock result: %w", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
			}, nil, nil

		case tfe.PlanExportErrored:
			return nil, nil, fmt.Errorf("plan export failed with error status for plan %s", planID)

		case tfe.PlanExportCanceled:
			return nil, nil, fmt.Errorf("plan export was canceled for plan %s", planID)

		case tfe.PlanExportExpired:
			return nil, nil, fmt.Errorf("plan export expired for plan %s", planID)

		case tfe.PlanExportPending, tfe.PlanExportQueued:
			// Still processing, wait and retry
			logger.Debug("plan export still processing, waiting",
				"plan_export_id", planExportID,
				"status", planExport.Status,
				"attempt", i+1,
			)

			select {
			case <-ctx.Done():
				return nil, nil, fmt.Errorf("context canceled while waiting for plan export: %w", ctx.Err())
			case <-time.After(pollInterval):
				continue
			}

		default:
			return nil, nil, fmt.Errorf("unexpected plan export status: %s", planExport.Status)
		}
	}

	return nil, nil, fmt.Errorf("plan export timed out after %d attempts for plan %s", maxAttempts, planID)
}
