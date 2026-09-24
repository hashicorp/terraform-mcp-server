// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
)

func TestParseTagBindings(t *testing.T) {
	tests := []struct {
		name string
		tags string
		want []*tfe.TagBinding
	}{
		{
			name: "plain tag becomes a key-only binding",
			tags: "production",
			want: []*tfe.TagBinding{{Key: "production"}},
		},
		{
			name: "key:value becomes a key-value binding",
			tags: "env:staging",
			want: []*tfe.TagBinding{{Key: "env", Value: "staging"}},
		},
		{
			name: "surrounding spaces are trimmed from both key and value",
			tags: "  test-tag ,  env : staging  ",
			want: []*tfe.TagBinding{{Key: "test-tag"}, {Key: "env", Value: "staging"}},
		},
		{
			name: "only the first colon splits the entry",
			tags: "owner:team:platform",
			want: []*tfe.TagBinding{{Key: "owner", Value: "team:platform"}},
		},
		{
			name: "a trailing colon yields an empty value",
			tags: "env:",
			want: []*tfe.TagBinding{{Key: "env"}},
		},
		{
			name: "empty entries and trailing commas are dropped",
			tags: "a,,b,",
			want: []*tfe.TagBinding{{Key: "a"}, {Key: "b"}},
		},
		{
			name: "an entry with a blank key is dropped",
			tags: ":orphan,keep",
			want: []*tfe.TagBinding{{Key: "keep"}},
		},
		{
			name: "separators alone parse to nothing",
			tags: " , , ",
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, parseTagBindings(test.tags))
		})
	}
}

func TestFormatTagBinding(t *testing.T) {
	tests := []struct {
		name    string
		binding *tfe.TagBinding
		want    string
	}{
		{
			name:    "key-only binding renders as a bare key",
			binding: &tfe.TagBinding{Key: "production"},
			want:    "production",
		},
		{
			name:    "key-value binding renders as key:value",
			binding: &tfe.TagBinding{Key: "env", Value: "staging"},
			want:    "env:staging",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, formatTagBinding(test.binding))
		})
	}
}

// TestTagBindingRoundTrip pins the property the tag tools depend on: a tag created from a
// caller's string reads back in the form the caller wrote it.
func TestTagBindingRoundTrip(t *testing.T) {
	input := "test-tag,env:staging"

	var formatted []string
	for _, binding := range parseTagBindings(input) {
		formatted = append(formatted, formatTagBinding(binding))
	}

	assert.Equal(t, []string{"test-tag", "env:staging"}, formatted)
}
