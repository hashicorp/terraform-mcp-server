package terraform

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

//go:embed testdata/run_test.tf
var runTestConfiguration string

func TestCreateRunLockedWorkspace(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)
	workspaceName := randomName("run-test-")
	workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{Name: &workspaceName})
	require.NoError(t, err, "failed to create test workspace")
	defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

	lockReason := "Test create_run with a locked workspace"
	_, err = client.Workspaces.Lock(t.Context(), workspace.ID, tfe.WorkspaceLockOptions{Reason: &lockReason})
	require.NoError(t, err, "failed to lock test workspace")
	defer client.Workspaces.ForceUnlock(t.Context(), workspace.ID)

	result, resultText := callTool(t, s, "create_run", map[string]any{
		"terraform_org_name": tfeOrgName,
		"workspace_name":     workspaceName,
	})

	assert.True(t, result.IsError, "create_run should return an error for a locked workspace")
	assert.Equal(t, fmt.Sprintf(`workspace %q is locked and cannot accept new runs. Use the force_unlock_workspace tool to unlock first`, workspaceName), resultText)
}

func TestRunLifecycle(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	// Create an isolated remote workspace for the run lifecycle.
	client := tfeClient(t)
	workspaceName := randomName("run-test-")
	executionMode := "remote"

	workspace, err := client.Workspaces.Create(t.Context(), tfeOrgName, tfe.WorkspaceCreateOptions{
		Name:          &workspaceName,
		ExecutionMode: &executionMode,
		AutoApply:     tfe.Bool(false),
	})
	require.NoError(t, err, "failed to create test workspace")
	defer client.Workspaces.DeleteByID(t.Context(), workspace.ID)

	// Upload the configuration without automatically queuing a run.
	uploadRunTestConfiguration(t, client, workspace.ID)

	const (
		runMessage  = "Created by terraform-mcp-server integration tests"
		commentBody = "Run comment created by integration tests"
	)

	var runID string
	t.Run("Create run", func(t *testing.T) {
		result, resultText := callTool(t, s, "create_run", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     workspaceName,
			"run_type":           "plan_and_apply",
			"message":            runMessage,
		})
		require.False(t, result.IsError, "create_run should not return an error")
		require.NotEmpty(t, resultText, "create_run response must not be empty")

		runID = gjson.Get(resultText, "data.id").String()
		require.NotEmpty(t, runID, "Tool response should include a run ID")
		toolRunMessage := gjson.Get(resultText, "data.attributes.message").String()
		assert.Equal(t, runMessage, toolRunMessage)

		// Verify against the TFE API directly
		run, err := client.Runs.Read(t.Context(), runID)
		require.NoError(t, err, "create run tool reported as created but produced an error when reading")
		assert.Equal(t, run.ID, runID)
		assert.Equal(t, run.Message, toolRunMessage)
		require.NotNil(t, run.Workspace)
		assert.Equal(t, workspace.ID, run.Workspace.ID)
	})

	t.Run("List runs", func(t *testing.T) {
		result, resultText := callTool(t, s, "list_runs", map[string]any{
			"terraform_org_name": tfeOrgName,
			"workspace_name":     workspaceName,
		})
		require.False(t, result.IsError, "list_runs should not return an error")
		require.NotEmpty(t, resultText, "list_runs response must not be empty")

		listedRunID := gjson.Get(resultText, "items.0.id").String()
		require.NotEmpty(t, listedRunID, "Tool response should include a run ID")
		assert.Equal(t, runID, listedRunID)
		listedRunMessage := gjson.Get(resultText, "items.0.message").String()
		listedWorkspaceName := gjson.Get(resultText, "items.0.workspace_name").String()

		// Verify against the TFE API directly
		runs, err := client.Runs.List(t.Context(), workspace.ID, nil)
		require.NoError(t, err)
		require.Len(t, runs.Items, 1, "the dedicated workspace should contain one run")
		assert.Equal(t, runs.Items[0].ID, listedRunID)
		assert.Equal(t, runs.Items[0].Message, listedRunMessage)
		require.NotNil(t, runs.Items[0].Workspace)
		assert.Equal(t, runs.Items[0].Workspace.Name, listedWorkspaceName)
	})

	t.Run("Get run details", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_run_details", map[string]any{"run_id": runID})
		require.False(t, result.IsError, "get_run_details should not return an error")
		require.NotEmpty(t, resultText, "get_run_details response must not be empty")
		toolRunID := gjson.Get(resultText, "data.id").String()
		toolRunMessage := gjson.Get(resultText, "data.attributes.message").String()

		// Verify against the TFE API directly
		run, err := client.Runs.Read(t.Context(), runID)
		require.NoError(t, err)
		assert.Equal(t, run.ID, toolRunID)
		assert.Equal(t, run.Message, toolRunMessage)
	})

	t.Run("Get run comments", func(t *testing.T) {
		comment, err := client.Comments.Create(t.Context(), runID, tfe.CommentCreateOptions{Body: commentBody})
		require.NoError(t, err, "failed to create a run comment with the TFE client")

		result, resultText := callTool(t, s, "get_run_comments", map[string]any{"run_id": runID})
		require.False(t, result.IsError, "get_run_comments should not return an error")
		require.NotEmpty(t, resultText, "get_run_comments response must not be empty")

		listedCommentID := gjson.Get(resultText, "items.0.id").String()
		listedCommentBody := gjson.Get(resultText, "items.0.body").String()

		comments, err := client.Comments.List(t.Context(), runID)
		require.NoError(t, err)
		require.Len(t, comments.Items, 1, "the run should contain one comment")
		assert.Equal(t, comment.ID, comments.Items[0].ID)
		assert.Equal(t, comments.Items[0].ID, listedCommentID)
		assert.Equal(t, comments.Items[0].Body, listedCommentBody)
	})

	// create_run starts planning asynchronously; wait until the run can be applied.
	plannedRun := waitForRun(t, client, runID, "become confirmable", func(run *tfe.Run) bool {
		return run.Actions != nil && run.Actions.IsConfirmable
	})
	require.NotNil(t, plannedRun.Plan, "the planned run should have a plan")
	planID := plannedRun.Plan.ID
	require.NotEmpty(t, planID, "the planned run should have a plan ID")

	t.Run("Get plan details", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_plan_details", map[string]any{"plan_id": planID})
		require.False(t, result.IsError, "get_plan_details should not return an error")
		require.NotEmpty(t, resultText, "get_plan_details response must not be empty")
		toolPlanID := gjson.Get(resultText, "data.id").String()
		toolPlanStatus := gjson.Get(resultText, "data.attributes.status").String()
		toolPlanHasChanges := gjson.Get(resultText, "data.attributes.has-changes").Bool()

		plan, err := client.Plans.Read(t.Context(), planID)
		require.NoError(t, err)
		assert.Equal(t, plan.ID, toolPlanID)
		assert.Equal(t, string(plan.Status), toolPlanStatus)
		assert.Equal(t, plan.HasChanges, toolPlanHasChanges)
		assert.Equal(t, tfe.PlanFinished, plan.Status)
		assert.True(t, plan.HasChanges)
	})

	t.Run("Get plan logs", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_plan_logs", map[string]any{"plan_id": planID})
		require.False(t, result.IsError, "get_plan_logs should not return an error")
		require.NotEmpty(t, resultText, "get_plan_logs response must not be empty")
		assert.Contains(t, resultText, "terraform_data.run_test")

		logReader, err := client.Plans.Logs(t.Context(), planID)
		require.NoError(t, err)
		directLogs, err := io.ReadAll(logReader)
		require.NoError(t, err)
		assert.Equal(t, string(directLogs), resultText)
	})

	t.Run("Get plan JSON output", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_plan_json_output", map[string]any{"plan_id": planID})
		require.False(t, result.IsError, "get_plan_json_output should not return an error")
		require.NotEmpty(t, resultText, "get_plan_json_output response must not be empty")
		require.True(t, gjson.Valid(resultText), "get_plan_json_output should return valid JSON")
		resourceAction := gjson.Get(resultText, `resource_changes.#(address=="terraform_data.run_test").change.actions.0`).String()
		assert.Equal(t, "create", resourceAction)

		directJSON, err := client.Plans.ReadJSONOutput(t.Context(), planID)
		require.NoError(t, err)
		assert.JSONEq(t, string(directJSON), resultText)
	})

	t.Run("Action run", func(t *testing.T) {
		result, resultText := callTool(t, s, "action_run", map[string]any{
			"run_id":     runID,
			"run_action": "apply",
			"comment":    "Approved by integration tests",
		})
		require.False(t, result.IsError, "action_run should not return an error")
		require.NotEmpty(t, resultText, "action_run response must not be empty")
		// TODO: update this after MCP SKD migration
		assert.Contains(t, resultText, "Run approved and applied successfully")
	})

	// action_run starts applying asynchronously; wait until the apply finishes.
	appliedRun := waitForRun(t, client, runID, "finish applying", func(run *tfe.Run) bool {
		return run.Status == tfe.RunApplied
	})
	assert.Equal(t, appliedRun.ID, runID)
	require.NotNil(t, appliedRun.Apply, "the applied run should have an apply")
	applyID := appliedRun.Apply.ID
	require.NotEmpty(t, applyID, "the applied run should have an apply ID")

	t.Run("Get apply details", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_apply_details", map[string]any{"apply_id": applyID})
		require.False(t, result.IsError, "get_apply_details should not return an error")
		require.NotEmpty(t, resultText, "get_apply_details response must not be empty")
		toolApplyID := gjson.Get(resultText, "data.id").String()
		toolApplyStatus := gjson.Get(resultText, "data.attributes.status").String()

		apply, err := client.Applies.Read(t.Context(), applyID)
		require.NoError(t, err)
		assert.Equal(t, apply.ID, toolApplyID)
		assert.Equal(t, string(apply.Status), toolApplyStatus)
		assert.Equal(t, tfe.ApplyFinished, apply.Status)
	})

	t.Run("Get apply logs", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_apply_logs", map[string]any{"apply_id": applyID})
		require.False(t, result.IsError, "get_apply_logs should not return an error")
		require.NotEmpty(t, resultText, "get_apply_logs response must not be empty")
		assert.Contains(t, resultText, "Apply complete!")

		logReader, err := client.Applies.Logs(t.Context(), applyID)
		require.NoError(t, err)
		directLogs, err := io.ReadAll(logReader)
		require.NoError(t, err)
		assert.Equal(t, string(directLogs), resultText)
	})
}

// packages the HCL as a gzipped tar archive and uploads it to the workspace.
func uploadRunTestConfiguration(t *testing.T, client *tfe.Client, workspaceID string) {
	t.Helper()

	// Package main.tf as gzip(tar(main.tf)) entirely in memory.
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	configuration := []byte(runTestConfiguration)

	require.NoError(t, tarWriter.WriteHeader(&tar.Header{
		Name: "main.tf",
		Mode: 0o600,
		Size: int64(len(configuration)),
	}))
	_, err := tarWriter.Write(configuration)
	require.NoError(t, err)
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	// Create the configuration-version record without auto-queuing a run.
	configurationVersion, err := client.ConfigurationVersions.Create(t.Context(), workspaceID, tfe.ConfigurationVersionCreateOptions{
		AutoQueueRuns: tfe.Bool(false),
	})
	require.NoError(t, err, "failed to create a configuration version")
	require.NoError(t, client.ConfigurationVersions.UploadTarGzip(t.Context(), configurationVersion.UploadURL, &archive), "failed to upload the test configuration")
}

// waitForRun polls TFE until the run reaches the requested condition or times out.
func waitForRun(t *testing.T, client *tfe.Client, runID, condition string, conditionMet func(*tfe.Run) bool) *tfe.Run {
	t.Helper()

	description := fmt.Sprintf("run %s to %s", runID, condition)
	return waitFor(t, 2*time.Minute, description, func(ctx context.Context) (*tfe.Run, error) {
		run, err := client.Runs.ReadWithOptions(ctx, runID, &tfe.RunReadOptions{
			Include: []tfe.RunIncludeOpt{tfe.RunPlan, tfe.RunApply},
		})
		require.NoError(t, err, "failed to poll run with the TFE client")
		if conditionMet(run) {
			return run, nil
		}

		switch run.Status {
		case tfe.RunErrored, tfe.RunCanceled, tfe.RunDiscarded:
			t.Fatalf("run %s reached terminal state %s while waiting for it to %s", runID, run.Status, condition)
		}
		return nil, nil
	})
}
