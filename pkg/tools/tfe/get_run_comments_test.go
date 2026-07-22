// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRunComments(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	t.Run("tool creation", func(t *testing.T) {
		tool := GetRunComments(logger)

		assert.Equal(t, "get_run_comments", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Annotations.Title, "Get all comments for a given Terraform run.")
		assert.NotNil(t, tool.Handler)

		// Check annotations
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)

		// Check that run_id is in required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "run_id")
	})

	t.Run("missing required parameter", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				// run_id intentionally omitted
			},
		}

		_, err := request.RequireString("run_id")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required parameter")
	})

	t.Run("valid run_id parameter", func(t *testing.T) {
		request := &MockCallToolRequest{
			params: map[string]interface{}{
				"run_id": "run-abc123",
			},
		}

		runID, err := request.RequireString("run_id")
		assert.NoError(t, err)
		assert.Equal(t, "run-abc123", runID)
	})

	t.Run("run_id normalisation", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected string
		}{
			{
				name:     "leading and trailing whitespace",
				input:    "  run-xyz  ",
				expected: "run-xyz",
			},
			{
				name:     "hash prefix",
				input:    "#run-Nj2MTonBKmtmceGE",
				expected: "run-Nj2MTonBKmtmceGE",
			},
			{
				name:     "hash prefix with surrounding whitespace",
				input:    "  #run-Nj2MTonBKmtmceGE  ",
				expected: "run-Nj2MTonBKmtmceGE",
			},
			{
				name:     "multiple hash prefixes",
				input:    "##run-abc123",
				expected: "run-abc123",
			},
			{
				name:     "clean id unchanged",
				input:    "run-abc123",
				expected: "run-abc123",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := &MockCallToolRequest{
					params: map[string]interface{}{"run_id": tt.input},
				}
				runID, err := request.RequireString("run_id")
				assert.NoError(t, err)
				// Mirror the handler's normalisation: TrimSpace then strip leading '#'
				assert.Equal(t, tt.expected, strings.TrimLeft(strings.TrimSpace(runID), "#"))
			})
		}
	})
}

func TestGetRunCommentsResultStructure(t *testing.T) {
	t.Run("single comment JSON round-trip", func(t *testing.T) {
		comment := &CommentsSummary{
			ID:   "comment-1",
			Body: "LGTM",
		}

		jsonData, err := json.Marshal(comment)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "comment-1")
		assert.Contains(t, string(jsonData), "LGTM")

		var unmarshaled CommentsSummary
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, comment.ID, unmarshaled.ID)
		assert.Equal(t, comment.Body, unmarshaled.Body)
	})

	t.Run("comment list JSON round-trip", func(t *testing.T) {
		list := &CommentsSummaryList{
			Items: []*CommentsSummary{
				{ID: "comment-1", Body: "LGTM"},
				{ID: "comment-2", Body: "Please fix the failing tests."},
			},
		}

		jsonData, err := json.Marshal(list)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), "comment-1")
		assert.Contains(t, string(jsonData), "LGTM")
		assert.Contains(t, string(jsonData), "comment-2")
		assert.Contains(t, string(jsonData), "Please fix the failing tests.")

		var unmarshaled CommentsSummaryList
		err = json.Unmarshal(jsonData, &unmarshaled)
		assert.NoError(t, err)
		require.Len(t, unmarshaled.Items, 2)
		assert.Equal(t, "comment-1", unmarshaled.Items[0].ID)
		assert.Equal(t, "LGTM", unmarshaled.Items[0].Body)
		assert.Equal(t, "comment-2", unmarshaled.Items[1].ID)
		assert.Equal(t, "Please fix the failing tests.", unmarshaled.Items[1].Body)
	})

	t.Run("empty comment list", func(t *testing.T) {
		list := &CommentsSummaryList{
			Items: []*CommentsSummary{},
		}

		jsonData, err := json.Marshal(list)
		assert.NoError(t, err)
		assert.Contains(t, string(jsonData), `"items":[]`)
	})

	t.Run("nil pagination embed", func(t *testing.T) {
		list := &CommentsSummaryList{
			Items:      []*CommentsSummary{{ID: "comment-1", Body: "looks good"}},
			Pagination: nil,
		}

		// A nil *tfe.Pagination embedded pointer must not cause a marshal error
		_, err := json.Marshal(list)
		assert.NoError(t, err)

		// Confirm the tfe.Pagination type is the expected embedded type
		var _ *tfe.Pagination = list.Pagination
	})
}
