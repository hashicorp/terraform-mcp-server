package acceptance

import (
	"regexp"
)

const TFCTestOrg = "terraform-mcp-server-acc"

var testWorkspaceName = randomName("test-acc")

var TerraformToolTests = []ToolAcceptanceTest{
	{
		Name:        "create_workspace",
		Description: "Create a terraform workspace",
		ToolName:    "create_workspace",
		Arguments: map[string]any{
			"terraform_org_name": TFCTestOrg,
			"workspace_name":     testWorkspaceName,
			"description":        "Acceptance test workspace",
		},
	},
	{
		Name:        "get_workspace_details",
		Description: "Get details for a terraform workspace",
		ToolName:    "get_workspace_details",
		Arguments: map[string]any{
			"terraform_org_name": TFCTestOrg,
			"workspace_name":     testWorkspaceName,
		},
		Checks: []ToolTestCheck{
			CheckJSONContent("data.attributes.workspace.name", testWorkspaceName),
			CheckJSONContent("data.attributes.workspace.description", "Acceptance test workspace"),
		},
	},
	{
		Name:        "get_workspace_details_not_found",
		Description: "Get details for a terraform workspace that does not exist",
		ToolName:    "get_workspace_details",
		Arguments: map[string]any{
			"terraform_org_name": TFCTestOrg,
			"workspace_name":     "bill-lumberg-tps-reports",
		},
		ExpectError: regexp.MustCompile(`resource not found`),
	},
	{
		Name:        "list_workspaces_bad_org",
		Description: "List workspaces in an organization that does not exist",
		ToolName:    "list_workspaces",
		Arguments: map[string]any{
			"terraform_org_name": "this_should_not_exist",
		},
		ExpectError: regexp.MustCompile(`resource not found`),
	},
	{
		Name:        "list_workspaces",
		Description: "List workspaces in an organization",
		ToolName:    "list_workspaces",
		Arguments: map[string]any{
			"terraform_org_name": TFCTestOrg,
		},
		Checks: []ToolTestCheck{
			CheckJSONContentExists("data"),
		},
	},
	{
		Name:        "list_terraform_orgs",
		Description: "List terraform organizations",
		ToolName:    "list_terraform_orgs",
		Arguments:   map[string]any{},
		Checks: []ToolTestCheck{
			CheckJSONContentExists(""),
		},
	},
	{
		Name:        "list_terraform_projects",
		Description: "List terraform projects",
		ToolName:    "list_terraform_projects",
		Arguments: map[string]any{
			"terraform_org_name": TFCTestOrg,
		},
		Checks: []ToolTestCheck{
			CheckJSONContentExists(""),
		},
	},
	{
		Name:        "list_terraform_projects_bad_org",
		Description: "List terraform projects with an organization that does not exist",
		ToolName:    "list_terraform_projects",
		Arguments: map[string]any{
			"terraform_org_name": "initech-123preq",
		},
		ExpectError: regexp.MustCompile(`resource not found`),
	},
	{
		Name:        "list_runs",
		Description: "List terraform runs with an organization",
		ToolName:    "list_runs",
		Arguments: map[string]any{
			"terraform_org_name": TFCTestOrg,
		},
		Checks: []ToolTestCheck{
			CheckJSONContentExists("data"),
		},
	},
	{
		Name:        "list_runs_bad_org",
		Description: "List terraform runs with an organization that does not exist",
		ToolName:    "list_runs",
		Arguments: map[string]any{
			"terraform_org_name": "initech-123preq",
		},
		ExpectError: regexp.MustCompile(`resource not found`),
	},
}
