// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	searchlogic "github.com/hashicorp/terraform-mcp-server/pkg/tools/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestOfficialSearchArgumentsAndValidation(t *testing.T) {
	require.Contains(t, GetQueryStatusTool().Description, "query_run_id returned by execute_query")
	require.NotContains(t, GetQueryStatusTool().Description, "latest-query-run")
	schema, err := jsonschema.For[QueryRunArguments](nil)
	require.NoError(t, err)
	require.Equal(t, []string{"query_run_id"}, schema.Required)
	statusSchema, ok := GetQueryStatusTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, statusSchema.Required, "terminal")
	require.Contains(t, statusSchema.Properties, "status_timestamps")
	require.Contains(t, statusSchema.Properties, "retry_after_seconds")
	summarySchema, ok := GetQuerySummaryTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	require.Contains(t, summarySchema.Properties, "selectable")
	require.Contains(t, summarySchema.Properties, "lists")
	require.NotContains(t, summarySchema.Properties, "has_more")
	require.NotContains(t, summarySchema.Properties, "next_cursor")
	summaryInputSchema, err := jsonschema.For[QuerySummaryArguments](nil)
	require.NoError(t, err)
	require.NotContains(t, summaryInputSchema.Properties, "limit")
	require.NotContains(t, summaryInputSchema.Properties, "after")

	_, _, err = GetQueryStatusFunc(slog.Default())(context.Background(), nil, QueryRunArguments{QueryRunID: " "})
	require.ErrorContains(t, err, "missing required input: query_run_id")
	_, _, err = ExecuteQueryFunc(context.Background(), nil, ExecuteQueryArguments{
		OrganizationName: "org", WorkspaceName: "ws", QueryConfiguration: `{`,
	})
	require.ErrorContains(t, err, "invalid query_configuration")
}

func TestOfficialPrepareImportRejectsAbbreviatedProviderBeforeClientUse(t *testing.T) {
	input := searchlogic.PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []searchlogic.ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}},
		LockedProviders: map[string]string{"hashicorp/aws": "6.18.0"},
	}
	result, output, err := PrepareImportFunc(context.Background(), nil, input)
	require.Nil(t, result)
	require.Nil(t, output)
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.ErrorContains(t, err, `locked_providers key "hashicorp/aws"`)
	require.ErrorContains(t, err, `"registry.terraform.io/hashicorp/aws"`)

	schema, err := jsonschema.For[searchlogic.PrepareImportInput](nil)
	require.NoError(t, err)
	require.Contains(t, schema.Properties["locked_providers"].Description, "complete quoted hostname/namespace/type")
	require.Contains(t, schema.Properties["locked_providers"].Description, "do not abbreviate")
}

func TestOfficialSearchDescriptionsUseSharedValues(t *testing.T) {
	tests := []struct {
		tool *mcp.Tool
		want string
	}{
		{ProviderListSchemaListTool(), searchlogic.ProviderListSchemaListDescription},
		{GenerateQueryConfigurationTool(), searchlogic.GenerateQueryConfigurationDescription},
		{ExecuteQueryTool(), searchlogic.ExecuteQueryDescription},
		{GetQueryStatusTool(), searchlogic.GetQueryStatusDescription},
		{GetQuerySummaryTool(), searchlogic.GetQuerySummaryDescription},
		{PrepareImportTool(), searchlogic.PrepareImportDescription},
		{VerifyImportPlanTool(), searchlogic.VerifyImportPlanDescription},
	}
	for _, test := range tests {
		require.Equal(t, test.want, test.tool.Description, test.tool.Name)
	}
}

func TestOfficialSearchToolAnnotations(t *testing.T) {
	for _, tool := range []*mcp.Tool{GenerateQueryConfigurationTool(), ProviderListSchemaListTool(), GetQueryStatusTool(), GetQuerySummaryTool()} {
		require.True(t, tool.Annotations.ReadOnlyHint)
		require.False(t, *tool.Annotations.DestructiveHint)
	}
	require.False(t, ExecuteQueryTool().Annotations.ReadOnlyHint)
}
