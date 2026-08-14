package terraform

import (
	"errors"
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestListStacks(t *testing.T) {
	client := tfeClient(t)
	requireStacksEntitlement(t, client)

	// Create a temporary project to scope the stack under.
	projectName := randomName("project-")
	project, err := client.Projects.Create(t.Context(), tfeOrgName, tfe.ProjectCreateOptions{
		Name: projectName,
	})
	require.NoError(t, err, "setup: failed to create test project via TFE API")
	defer client.Projects.Delete(t.Context(), project.ID)

	// Create a stack in the temporary project — no VCS repo needed for CLI-sourced stacks.
	// If the API returns a 404 the Stacks feature is not yet active for this org; skip cleanly.
	stackName := randomName("stack-")
	stack, err := client.Stacks.Create(t.Context(), tfe.StackCreateOptions{
		Name:    stackName,
		Project: &tfe.Project{ID: project.ID},
	})
	if errors.Is(err, tfe.ErrResourceNotFound) {
		t.Skipf("Stacks feature is not active for organization %q (resource not found)", tfeOrgName)
	}
	require.NoError(t, err, "setup: failed to create test stack via TFE API")
	defer client.Stacks.ForceDelete(t.Context(), stack.ID)

	s := newTestingSession(t)
	defer s.Close()

	t.Run("list all stacks returns the created stack", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_stacks", map[string]any{
			"terraform_org_name": tfeOrgName,
		})

		require.False(t, result.IsError, "list_stacks should not return an error")
		require.NotEmpty(t, resultText, "list_stacks should return a non-empty response")

		assert.NotEqual(t, 0, int(gjson.Get(resultText, "items.#").Int()), "list_stacks should return at least one stack")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.ID").String(), "each stack item should contain an ID")
		assert.NotEmpty(t, gjson.Get(resultText, "items.0.name").String(), "each stack item should contain a name")
		assert.True(t, gjson.Get(resultText, "items.0.description").Exists(), "each stack item should contain a description field")
	})

	t.Run("filter by search_query matching the stack name", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_stacks", map[string]any{
			"terraform_org_name": tfeOrgName,
			"search_query":       stackName,
		})

		require.False(t, result.IsError, "list_stacks with a matching search_query should not return an error")
		assert.Equal(t, stackName, gjson.Get(resultText, "items.0.name").String(), "search_query result should contain the matching stack")
	})

	t.Run("filter by project_id returns only stacks in that project", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_stacks", map[string]any{
			"terraform_org_name": tfeOrgName,
			"project_id":         project.ID,
		})

		require.False(t, result.IsError, "list_stacks with a valid project_id should not return an error")
		assert.Equal(t, 1, int(gjson.Get(resultText, "items.#").Int()), "project_id filter should return exactly the one stack in the project")
		assert.Equal(t, stack.ID, gjson.Get(resultText, "items.0.ID").String(), "filtered result should reference the created stack ID")
	})

	t.Run("search_query with no match returns an error", func(t *testing.T) {
		result, _ := callTool(t, s, "list_stacks", map[string]any{
			"terraform_org_name": tfeOrgName,
			"search_query":       "zzz-no-stack-will-ever-match-this-query-xyz",
		})

		assert.True(t, result.IsError, "list_stacks with a non-matching search_query should return an error")
	})

	t.Run("non-existent org returns an error", func(t *testing.T) {
		result, _ := callTool(t, s, "list_stacks", map[string]any{
			"terraform_org_name": randomName("org-"),
		})

		assert.True(t, result.IsError, "list_stacks with a non-existent org should return an error")
	})
}

func TestGetStackDetails(t *testing.T) {
	client := tfeClient(t)
	requireStacksEntitlement(t, client)

	// Create a temporary project and stack to look up.
	projectName := randomName("project-")
	project, err := client.Projects.Create(t.Context(), tfeOrgName, tfe.ProjectCreateOptions{
		Name: projectName,
	})
	require.NoError(t, err, "setup: failed to create test project via TFE API")
	defer client.Projects.Delete(t.Context(), project.ID)

	stackName := randomName("stack-")
	stack, err := client.Stacks.Create(t.Context(), tfe.StackCreateOptions{
		Name:    stackName,
		Project: &tfe.Project{ID: project.ID},
	})
	if errors.Is(err, tfe.ErrResourceNotFound) {
		t.Skipf("Stacks feature is not active for organization %q (resource not found)", tfeOrgName)
	}
	require.NoError(t, err, "setup: failed to create test stack via TFE API")
	defer client.Stacks.ForceDelete(t.Context(), stack.ID)

	s := newTestingSession(t)
	defer s.Close()

	t.Run("returns details for a valid stack_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_stack_details", map[string]any{
			"stack_id": stack.ID,
		})

		require.False(t, result.IsError, "get_stack_details should not return an error")
		require.NotEmpty(t, resultText, "get_stack_details should return a non-empty response")

		assert.Equal(t, stack.ID, gjson.Get(resultText, "data.id").String(), "response should echo back the requested stack ID")
		assert.Equal(t, stackName, gjson.Get(resultText, "data.attributes.name").String(), "response should contain the stack name")
		assert.True(t, gjson.Get(resultText, "data.attributes.description").Exists(), "response should contain the description field")
		assert.True(t, gjson.Get(resultText, "data.attributes.created-at").Exists(), "response should contain the created-at field")
	})

	t.Run("returns an error for a non-existent stack_id", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_stack_details", map[string]any{
			"stack_id": "st-0000000000dead",
		})

		require.True(t, result.IsError, "get_stack_details should return an error for a non-existent stack_id")
		assert.Contains(t, resultText, "st-0000000000dead", "error message should reference the unknown stack_id")
	})
}
