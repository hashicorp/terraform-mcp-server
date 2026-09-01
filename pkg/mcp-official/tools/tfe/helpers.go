// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
)

// Pagination defaults, matching utils.OptionalPaginationParams in the mark3labs
// tools so both endpoints page identically.
const (
	defaultPage     = 1
	defaultPageSize = 30
	minPageSize     = 1
	maxPageSize     = 100
)

// Pagination holds the optional pagination inputs shared by the list tools.
// Embed it in an arguments struct: both encoding/json and jsonschema-go promote
// the fields of an anonymous struct, so they appear as top-level properties.
type Pagination struct {
	Page     int `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize int `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

// ListOptions normalizes the pagination inputs into tfe.ListOptions. Absent or
// out-of-range values fall back to the defaults rather than being passed through
// as zero, which would otherwise let the TFE API apply its own page size.
func (p Pagination) ListOptions() tfe.ListOptions {
	page := p.Page
	if page < defaultPage {
		page = defaultPage
	}

	pageSize := p.PageSize
	if pageSize < minPageSize || pageSize > maxPageSize {
		pageSize = defaultPageSize
	}

	return tfe.ListOptions{PageNumber: page, PageSize: pageSize}
}

// PaginationDetails mirrors the pagination metadata the TFE API returns, using
// the same wire keys as tfe.Pagination.
//
// It exists rather than embedding *tfe.Pagination directly because that type's
// fields carry no omitempty, so every one of them is inferred as required in the
// tool's output schema. A nil *tfe.Pagination marshals with those fields absent,
// which then fails the SDK's output validation and turns a successful list into
// a protocol error. Embedding this by value keeps the fields optional and the
// nil case harmless.
type PaginationDetails struct {
	CurrentPage  int `json:"current-page,omitempty"`
	PreviousPage int `json:"prev-page,omitempty"`
	NextPage     int `json:"next-page,omitempty"`
	TotalCount   int `json:"total-count,omitempty"`
	TotalPages   int `json:"total-pages,omitempty"`
}

// paginationDetails converts go-tfe pagination metadata, tolerating a nil value.
func paginationDetails(p *tfe.Pagination) PaginationDetails {
	if p == nil {
		return PaginationDetails{}
	}

	return PaginationDetails{
		CurrentPage:  p.CurrentPage,
		PreviousPage: p.PreviousPage,
		NextPage:     p.NextPage,
		TotalCount:   p.TotalCount,
		TotalPages:   p.TotalPages,
	}
}

// inferSchema derives the input schema for T from its struct tags. It panics on
// failure because an arguments struct that cannot be represented as a schema is
// a programmer error, and this runs at tool registration during startup.
func inferSchema[T any](toolName string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("%s: inferring input schema: %v", toolName, err))
	}
	return schema
}

// withPaginationConstraints patches the numeric bounds onto the page and
// pageSize properties. The `jsonschema` struct tag can only set a description,
// so the min/max advertised by the mark3labs utils.WithPagination() would
// otherwise be lost.
func withPaginationConstraints(schema *jsonschema.Schema) *jsonschema.Schema {
	if page, ok := schema.Properties["page"]; ok {
		page.Minimum = ptr(float64(defaultPage))
	}
	if pageSize, ok := schema.Properties["pageSize"]; ok {
		pageSize.Minimum = ptr(float64(minPageSize))
		pageSize.Maximum = ptr(float64(maxPageSize))
	}
	return schema
}

// ptr returns a pointer to v, for the optional pointer fields in mcp.Tool
// annotations and jsonschema.Schema constraints.
func ptr[T any](v T) *T {
	return &v
}
