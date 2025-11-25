package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func CheckTextContentContains(expr string) ToolTestCheck {
	return func(t *testing.T, res *mcp.CallToolResult) {
		content, ok := res.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatal("response is not text content")
		}
		require.Contains(t, content.Text, expr)
	}
}

func CheckJSONContentExists(jsonPath string) ToolTestCheck {
	return func(t *testing.T, res *mcp.CallToolResult) {
		content, ok := res.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatal("response is not text content")
		}

		jsonData := map[string]any{}
		json.Unmarshal([]byte(content.Text), &jsonData)

		_, err := getByPath(jsonData, jsonPath)
		if err != nil {
			t.Fatalf("path does not exist %q", jsonPath)
		}
	}
}

func CheckJSONContent(jsonPath string, expected string) ToolTestCheck {
	return func(t *testing.T, res *mcp.CallToolResult) {
		content, ok := res.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatal("response is not text content")
		}

		jsonData := map[string]any{}
		json.Unmarshal([]byte(content.Text), &jsonData)

		val, err := getByPath(jsonData, jsonPath)
		if err != nil {
			t.Fatalf("path does not exist %q", jsonPath)
		}

		require.Equal(t, expected, val)
	}
}

func parsePart(part string) (key string, index int, hasIndex bool, err error) {
	key = part
	index = -1
	if i := strings.Index(part, "["); i != -1 {
		key = part[:i]
		j := strings.Index(part, "]")
		if j == -1 {
			return "", -1, false, errors.New("missing closing ']'")
		}

		idxStr := part[i+1 : j]
		idx, convErr := strconv.Atoi(idxStr)
		if convErr != nil {
			return "", -1, false, fmt.Errorf("invalid index '%s'", idxStr)
		}
		return key, idx, true, nil
	}
	return key, -1, false, nil
}

func getByPath(data map[string]any, path string) (any, error) {
	current := any(data)
	parts := strings.Split(path, ".")

	for _, raw := range parts {
		key, idx, hasIndex, err := parsePart(raw)
		if err != nil {
			return nil, err
		}

		if key != "" {
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected object for '%s'", key)
			}
			val, ok := obj[key]
			if !ok {
				return nil, fmt.Errorf("key not found: %s", key)
			}
			current = val
		}

		if hasIndex {
			list, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("expected list for index [%d]", idx)
			}
			if idx < 0 || idx >= len(list) {
				return nil, fmt.Errorf("index %d out of range", idx)
			}
			current = list[idx]
		}
	}

	return current, nil
}
