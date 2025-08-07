// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCallToolRequest creates a mock CallToolRequest with the given arguments
func mockCallToolRequest(args map[string]interface{}) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

func TestOptionalParam(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		param       string
		expectValue interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "string parameter exists",
			args:        map[string]interface{}{"test": "value"},
			param:       "test",
			expectValue: "value",
			expectError: false,
		},
		{
			name:        "parameter does not exist",
			args:        map[string]interface{}{"other": "value"},
			param:       "missing",
			expectValue: "",
			expectError: false,
		},
		{
			name:        "parameter exists but wrong type",
			args:        map[string]interface{}{"test": 123},
			param:       "test",
			expectValue: "",
			expectError: true,
			errorMsg:    "parameter test is not of type string, is int",
		},
		{
			name:        "empty args map",
			args:        map[string]interface{}{},
			param:       "test",
			expectValue: "",
			expectError: false,
		},
		{
			name:        "nil value in args",
			args:        map[string]interface{}{"test": nil},
			param:       "test",
			expectValue: "",
			expectError: true,
			errorMsg:    "parameter test is not of type string, is <nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockCallToolRequest(tt.args)
			
			// Test with string type
			result, err := OptionalParam[string](req, tt.param)
			
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				if tt.expectValue != nil {
					assert.Equal(t, tt.expectValue, result)
				} else {
					assert.Equal(t, "", result) // zero value for string
				}
			}
		})
	}
}

func TestOptionalParam_DifferentTypes(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		param       string
		testType    string
		expectValue interface{}
		expectError bool
	}{
		{
			name:        "int type",
			args:        map[string]interface{}{"value": 42},
			param:       "value",
			testType:    "int",
			expectValue: 42,
			expectError: false,
		},
		{
			name:        "float64 type",
			args:        map[string]interface{}{"value": 3.14},
			param:       "value",
			testType:    "float64",
			expectValue: 3.14,
			expectError: false,
		},
		{
			name:        "bool type",
			args:        map[string]interface{}{"value": true},
			param:       "value",
			testType:    "bool",
			expectValue: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockCallToolRequest(tt.args)
			
			switch tt.testType {
			case "int":
				result, err := OptionalParam[int](req, tt.param)
				if tt.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.expectValue, result)
				}
			case "float64":
				result, err := OptionalParam[float64](req, tt.param)
				if tt.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.expectValue, result)
				}
			case "bool":
				result, err := OptionalParam[bool](req, tt.param)
				if tt.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.Equal(t, tt.expectValue, result)
				}
			}
		})
	}
}

func TestOptionalIntParam(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		param       string
		expectValue int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid float64 converts to int",
			args:        map[string]interface{}{"count": 42.0},
			param:       "count",
			expectValue: 42,
			expectError: false,
		},
		{
			name:        "valid float64 with decimal converts to int",
			args:        map[string]interface{}{"count": 42.7},
			param:       "count",
			expectValue: 42,
			expectError: false,
		},
		{
			name:        "parameter does not exist",
			args:        map[string]interface{}{"other": 123.0},
			param:       "missing",
			expectValue: 0,
			expectError: false,
		},
		{
			name:        "parameter exists but wrong type",
			args:        map[string]interface{}{"count": "not a number"},
			param:       "count",
			expectValue: 0,
			expectError: true,
			errorMsg:    "parameter count is not of type float64, is string",
		},
		{
			name:        "zero value",
			args:        map[string]interface{}{"count": 0.0},
			param:       "count",
			expectValue: 0,
			expectError: false,
		},
		{
			name:        "negative value",
			args:        map[string]interface{}{"count": -5.0},
			param:       "count",
			expectValue: -5,
			expectError: false,
		},
		{
			name:        "large value",
			args:        map[string]interface{}{"count": 999999.0},
			param:       "count",
			expectValue: 999999,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockCallToolRequest(tt.args)
			
			result, err := OptionalIntParam(req, tt.param)
			
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectValue, result)
			}
		})
	}
}

func TestOptionalIntParamWithDefault(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		param        string
		defaultValue int
		expectValue  int
		expectError  bool
		errorMsg     string
	}{
		{
			name:         "valid value returns value",
			args:         map[string]interface{}{"count": 42.0},
			param:        "count",
			defaultValue: 10,
			expectValue:  42,
			expectError:  false,
		},
		{
			name:         "parameter does not exist returns default",
			args:         map[string]interface{}{"other": 123.0},
			param:        "missing",
			defaultValue: 10,
			expectValue:  10,
			expectError:  false,
		},
		{
			name:         "zero value returns default",
			args:         map[string]interface{}{"count": 0.0},
			param:        "count",
			defaultValue: 10,
			expectValue:  10,
			expectError:  false,
		},
		{
			name:         "parameter exists but wrong type",
			args:         map[string]interface{}{"count": "not a number"},
			param:        "count",
			defaultValue: 10,
			expectValue:  0,
			expectError:  true,
			errorMsg:     "parameter count is not of type float64, is string",
		},
		{
			name:         "negative value returns value (not default)",
			args:         map[string]interface{}{"count": -5.0},
			param:        "count",
			defaultValue: 10,
			expectValue:  -5,
			expectError:  false,
		},
		{
			name:         "default value is zero",
			args:         map[string]interface{}{"other": 123.0},
			param:        "missing",
			defaultValue: 0,
			expectValue:  0,
			expectError:  false,
		},
		{
			name:         "default value is negative",
			args:         map[string]interface{}{"other": 123.0},
			param:        "missing",
			defaultValue: -1,
			expectValue:  -1,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockCallToolRequest(tt.args)
			
			result, err := OptionalIntParamWithDefault(req, tt.param, tt.defaultValue)
			
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectValue, result)
			}
		})
	}
}

func TestOptionalPaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectParams   PaginationParams
		expectError    bool
		errorMsg       string
	}{
		{
			name: "all parameters provided",
			args: map[string]interface{}{
				"page":     5.0,
				"pageSize": 20.0,
				"after":    "cursor123",
			},
			expectParams: PaginationParams{
				Page:     5,
				PageSize: 20,
				After:    "cursor123",
			},
			expectError: false,
		},
		{
			name: "no parameters provided - uses defaults",
			args: map[string]interface{}{},
			expectParams: PaginationParams{
				Page:     1,
				PageSize: 30,
				After:    "",
			},
			expectError: false,
		},
		{
			name: "only page provided",
			args: map[string]interface{}{
				"page": 3.0,
			},
			expectParams: PaginationParams{
				Page:     3,
				PageSize: 30,
				After:    "",
			},
			expectError: false,
		},
		{
			name: "only pageSize provided",
			args: map[string]interface{}{
				"pageSize": 50.0,
			},
			expectParams: PaginationParams{
				Page:     1,
				PageSize: 50,
				After:    "",
			},
			expectError: false,
		},
		{
			name: "only after provided",
			args: map[string]interface{}{
				"after": "token456",
			},
			expectParams: PaginationParams{
				Page:     1,
				PageSize: 30,
				After:    "token456",
			},
			expectError: false,
		},
		{
			name: "zero values use defaults",
			args: map[string]interface{}{
				"page":     0.0,
				"pageSize": 0.0,
				"after":    "",
			},
			expectParams: PaginationParams{
				Page:     1,
				PageSize: 30,
				After:    "",
			},
			expectError: false,
		},
		{
			name: "invalid page type",
			args: map[string]interface{}{
				"page": "not a number",
			},
			expectParams: PaginationParams{},
			expectError:  true,
			errorMsg:     "parameter page is not of type float64, is string",
		},
		{
			name: "invalid pageSize type",
			args: map[string]interface{}{
				"pageSize": "not a number",
			},
			expectParams: PaginationParams{},
			expectError:  true,
			errorMsg:     "parameter pageSize is not of type float64, is string",
		},
		{
			name: "invalid after type",
			args: map[string]interface{}{
				"after": 123,
			},
			expectParams: PaginationParams{},
			expectError:  true,
			errorMsg:     "parameter after is not of type string, is int",
		},
		{
			name: "negative page value",
			args: map[string]interface{}{
				"page": -1.0,
			},
			expectParams: PaginationParams{
				Page:     -1,
				PageSize: 30,
				After:    "",
			},
			expectError: false,
		},
		{
			name: "large values",
			args: map[string]interface{}{
				"page":     999.0,
				"pageSize": 100.0,
				"after":    "very-long-cursor-token-with-special-chars-!@#$%^&*()",
			},
			expectParams: PaginationParams{
				Page:     999,
				PageSize: 100,
				After:    "very-long-cursor-token-with-special-chars-!@#$%^&*()",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mockCallToolRequest(tt.args)
			
			result, err := OptionalPaginationParams(req)
			
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectParams, result)
			}
		})
	}
}

func TestWithPagination(t *testing.T) {
	// Test that WithPagination returns a valid ToolOption
	option := WithPagination()
	assert.NotNil(t, option)

	// Create a properly initialized tool to test the option
	tool := mcp.NewTool("test-tool", mcp.WithDescription("Test tool"))
	
	// Apply the pagination option
	option(&tool)
	
	// Verify that the tool has been modified and doesn't panic
	assert.NotNil(t, tool)
	
	// The function should not panic when applied to a valid tool
	// Since we can't easily inspect the internal structure of mcp.Tool,
	// we verify that the option can be applied without errors
}

func TestPaginationParams_Struct(t *testing.T) {
	// Test that PaginationParams struct can be created and accessed
	params := PaginationParams{
		Page:     5,
		PageSize: 25,
		After:    "cursor123",
	}
	
	assert.Equal(t, 5, params.Page)
	assert.Equal(t, 25, params.PageSize)
	assert.Equal(t, "cursor123", params.After)
	
	// Test zero values
	zeroParams := PaginationParams{}
	assert.Equal(t, 0, zeroParams.Page)
	assert.Equal(t, 0, zeroParams.PageSize)
	assert.Equal(t, "", zeroParams.After)
}

// Benchmark tests for performance
func BenchmarkOptionalParam(b *testing.B) {
	req := mockCallToolRequest(map[string]interface{}{
		"test": "value",
		"count": 42.0,
		"enabled": true,
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = OptionalParam[string](req, "test")
	}
}

func BenchmarkOptionalIntParam(b *testing.B) {
	req := mockCallToolRequest(map[string]interface{}{
		"count": 42.0,
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = OptionalIntParam(req, "count")
	}
}

func BenchmarkOptionalPaginationParams(b *testing.B) {
	req := mockCallToolRequest(map[string]interface{}{
		"page":     5.0,
		"pageSize": 20.0,
		"after":    "cursor123",
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = OptionalPaginationParams(req)
	}
}
