// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retiredPhrases are wordings the shared vocabulary (ADR 0009) replaced. They
// let one concept be named two ways, which confused agents and users.
var retiredPhrases = []string{
	"agent's workspace", "existing directory they approve", "approved empty directory",
	"which directory to use", "directory the user chose", "chosen directory", "chosen local directory",
	"destination workspace", "destination managed", "destination provider", "destination schema",
	"a local directory with the terraform cli", "optional and worthwhile",
}

func importToolTexts(t *testing.T) map[string]string {
	t.Helper()
	texts := map[string]string{
		"importToolInstructions":      strings.Join(importToolInstructions, "\n"),
		"importDownloadInstructions":  strings.Join(importDownloadInstructions, "\n"),
		"importAgentSchemaNextAction": importAgentSchemaNextAction,
		"importGuideOnlyNextAction":   importGuideOnlyNextAction("reason"),
		"importAuthoringQuestion":     importAuthoringDirectoryQuestion,
		"importBlankAuthoringRule":    importBlankAuthoringDirectoryRule,
		"importArchiveRootRule":       importArchiveRootRule,
		"importConfigurationRootStop": importConfigurationRootStop,
		"importConfirmationRule":      importConfirmationRule,
		"importClosingRule":           importClosingRule,
		"importScratchRule":           importScratchRule,
		"importValidationRule":        importValidationRule,
		"importSensitiveFileRule":     importSensitiveFileRule,
	}
	for _, tool := range []struct{ name, text string }{
		{"prepare_import", PrepareImportDefinition().Description},
		{"get_import_configuration_download", GetImportConfigurationDownloadDefinition().Description},
		{"create_import_cv", CreateImportCVDefinition().Description},
		{"create_import_run", CreateImportRunDefinition().Description},
		{"verify_import_plan", VerifyImportPlanDefinition().Description},
	} {
		texts[tool.name] = tool.text
	}
	return texts
}

func TestImportToolTextUsesSharedVocabulary(t *testing.T) {
	for name, text := range importToolTexts(t) {
		lower := strings.ToLower(text)
		for _, phrase := range retiredPhrases {
			assert.NotContains(t, lower, phrase, "%s uses retired wording", name)
		}
		assert.NotContains(t, lower, "route 1", name)
		assert.NotContains(t, lower, "route 2", name)
	}
}

func TestImportToolTextStatesVocabularyRules(t *testing.T) {
	q := importAuthoringDirectoryQuestion
	for _, want := range []string{"existing empty directory", "new directory", "never overwrite", "git", "do not choose one yourself", "persists"} {
		assert.Contains(t, q, want)
	}
	for _, want := range []string{"no path stripping", "no wrapper folder", "same relative paths", "authoring directory root"} {
		assert.Contains(t, importArchiveRootRule, want)
	}
	for _, want := range []string{".env", ".envrc", "*.tfstate", ".terraform/", ".git/", "plan as creates", "explicit file list", "backend or cloud block"} {
		assert.Contains(t, importBlankAuthoringDirectoryRule, want)
	}
	for _, want := range []string{"target workspace (organization/name)", "authoring directory path", "create_import_cv", "create_import_run"} {
		assert.Contains(t, importConfirmationRule, want)
	}
	for _, want := range []string{"scratch directory", "never the authoring directory", "Never delete the authoring directory"} {
		assert.Contains(t, importScratchRule+importValidationRule, want)
	}
	assert.Contains(t, importValidationRule, "-backend=false")
	assert.Contains(t, importClosingRule, "cannot download")
	assert.Contains(t, importSensitiveFileRule, "silently")
	assert.Contains(t, importConfigurationRootStop, "Do not ask for an authoring directory")

	assert.Contains(t, CreateImportCVDefinition().Description, importConfirmationRule)
	assert.Contains(t, CreateImportCVDefinition().Description, importValidationRule)
	assert.Contains(t, CreateImportCVDefinition().Description, importArchiveRootRule)
	assert.Contains(t, strings.Join(importDownloadInstructions, " "), importArchiveRootRule)
	assert.Contains(t, strings.Join(importToolInstructions, " "), importScratchRule)
	assert.Contains(t, importAgentSchemaNextAction, importAuthoringDirectoryQuestion)
}

func TestPrepareImportAsksForAuthoringDirectoryWithConfirmationNamingBoth(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	assert.Contains(t, out.NextAction, importAuthoringDirectoryQuestion)
	assert.Contains(t, out.NextAction, importArchiveRootRule)
	assert.Contains(t, out.NextAction, importConfirmationRule)
	assert.NotContains(t, out.Notes, "configuration_root_setting_unsupported")
}

func TestPrepareImportBlankTargetGetsFileListRules(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "ready_for_authoring", out.Status, out.Diagnostics)
	assert.Contains(t, out.NextAction, importBlankAuthoringDirectoryRule)
	assert.Contains(t, out.NextAction, importConfirmationRule)
	assert.NotContains(t, out.NextAction, importAuthoringDirectoryQuestion, "a blank target downloads nothing")
}

func TestPrepareImportStopsEarlyForConfigurationRootSetting(t *testing.T) {
	f := importBackendFixture(t)
	setWorkspaceAttribute(t, f, "working-directory", "environments/production")
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.NotNil(t, out.Carry, "candidates are still returned")
	assert.Contains(t, out.Notes, "configuration_root_setting_unsupported")
	assert.Contains(t, out.NextAction, importConfigurationRootStop)
	assert.NotContains(t, out.NextAction, "Ask the user where the authoring directory")
	assert.NotContains(t, out.NextAction, importConfirmationRule)
}

func TestCarryBlockUsesTargetNotDestination(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	raw, err := json.Marshal(out.Carry)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"target":`)
	assert.NotContains(t, string(raw), "destination")
}
