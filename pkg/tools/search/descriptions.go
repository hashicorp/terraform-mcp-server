// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

// Descriptions are shared so the two MCP transports give agents identical
// Search workflow guidance. Each transport owns its own tool schema and result.
const (
	GenerateQueryConfigurationDescription = generateQueryConfigDescription
	ProviderListSchemaListDescription     = providerListSchemaListDescription
	ExecuteQueryDescription               = executeQueryDescription
	GetQueryStatusDescription             = getQueryStatusDescription
	GetQuerySummaryDescription            = getQuerySummaryDescription
)
