// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

type PaginationParams struct {
	Page     int
	PageSize int
	After    string
}

// OptionalParam is a helper function to retrieve an optional parameter from the request.
// It returns the value as type T and an error if the parameter is not present or cannot be converted.
func OptionalParam[T any](r mcp.CallToolRequest, p string) (T, error) {
	var zero T

	// Check if the parameter exists in the request
	if _, ok := r.GetArguments()[p]; !ok {
		return zero, nil
	}

	// Check if the parameter can be converted to type T
	if _, ok := r.GetArguments()[p].(T); !ok {
		return zero, fmt.Errorf("parameter %s is not of type %T, is %T", p, zero, r.GetArguments()[p])
	}

	return r.GetArguments()[p].(T), nil
}

// OptionalIntParam is a helper function to retrieve an optional integer parameter from the request.
// It returns the value as an int and an error if the parameter is not present or cannot be converted.
func OptionalIntParam(r mcp.CallToolRequest, p string) (int, error) {
	v, err := OptionalParam[float64](r, p)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// OptionalIntParamWithDefault retrieves an optional integer parameter from the request.
// If the parameter is not present or is zero, it returns the default value.
func OptionalIntParamWithDefault(r mcp.CallToolRequest, p string, d int) (int, error) {
	v, err := OptionalIntParam(r, p)
	if err != nil {
		return 0, err
	}
	if v == 0 {
		return d, nil
	}
	return v, nil
}

// OptionalPaginationParams returns pagination parameters from the request.
// It retrieves "page", "pageSize", and "after" parameters, providing defaults where necessary.
func OptionalPaginationParams(r mcp.CallToolRequest) (PaginationParams, error) {
	page, err := OptionalIntParamWithDefault(r, "page", 1)
	if err != nil {
		return PaginationParams{}, err
	}
	pageSize, err := OptionalIntParamWithDefault(r, "pageSize", 30)
	if err != nil {
		return PaginationParams{}, err
	}
	after, err := OptionalParam[string](r, "after")
	if err != nil {
		return PaginationParams{}, err
	}
	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
		After:    after,
	}, nil
}

// WithPagination adds pagination parameters to a tool.
// It adds "page", "pageSize", and "after" parameters with appropriate descriptions and defaults.
func WithPagination() mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithNumber("page",
			mcp.Description("Page number for pagination (min 1)"),
			mcp.Min(1),
		)(tool)

		mcp.WithNumber("pageSize",
			mcp.Description("Results per page for pagination (min 1, max 100)"),
			mcp.Min(1),
			mcp.Max(100),
		)(tool)
	}
}
