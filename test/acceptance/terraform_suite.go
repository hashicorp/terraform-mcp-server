package acceptance

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark3labs/mcp-go/mcp"
)

const TFCTestOrg = "jhouston-test-org" // FIXME change this

var TerraformToolTests = []ToolAcceptanceTest{
	{
		Description: "List workspaces in an organization that does not exist",
		ToolName:    "list_workspaces",
		Arguments: map[string]interface{}{
			"terraform_org_name": "this_should_not_exist",
		},
		ExpectError: regexp.MustCompile(`resource not found`),
	},
	{
		Description: "List workspaces in an organization",
		ToolName:    "list_workspaces",
		Arguments: map[string]interface{}{
			"terraform_org_name": TFCTestOrg,
		},
		Checks: []AcceptanceTestCheck{
			func(t *testing.T, res *mcp.CallToolResult) {
				content, ok := res.Content[0].(mcp.TextContent)
				if !ok {
					t.Fatal("response is not text content")
				}

				response := map[string]interface{}{}
				json.Unmarshal([]byte(content.Text), &response)

				data, ok := response["data"].([]interface{})
				if !ok {
					t.Fatal(`expected response JSON to contain a "data" field`)
				}

				require.NotEmpty(t, data)
			},
		},
	},
}
