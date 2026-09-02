// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
)

// Pagination defaults. Keep these in sync with legacy implementation
// so both behave the same when a caller omits page/pageSize.
const (
	defaultPage     = 1
	defaultPageSize = 30
	minPageSize     = 1
	maxPageSize     = 100
)

// Pagination holds the page/pageSize inputs used by all list tools.
// Embed it anonymously in an arguments struct and the fields are promoted automatically.
type Pagination struct {
	Page     int `json:"page,omitempty" jsonschema:"Page number for pagination (min 1)"`
	PageSize int `json:"pageSize,omitempty" jsonschema:"Results per page for pagination (min 1, max 100)"`
}

// ListOptions converts the pagination inputs into what the TFE client expects.
// Zero or out-of-range values fall back to our defaults
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

// PaginationDetails carries the page metadata we include in list responses.
//
// We can't embed *tfe.Pagination directly: its fields have no omitempty, so the
// SDK treats all of them as required output. When pagination is nil the fields
// are missing from the JSON, the SDK fails validation, and a successful list
// comes back as an error. This value type with omitempty avoids all of that.
type PaginationDetails struct {
	CurrentPage  int `json:"current-page,omitempty"`
	PreviousPage int `json:"prev-page,omitempty"`
	NextPage     int `json:"next-page,omitempty"`
	TotalCount   int `json:"total-count,omitempty"`
	TotalPages   int `json:"total-pages,omitempty"`
}

// paginationDetails maps a TFE pagination response to our output type.
// Safe to call with nil — returns an empty struct.
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

// inferSchema builds the JSON schema for T from its struct tags.
// Panics on failure — this only runs at startup during tool registration,
// and an error here means T has a field type or tag that can't be represented
// as JSON Schema
func inferSchema[T any](toolName string) *jsonschema.Schema {
	schema, err := jsonschema.For[T](nil)
	if err != nil {
		panic(fmt.Sprintf("%s: inferring input schema: %v", toolName, err))
	}
	return schema
}

// withPaginationConstraints adds the numeric min/max bounds to page and pageSize.
// Struct tags can only set a field description, so the bounds have to be patched
// onto the schema separately.
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

// enumOf converts string values into the []any form jsonschema.Schema.Enum expects.
// Lets a tool reuse its package-level list of valid values as the schema constraint,
// so the two can't drift apart.
func enumOf(values ...string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

// ptr is a convenience helper for taking the address of a literal value,
// needed wherever the SDK or schema types expect a *bool, *float64, etc.
func ptr[T any](v T) *T {
	return &v
}
