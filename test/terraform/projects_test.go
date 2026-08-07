package terraform

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListAndGetProject(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	projectsResult, projectsText := callTool(t, s, "list_terraform_projects", map[string]any{
		"terraform_org_name": tfeOrgName,
	})
	require.False(t, projectsResult.IsError, "list_terraform_projects should not return an error")
	require.NotEmpty(t, projectsText, "list_terraform_projects should return a non-empty response")

	projectID := gjson.Get(projectsText, "items.0.project_id").String()
	require.NotEmpty(t, projectID, "expected at least one project to be available in org %q", tfeOrgName)

	t.Run("list_terraform_projects returns a non-empty list", func(t *testing.T) {
		assert.NotEqual(t, 0, int(gjson.Get(projectsText, "items.#").Int()), "list_terraform_projects should return at least one project")
		assert.NotEmpty(t, gjson.Get(projectsText, "items.0.project_id").String(), "items should contain project_id")
		assert.NotEmpty(t, gjson.Get(projectsText, "items.0.project_name").String(), "items should contain project_name")
	})

	t.Run("get_project returns details for a valid project_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_project", map[string]any{
			"project_id": projectID,
		})

		require.False(t, result.IsError, "get_project should not return an error")
		require.NotEmpty(t, resultText, "get_project should return a non-empty response")

		assert.Equal(t, projectID, gjson.Get(resultText, "project_id").String(), "response should echo back the requested project_id")
		assert.NotEmpty(t, gjson.Get(resultText, "project_name").String(), "response should contain a project_name")
		assert.True(t, gjson.Get(resultText, "is_unified").Exists(), "response should always contain the is_unified field")
	})

	t.Run("get_project returns an error for a non-existent project_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_project", map[string]any{
			"project_id": "prj-doesnotexist000",
		})

		require.True(t, result.IsError, "get_project should return an error for an unknown project_id")
		assert.Contains(t, resultText, "prj-doesnotexist000", "error message should reference the unknown project_id")
	})
}

func TestCreateProject(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)
	projectName := randomName("project-")

	result, resultText := callTool(t, s, "create_project", map[string]any{
		"terraform_org_name": tfeOrgName,
		"project_name":       projectName,
		"description":        "Created by terraform-mcp-server integration tests",
	})

	require.False(t, result.IsError, "create_project should not return an error")
	require.NotEmpty(t, resultText, "create_project should return a non-empty response")

	createdID := gjson.Get(resultText, "project_id").String()
	require.NotEmpty(t, createdID, "create_project response should contain a project_id")

	// Clean up directly via the TFE client — independent of the tools under test.
	defer client.Projects.Delete(t.Context(), createdID)

	assert.Equal(t, projectName, gjson.Get(resultText, "project_name").String(), "response should echo back the project name")

	// Verify the project exists via the API directly.
	project, err := client.Projects.Read(t.Context(), createdID)
	require.NoError(t, err, "project reported as created but could not be read via the TFE API")
	assert.Equal(t, projectName, project.Name, "project name in TFE API should match the requested name")
	assert.Equal(t, "Created by terraform-mcp-server integration tests", project.Description, "project description in TFE API should match")

	t.Run("returns an error for a duplicate project name", func(t *testing.T) {
		dupResult, _ := callTool(t, s, "create_project", map[string]any{
			"terraform_org_name": tfeOrgName,
			"project_name":       projectName,
		})
		assert.True(t, dupResult.IsError, "create_project should return an error when a project with the same name already exists")
	})
}

func TestDeleteProject(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)

	// Create a project directly via the TFE API — independent of the tool under test.
	projectName := randomName("project-")
	project, err := client.Projects.Create(t.Context(), tfeOrgName, tfe.ProjectCreateOptions{
		Name: projectName,
	})
	require.NoError(t, err, "setup: failed to create project via TFE API")

	// Safety net: if the delete tool fails mid-test, clean up via the API.
	defer client.Projects.Delete(t.Context(), project.ID)

	t.Run("deletes an existing project", func(t *testing.T) {
		result, resultText := callTool(t, s, "delete_project", map[string]any{
			"project_id": project.ID,
		})

		require.False(t, result.IsError, "delete_project should not return an error for an empty project")
		assert.Contains(t, resultText, project.ID, "response should reference the deleted project_id")

		// Confirm the project is gone via the API directly.
		_, err := client.Projects.Read(t.Context(), project.ID)
		assert.Error(t, err, "project should no longer exist in the TFE API after deletion")
	})

	t.Run("returns an error for a non-existent project_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "delete_project", map[string]any{
			"project_id": "prj-doesnotexist000",
		})

		require.True(t, result.IsError, "delete_project should return an error for a non-existent project_id")
		assert.Contains(t, resultText, "prj-doesnotexist000", "error message should reference the unknown project_id")
	})
}
