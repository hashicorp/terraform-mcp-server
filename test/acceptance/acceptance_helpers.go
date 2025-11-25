package acceptance

import (
	"context"
	"regexp"
	"testing"

	"github.com/mark3labs/mcp-go/client"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolAcceptanceTest encapsulates one test we want to run against an MCP server tool
type ToolAcceptanceTest struct {
	// Name of what we're testing
	Name string

	// Description of the test
	Description string

	// ToolName is the name of the the tool we want to test
	ToolName string

	// Arguments we want to pass into the tool call
	Arguments map[string]interface{}

	// ExpectError is a regexp to match expected error message
	ExpectError *regexp.Regexp

	// ExpectTextContent is a regexp to match expected text content
	ExpectTextContent *regexp.Regexp

	// Checks are arbitrary functions we can add to check behaviour
	Checks []ToolTestCheck

	// Skip the test
	Skip bool
}

type ToolTestCheck func(t *testing.T, res *mcp.CallToolResult)

func runAcceptanceTest(t *testing.T, ctx context.Context, at ToolAcceptanceTest, c *client.Client) {
	t.Run(at.Description, func(t *testing.T) {
		if at.Skip {
			t.Skip("Skip set to true")
			return
		}

		res, err := c.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      at.ToolName,
				Arguments: at.Arguments,
			},
		})

		if err != nil {
			if at.ExpectError != nil {
				if !at.ExpectError.MatchString(err.Error()) {
					t.Fatalf("Expected error from tool %q to match %q, got: %q", at.ToolName, at.ExpectError, err.Error())
				}
			} else {
				t.Fatalf("Error when calling tool %q: %v", at.ToolName, err)
			}
		} else if at.ExpectError != nil {
			t.Fatalf("Expected tool %q to error but it was called successfully", at.ToolName)
		}

		if at.ExpectTextContent != nil {
			content, ok := res.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatal("Response did not contain text content")
			}

			if !at.ExpectTextContent.MatchString(content.Text) {
				t.Fatalf("Expected text from tool %q to match %q, got: %q", at.ToolName, at.ExpectTextContent, content.Text)
			}
		}
	})
}
