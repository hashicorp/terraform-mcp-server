package terraform

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// deleteProject deletes a project using its own context so it can be safely
// called from t.Cleanup, where t.Context() is already cancelled.
func deleteProject(t *testing.T, s *mcp.ClientSession, projectID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), toolCallTimeout)
	defer cancel()
	_, _ = s.CallTool(ctx, &mcp.CallToolParams{
		Name:      "delete_project",
		Arguments: map[string]any{"project_id": projectID},
	})
}

func TestListAndGetProject(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	orgsResult, orgsText := callTool(t, s, "list_terraform_orgs", map[string]any{})
	require.False(t, orgsResult.IsError, "list_terraform_orgs should not return an error")
	require.NotEmpty(t, orgsText, "list_terraform_orgs should return a non-empty response")

	orgName := gjson.Get(orgsText, "items.0.organization_name").String()
	require.NotEmpty(t, orgName, "expected at least one organization to be available")

	projectsResult, projectsText := callTool(t, s, "list_terraform_projects", map[string]any{
		"terraform_org_name": orgName,
	})
	require.False(t, projectsResult.IsError, "list_terraform_projects should not return an error")
	require.NotEmpty(t, projectsText, "list_terraform_projects should return a non-empty response")

	projectID := gjson.Get(projectsText, "items.0.project_id").String()
	require.NotEmpty(t, projectID, "expected at least one project to be available in org %q", orgName)

	t.Run("returns project details for a valid project_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_project", map[string]any{
			"project_id": projectID,
		})

		require.False(t, result.IsError, "Tool call result should not be an error")
		require.NotEmpty(t, resultText, "Tool call result must not be empty")

		assert.Equal(t, projectID, gjson.Get(resultText, "project_id").String(), "response should contain the requested project_id")
		assert.NotEmpty(t, gjson.Get(resultText, "project_name").String(), "response should contain a project_name")
		assert.True(t, gjson.Get(resultText, "is_unified").Exists(), "response should always contain the is_unified field")
	})

	t.Run("returns an error for a non-existent project_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_project", map[string]any{
			"project_id": "prj-doesnotexist000",
		})

		require.True(t, result.IsError, "Tool call should return an error for an invalid project_id")
		assert.Contains(t, resultText, "prj-doesnotexist000", "error message should reference the unknown project_id")
	})
}

func TestCreateAndDeleteProject(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	orgsResult, orgsText := callTool(t, s, "list_terraform_orgs", map[string]any{})
	require.False(t, orgsResult.IsError, "list_terraform_orgs should not return an error")
	require.NotEmpty(t, orgsText, "list_terraform_orgs should return a non-empty response")

	orgName := gjson.Get(orgsText, "items.0.organization_name").String()
	require.NotEmpty(t, orgName, "expected at least one organization to be available")

	// Use a timestamped name to avoid collisions across concurrent or repeated runs.
	projectName := fmt.Sprintf("mcp-test-%d", time.Now().UnixMilli())

	t.Run("creates a project", func(t *testing.T) {
		createResult, createText := callTool(t, s, "create_project", map[string]any{
			"terraform_org_name": orgName,
			"project_name":       projectName,
			"description":        "Created by terraform-mcp-server integration tests",
		})

		require.False(t, createResult.IsError, "create_project should not return an error")
		require.NotEmpty(t, createText, "create_project should return a non-empty response")

		createdID := gjson.Get(createText, "project_id").String()
		require.NotEmpty(t, createdID, "create_project response should contain a project_id")
		assert.Equal(t, projectName, gjson.Get(createText, "project_name").String(), "create_project response should echo back the project name")

		// Safety net: delete the project if any assertion below fails before the
		// explicit delete subtest runs. Uses a standalone context because
		// t.Context() is already cancelled when Cleanup fires.
		t.Cleanup(func() { deleteProject(t, s, createdID) })

		t.Run("get_project returns the newly created project", func(t *testing.T) {
			getResult, getText := callTool(t, s, "get_project", map[string]any{
				"project_id": createdID,
			})

			require.False(t, getResult.IsError, "get_project should not return an error for the newly created project")
			assert.Equal(t, createdID, gjson.Get(getText, "project_id").String(), "get_project should return the correct project_id")
			assert.Equal(t, projectName, gjson.Get(getText, "project_name").String(), "get_project should return the correct project_name")
			assert.Equal(t, "Created by terraform-mcp-server integration tests", gjson.Get(getText, "description").String(), "get_project should return the correct description")
		})

		t.Run("deletes the project", func(t *testing.T) {
			deleteResult, deleteText := callTool(t, s, "delete_project", map[string]any{
				"project_id": createdID,
			})

			require.False(t, deleteResult.IsError, "delete_project should not return an error for an empty project")
			assert.Contains(t, deleteText, createdID, "delete_project response should reference the deleted project_id")
		})

		t.Run("get_project returns an error after deletion", func(t *testing.T) {
			getResult, _ := callTool(t, s, "get_project", map[string]any{
				"project_id": createdID,
			})

			require.True(t, getResult.IsError, "get_project should return an error for a deleted project")
		})
	})

	t.Run("returns an error for a duplicate project name", func(t *testing.T) {
		dupName := projectName + "-dup"

		firstResult, firstText := callTool(t, s, "create_project", map[string]any{
			"terraform_org_name": orgName,
			"project_name":       dupName,
		})
		require.False(t, firstResult.IsError, "first create_project should succeed")

		firstID := gjson.Get(firstText, "project_id").String()
		t.Cleanup(func() { deleteProject(t, s, firstID) })

		// A second project with the same name in the same org should be rejected.
		dupResult, _ := callTool(t, s, "create_project", map[string]any{
			"terraform_org_name": orgName,
			"project_name":       dupName,
		})
		assert.True(t, dupResult.IsError, "create_project should return an error when a project with the same name already exists")
	})

	t.Run("returns an error when deleting a non-existent project_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "delete_project", map[string]any{
			"project_id": "prj-doesnotexist000",
		})

		require.True(t, result.IsError, "delete_project should return an error for a non-existent project_id")
		assert.Contains(t, resultText, "prj-doesnotexist000", "error message should reference the unknown project_id")
	})
}
