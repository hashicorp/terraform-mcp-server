// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
)

// Pagination defaults.
const (
	defaultPage     = 1
	defaultPageSize = 30
	minPageSize     = 1
	maxPageSize     = 100
)

// Pagination holds the page/pageSize inputs used by all list tools.
type Pagination struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"pageSize,omitempty"`
}

// ListOptions converts the pagination inputs into what the TFE client expects.
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
type PaginationDetails struct {
	CurrentPage  int `json:"current-page,omitempty"`
	PreviousPage int `json:"prev-page,omitempty"`
	NextPage     int `json:"next-page,omitempty"`
	TotalCount   int `json:"total-count,omitempty"`
	TotalPages   int `json:"total-pages,omitempty"`
}

// paginationDetails maps a TFE pagination response to our output type.
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

// paginationSchemaProperties returns a fresh property map for list-tool input
// schemas. Callers may add tool-specific properties without affecting others.
func paginationSchemaProperties() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{
		"page": {
			Type:        "integer",
			Description: "Page number for pagination (min 1)",
			Minimum:     ptr(float64(defaultPage)),
		},
		"pageSize": {
			Type:        "integer",
			Description: "Results per page for pagination (min 1, max 100)",
			Minimum:     ptr(float64(minPageSize)),
			Maximum:     ptr(float64(maxPageSize)),
		},
	}
}

// ptr is a convenience helper for taking the address of a literal value,
// needed wherever the SDK or schema types expect a *bool, *float64, etc.
func ptr[T any](v T) *T {
	return &v
}
