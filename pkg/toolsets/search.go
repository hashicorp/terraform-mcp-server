// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

var searchTools = []ToolDef{
	{Name: "generate_query_configuration", Toolset: Search},
	{Name: "provider_list_schema_list", Toolset: Search, RequiresTFE: true},
	{Name: "execute_query", Toolset: Search, RequiresTFE: true},
	{Name: "get_query_status", Toolset: Search, RequiresTFE: true},
	{Name: "get_query_summary", Toolset: Search, RequiresTFE: true},
}
