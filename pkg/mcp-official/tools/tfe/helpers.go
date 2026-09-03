// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"
	"slices"

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

// outputSchema builds the JSON schema for a tool's result type.
//
// It narrows the nullable type unions that inference produces (see
// narrowNullableTypes) and is deliberately kept separate from inferSchema:
// outputs are values we produce, so promising "never null" is a promise we can
// keep, whereas inputs come from the caller and should stay permissive.
func outputSchema[T any](toolName string) *jsonschema.Schema {
	return narrowNullableTypes(inferSchema[T](toolName))
}

// narrowNullableTypes rewrites `"type": ["null", X]` unions to a plain
// `"type": X` throughout schema, in place.
//
// jsonschema-go emits the union for every slice and every pointer-following
// field, because a nil slice or nil pointer marshals to JSON null. That is
// accurate for the Go type and is an intentional upstream default (see the
// typeschemasnull note in jsonschema's doc.go), not a bug. We narrow it anyway
// for two reasons:
//
//  1. The array form of `type` is legal JSON Schema, but some MCP clients read
//     `type` as a single string and either reject the tool or silently drop the
//     constraint.
//  2. Our list tools never emit null: they return an error rather than an empty
//     result, build their slice with make, and assign every element. The
//     nonNilSlice guard keeps that true, so the narrower schema is the honest
//     contract.
//
// Narrowing only applies when removing "null" leaves exactly one type, which is
// the only case that resolves the portability problem. A genuine multi-type
// union (or a bare ["null"]) is left exactly as-is: it would still marshal to
// the array form, so rewriting it would change the accepted values without
// fixing anything. Neither case occurs in our current result types.
func narrowNullableTypes(schema *jsonschema.Schema) *jsonschema.Schema {
	if schema == nil {
		return nil
	}

	// Recurse first so nested nodes are narrowed regardless of this node's type.
	for _, property := range schema.Properties {
		narrowNullableTypes(property)
	}
	for _, item := range schema.PrefixItems {
		narrowNullableTypes(item)
	}
	for _, def := range schema.Defs {
		narrowNullableTypes(def)
	}
	for _, branch := range [][]*jsonschema.Schema{schema.AnyOf, schema.AllOf, schema.OneOf} {
		for _, sub := range branch {
			narrowNullableTypes(sub)
		}
	}
	narrowNullableTypes(schema.Items)
	narrowNullableTypes(schema.AdditionalProperties)
	narrowNullableTypes(schema.Not)

	if len(schema.Types) == 0 {
		return schema
	}

	remaining := slices.DeleteFunc(slices.Clone(schema.Types), func(t string) bool {
		return t == "null"
	})

	// Type and Types are mutually exclusive on the wire: the marshaller emits
	// Type when it is set and Types otherwise, so Types must be cleared here.
	if len(remaining) == 1 {
		schema.Type = remaining[0]
		schema.Types = nil
	}
	return schema
}

// nonNilSlice returns an empty slice in place of a nil one, so a list tool's
// result marshals as [] rather than null. This is what makes the narrowed
// output schema structurally true instead of merely intended: without it, a
// future refactor to `var items []*T` plus append would emit null against a
// schema that no longer permits it.
func nonNilSlice[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
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
