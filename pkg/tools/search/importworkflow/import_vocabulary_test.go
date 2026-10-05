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
		"importAdaptationGuidance":    importAdaptationGuidance,
		"importArchiveURLRule":        importArchiveURLRule,
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
		for _, phrase := range RetiredImportPhrases() {
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

// A shared sentence must stay on every surface that needs it, so a later edit
// cannot silently drop a rule from one tool.
func TestSharedSentencesStayOnTheirSurfaces(t *testing.T) {
	instructions := strings.Join(importToolInstructions, "\n")
	download := strings.Join(importDownloadInstructions, "\n")
	guide := importGuideOnlyNextAction("reason")
	create := CreateImportCVDefinition().Description
	verifyDone := verifyNextAction(importVerified{Selected: 1, Overall: "no_unintended_changes"})
	for _, tc := range []struct {
		name, sentence string
		surfaces       map[string]string
	}{
		{"secret files", importSecretFilesRule, map[string]string{"prepare instructions": instructions, "download instructions": download, "guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"nothing created", importNothingCreated, map[string]string{"guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"lock file check", importLockFileCheck, map[string]string{"guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"never apply", importNeverApplyRule, map[string]string{"guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"locked provider version", importLockedProviderVersionRule, map[string]string{"prepare instructions": instructions}},
		{"provider mismatch", importProviderMismatchRule, map[string]string{"prepare instructions": instructions, "agent schema": importAgentSchemaNextAction}},
		{"adaptation guidance", importAdaptationGuidance, map[string]string{"prepare instructions": instructions}},
		{"archive root", importArchiveRootRule, map[string]string{"prepare instructions": instructions, "download instructions": download, "create_import_cv": create, "agent schema": importAgentSchemaNextAction}},
		{"confirmation", importConfirmationRule, map[string]string{"prepare instructions": instructions, "create_import_cv": create}},
		{"signed URL", importArchiveURLRule, map[string]string{"download instructions": download}},
		{"closing", importClosingRule, map[string]string{"prepare instructions": instructions, "verify next action": verifyDone}},
		{"authoring directory", importAuthoringDirectoryQuestion, map[string]string{"agent schema": importAgentSchemaNextAction}},
	} {
		for surface, text := range tc.surfaces {
			assert.Contains(t, text, tc.sentence, "%s is missing from %s", tc.name, surface)
		}
	}
}

func TestEveryBlockedNextActionSaysNothingWasCreated(t *testing.T) {
	for _, code := range []string{"workspace_execution_mode_local", "terraform_version_unsupported", "state_producing_run_missing", "target_config_not_downloadable", "schema_source_plan_unavailable", "workspace_ownership_unverified", ""} {
		assert.Contains(t, importBlockedNextAction(code), importNothingCreated, code)
	}
}
