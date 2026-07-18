// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProject(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	t.Run("tool creation", func(t *testing.T) {
		tool := CreateProject(logger)

		assert.Equal(t, "create_project", tool.Tool.Name)
		assert.Contains(t, tool.Tool.Description, "Creates a new project")
		assert.NotNil(t, tool.Handler)

		// Not destructive, not read-only
		assert.NotNil(t, tool.Tool.Annotations.DestructiveHint)
		assert.False(t, *tool.Tool.Annotations.DestructiveHint)
		assert.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
		assert.False(t, *tool.Tool.Annotations.ReadOnlyHint)

		// Required parameters
		assert.Contains(t, tool.Tool.InputSchema.Required, "terraform_org_name")
		assert.Contains(t, tool.Tool.InputSchema.Required, "project_name")

		// Optional description parameter exists but is not required
		assert.NotNil(t, tool.Tool.InputSchema.Properties["description"])
		assert.NotContains(t, tool.Tool.InputSchema.Required, "description")
	})

	t.Run("missing terraform_org_name returns error", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"project_name": "my-project",
		}}

		_, err := request.RequireString("terraform_org_name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "terraform_org_name")
	})

	t.Run("missing project_name returns error", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"terraform_org_name": "my-org",
		}}

		_, err := request.RequireString("project_name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "project_name")
	})

	t.Run("all required parameters present", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"terraform_org_name": "my-org",
			"project_name":       "my-project",
		}}

		orgName, err := request.RequireString("terraform_org_name")
		require.NoError(t, err)
		assert.Equal(t, "my-org", orgName)

		projectName, err := request.RequireString("project_name")
		require.NoError(t, err)
		assert.Equal(t, "my-project", projectName)
	})

	t.Run("description is optional and defaults to empty", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"terraform_org_name": "my-org",
			"project_name":       "my-project",
		}}

		description := request.GetString("description", "")
		assert.Equal(t, "", description)
	})

	t.Run("description is passed through when provided", func(t *testing.T) {
		request := &MockCallToolRequest{params: map[string]interface{}{
			"terraform_org_name": "my-org",
			"project_name":       "my-project",
			"description":        "A useful project",
		}}

		description := request.GetString("description", "")
		assert.Equal(t, "A useful project", description)
	})

	t.Run("ProjectSummary JSON marshaling", func(t *testing.T) {
		summary := &ProjectSummary{
			ID:   "prj-abc123",
			Name: "my-project",
		}

		data, err := json.Marshal(summary)
		require.NoError(t, err)
		assert.Contains(t, string(data), "prj-abc123")
		assert.Contains(t, string(data), "my-project")

		var out ProjectSummary
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, summary.ID, out.ID)
		assert.Equal(t, summary.Name, out.Name)
	})
}
