// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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
		"importNewWorkspaceQuestion":  importNewWorkspaceDirectoryQuestion,
		"importNewWorkspaceUpload":    importNewWorkspaceUploadRule,
		"importBlankAuthoringRule":    importBlankAuthoringDirectoryRule,
		"importArchiveRootRule":       importArchiveRootRule,
		"importConfigurationRootStop": importConfigurationRootStop,
		"importConfirmationRule":      importConfirmationRule,
		"importClosingRule":           importClosingRule,
		"importSourceVersionRule":     importSourceVersionRule,
		"importDeleteDirectoryRule":   importDeleteDirectoryRule,
		"importValidationRule":        importValidationRule,
		"importSensitiveFileRule":     importSensitiveFileRule,
		"importAdaptationGuidance":    importAdaptationGuidance,
		"importArchiveURLRule":        importArchiveURLRule,
		"importKeepResultRule":        importKeepResultRule,
		"importCandidateFieldsRule":   importCandidateFieldsRule,
		"importModuleTargetRule":      importModuleTargetRule,
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
	for _, want := range []string{"existing empty directory", "new directory", "never overwrite", "git", "do not choose one yourself", "including a temporary one for testing", "run the local Terraform checks", "git repository", "unwanted credential leak", "flag anything you find", "may contain secrets"} {
		assert.Contains(t, q, want)
	}
	for _, want := range []string{"no path stripping", "no wrapper folder", "same relative paths", "authoring directory root"} {
		assert.Contains(t, importArchiveRootRule, want)
	}
	for _, want := range []string{".env", ".envrc", "*.tfstate", ".terraform/", ".git/", "plan as creates", "explicit file list", "backend or cloud block", "*.tfvars", "exact file list in the review, before uploading"} {
		assert.Contains(t, importBlankAuthoringDirectoryRule, want)
	}
	for _, want := range []string{"target workspace (organization/name)", "authoring directory path", "create_import_cv", "create_import_run"} {
		assert.Contains(t, importConfirmationRule, want)
	}
	for _, want := range []string{"terraform fmt", "terraform init -backend=false", "terraform validate", "in the authoring directory", importLockFileCheck, "For a new workspace, the lock file init writes is the one you upload", "never use -upgrade", "never upload .terraform/", "Do not run these after the upload"} {
		assert.Contains(t, importValidationRule, want)
	}
	for _, want := range []string{"already has a configuration or state", "different provider version", "lock file you upload", "does not apply to it"} {
		assert.Contains(t, importSourceVersionRule, want)
	}
	assert.Contains(t, importClosingRule, "cannot download")
	assert.Contains(t, importSensitiveFileRule, "silently")
	assert.Contains(t, importConfigurationRootStop, "Do not ask for an authoring directory")

	assert.Contains(t, CreateImportCVDefinition().Description, importConfirmationRule)
	assert.Contains(t, CreateImportCVDefinition().Description, importValidationRule)
	assert.Contains(t, CreateImportCVDefinition().Description, "Follow prepare_import's archive-root rule")
	assert.Contains(t, strings.Join(importDownloadInstructions, " "), importArchiveRootRule)
	assert.Contains(t, strings.Join(importToolInstructions, " "), importSourceVersionRule)
	assert.Contains(t, importAgentSchemaNextAction, importAuthoringDirectoryQuestion)
}

func TestPrepareImportAsksForAuthoringDirectoryWithConfirmationNamingBoth(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	assert.Contains(t, out.NextAction, importAuthoringDirectoryQuestion)
	assert.Contains(t, out.NextAction, "configuration_version_id=current_configuration_version_id")
	assert.Contains(t, out.NextAction, "organization_name and workspace_name")
	assert.Contains(t, strings.Join(out.AgentInstructions, " "), importArchiveRootRule)
	assert.Contains(t, strings.Join(out.AgentInstructions, " "), importConfirmationRule)
	assert.NotContains(t, out.Notes, "configuration_root_setting_unsupported")
}

func TestFollowOnToolDescriptionsLeadWithActualRequiredArguments(t *testing.T) {
	for _, tc := range []struct {
		tool     string
		desc     string
		required []string
	}{
		{"prepare_import", PrepareImportDefinition().Description, []string{"organization_name", "workspace_name", "query_run_id", "selections"}},
		{"get_import_configuration_download", GetImportConfigurationDownloadDefinition().Description, []string{"organization_name", "workspace_name", "prepared_target_workspace_id", "configuration_version_id"}},
		{"create_import_cv", CreateImportCVDefinition().Description, []string{"organization_name", "workspace_name", "prepared_target_workspace_id", "confirm_speculative_run"}},
		{"create_import_run", CreateImportRunDefinition().Description, []string{"organization_name", "workspace_name", "prepared_target_workspace_id", "configuration_version_id", "confirm_speculative_run"}},
		{"verify_import_plan", VerifyImportPlanDefinition().Description, []string{"organization_name", "workspace_name", "run_id", "configuration_version_id"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			first := strings.SplitN(tc.desc, "\n", 2)[0]
			for _, field := range tc.required {
				assert.Contains(t, first, field)
			}
		})
	}
}

func TestNextCallsDownloadPrecedesReviewButCreatesDoNot(t *testing.T) {
	instructions := strings.Join(importToolInstructions, " ")
	assert.Contains(t, instructions, "get_import_configuration_download only after the user chooses the authoring directory")
	assert.Contains(t, instructions, "review the authored archive before using either create template")
	assert.NotContains(t, instructions, "use next_calls only after the user has reviewed")
}

func TestPrepareImportBlankTargetGetsFileListRules(t *testing.T) {
	f, _, _ := blankImportFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "ready_for_authoring", out.Status, out.Diagnostics)
	assert.Contains(t, out.NextAction, importBlankAuthoringDirectoryRule)
	assert.Contains(t, out.NextAction, "Review with the user before create_import_cv")
	assert.Contains(t, out.NextAction, importNewWorkspaceDirectoryQuestion)
	assert.NotContains(t, out.NextAction, "git work tree", "a new workspace downloads nothing")
	assert.NotContains(t, out.NextAction, importAuthoringDirectoryQuestion, "a blank target downloads nothing")
	assert.Contains(t, strings.Join(out.AgentInstructions, " "), importBlankAuthoringDirectoryRule)
}

func TestPrepareNextCallsAreTargetScopedAndNotPreauthorized(t *testing.T) {
	for _, tc := range []struct {
		route   string
		fixture func(*testing.T) *importBackendTest
	}{
		{"existing", importBackendFixture},
		{"agent_schema", func(t *testing.T) *importBackendTest { return schemaFallbackFixture(t, "uploaded") }},
		{"blank", func(t *testing.T) *importBackendTest { f, _, _ := blankImportFixture(t); return f }},
	} {
		t.Run(tc.route, func(t *testing.T) {
			f := tc.fixture(t)
			if tc.route == "existing" {
				separateImportSource(t, f, "fixture-org")
				path := "/api/v2/workspaces/ws-fixture/current-state-version"
				f.responses[path] = []byte(strings.Replace(string(f.responses[path]), `"serial":42`, `"serial":0`, 1))
			}
			out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
			require.NotEqual(t, "blocked", out.Status, out.Diagnostics)
			if tc.route == "existing" {
				assert.Equal(t, "ws-search", out.SourceWorkspaceID)
			}
			require.NotNil(t, out.NextCalls)
			cv, run := out.NextCalls.CreateCV, out.NextCalls.CreateRunBase
			for _, args := range []map[string]any{cv, run} {
				assert.Equal(t, "fixture-org", args["organization_name"])
				assert.Equal(t, "import-root", args["workspace_name"])
				assert.Equal(t, "ws-fixture", args["prepared_target_workspace_id"])
				assert.Equal(t, false, args["confirm_speculative_run"])
			}
			assert.NotContains(t, run, "configuration_version_id")
			if tc.route == "blank" {
				assert.Empty(t, out.NextCalls.Download)
				assert.NotContains(t, cv, "baseline_cv_id")
				assert.NotContains(t, cv, "baseline_state_id")
				assert.NotContains(t, cv, "baseline_state_serial")
			} else {
				assert.Equal(t, out.Baseline.ConfigurationVersionID, cv["baseline_cv_id"])
				assert.Equal(t, out.Baseline.StateVersionID, cv["baseline_state_id"])
				assert.Equal(t, *out.Baseline.StateSerial, cv["baseline_state_serial"])
				if tc.route == "existing" {
					assert.Equal(t, int64(0), cv["baseline_state_serial"])
				}
				assert.Equal(t, cv["baseline_state_id"], run["baseline_state_id"])
				dl := out.NextCalls.Download
				assert.Equal(t, "fixture-org", dl["organization_name"])
				assert.Equal(t, "import-root", dl["workspace_name"])
				assert.Equal(t, "ws-fixture", dl["prepared_target_workspace_id"])
				version := out.CurrentConfigurationVersionID
				if tc.route == "agent_schema" {
					version = out.StateRunConfigurationVersionID
				}
				assert.Equal(t, version, dl["configuration_version_id"])
				assert.NotContains(t, dl, "current_configuration_version_id")
				for _, field := range GetImportConfigurationDownloadDefinition().InputSchema.Required {
					assert.Contains(t, dl, field)
				}
			}
			for _, tool := range []struct {
				definition mcp.Tool
				args       map[string]any
			}{
				{CreateImportCVDefinition(), cv}, {CreateImportRunDefinition(), run},
			} {
				for _, field := range tool.definition.InputSchema.Required {
					if field == "configuration_version_id" {
						continue
					} // created CV is not known at preparation
					assert.Contains(t, tool.args, field)
				}
			}
		})
	}
}

func TestPrepareImportExistingConfigurationOmitsNewWorkspaceRule(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	joined := strings.Join(out.AgentInstructions, " ")
	assert.NotContains(t, joined, "For this new workspace")
	assert.NotContains(t, joined, importBlankAuthoringDirectoryRule)
	assert.Contains(t, joined, importArchiveRootRule)
	assert.Contains(t, joined, importSensitiveFileRule)
	assert.Contains(t, joined, importSourceVersionRule)
	assert.NotContains(t, strings.Join(importToolInstructions, " "), "For this new workspace")
}

func TestPrepareImportStopsEarlyForConfigurationRootSetting(t *testing.T) {
	f := importBackendFixture(t)
	setWorkspaceAttribute(t, f, "working-directory", "environments/production")
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "blocked", out.Status)
	assert.Nil(t, out.Carry, "no candidates should be offered for an ineligible target")
	assert.Contains(t, out.Diagnostics, "configuration_root_setting_unsupported")
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
	verifyDone := verifyNextAction(importVerified{Selected: 1})
	for _, tc := range []struct {
		name, sentence string
		surfaces       map[string]string
	}{
		{"secret files", importSecretFilesRule, map[string]string{"prepare instructions": instructions, "download instructions": download, "guide-only": guide}},
		{"nothing created", importNothingCreated, map[string]string{"guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"lock file check", importLockFileCheck, map[string]string{"guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"never apply", importNeverApplyRule, map[string]string{"guide-only": guide, "agent schema": importAgentSchemaNextAction}},
		{"locked provider version", importLockedProviderVersionRule, map[string]string{"prepare instructions": instructions}},
		{"provider mismatch", importProviderMismatchRule, map[string]string{"prepare instructions": instructions, "agent schema": importAgentSchemaNextAction}},
		{"adaptation guidance", importAdaptationGuidance, map[string]string{"prepare instructions": instructions}},
		{"archive root", importArchiveRootRule, map[string]string{"prepare instructions": instructions, "download instructions": download}},
		{"confirmation", importConfirmationRule, map[string]string{"prepare instructions": instructions, "create_import_cv": create}},
		{"signed URL", importArchiveURLRule, map[string]string{"download instructions": download}},
		{"delete directory", importDeleteDirectoryRule, map[string]string{"prepare instructions": instructions, "create_import_cv": create}},
		{"source version", importSourceVersionRule, map[string]string{"prepare instructions": instructions}},
		{"validation", importValidationRule, map[string]string{"prepare instructions": instructions, "create_import_cv": create}},
		{"closing", importClosingRule, map[string]string{"verify next action": verifyDone}},
		{"existing question closing", "should not be committed as is", map[string]string{"existing question": importAuthoringDirectoryQuestion}},
		{"new question closing", "ready to review and commit to their own source", map[string]string{"new question": importNewWorkspaceDirectoryQuestion}},
		{"new workspace upload", importNewWorkspaceUploadRule, map[string]string{"create_import_cv": create}},
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

func TestDirectoryRulesStaySimple(t *testing.T) {
	assert.Equal(t, "Do not delete or clean up the authoring directory without asking the user first.", importDeleteDirectoryRule)
	for name, text := range importToolTexts(t) {
		lower := strings.ToLower(text)
		for _, banned := range []string{"never delete", "scratch", "temporary area", "persistent location", "remind the user to save", "only a backup", "ephemeral"} {
			assert.NotContains(t, lower, banned, name)
		}
	}
}

func TestNewWorkspaceQuestionOmitsTheGitWarningAndAllowsExistingFiles(t *testing.T) {
	for _, want := range []string{"new (no configuration and no state)", "nothing is downloaded", "already holds their Terraform files", "Tell the user", "do not choose one yourself", "including a temporary one for testing"} {
		assert.Contains(t, importNewWorkspaceDirectoryQuestion, want)
	}
	for _, unwanted := range []string{"git work tree", "must be an existing empty directory", "Never extract into a directory"} {
		assert.NotContains(t, importNewWorkspaceDirectoryQuestion, unwanted)
	}
	for _, want := range []string{"git repository", "must be an existing empty directory", "Never extract into a directory"} {
		assert.Contains(t, importAuthoringDirectoryQuestion, want)
	}
	// Each variant carries its own guidance for the end of the workflow; the
	// closing in verify_import_plan is the same for both.
	assert.Contains(t, importAuthoringDirectoryQuestion, "should not be committed as is")
	assert.NotContains(t, importNewWorkspaceDirectoryQuestion, "should not be committed as is")
	assert.Contains(t, importNewWorkspaceDirectoryQuestion, "not to commit .terraform/ or any state files")
	assert.Contains(t, importNewWorkspaceDirectoryQuestion, "check .tfvars files for secrets first")
	assert.Contains(t, importAuthoringDirectoryQuestion, "If the user already named a path, check it is empty or new first")
	assert.NotContains(t, importClosingRule, "should not be committed as is")
	assert.NotContains(t, importClosingRule, "sensitive files")
	assert.False(t, forbiddenNextActionWords.MatchString(importClosingRule), "no safe, approved or apply wording")
}

func TestNewWorkspaceUploadRuleIsAtTheUploadStep(t *testing.T) {
	for _, want := range []string{"key or credential files", "*.pem", "*.key", "*.tfbackend", "symlinks pointing out of the directory", "inline credentials", "provider keys", "upload only the file list the user confirmed"} {
		assert.Contains(t, importNewWorkspaceUploadRule, want)
	}
	// The question-time rule keeps the common exclusions and the exact file list in the review.
	for _, want := range []string{".env", ".envrc", "credential files", "*.tfstate", ".terraform/", ".git/", "exact file list in the review, before uploading"} {
		assert.Contains(t, importBlankAuthoringDirectoryRule, want)
	}
	for _, notThere := range []string{"inline credentials", "*.pem", "symlinks"} {
		assert.NotContains(t, importBlankAuthoringDirectoryRule, notThere)
	}
	assert.Contains(t, CreateImportCVDefinition().Description, importNewWorkspaceUploadRule)
}

func TestCreateImportCVReturnsTheUploadRuleOnlyForANewWorkspace(t *testing.T) {
	importExecutionFixture(t)
	existing := callCreate(t, false, createArgs(nil))
	require.Equal(t, "awaiting_agent_upload", existing.Status, existing.Diagnostics)
	assert.NotContains(t, existing.UploadInstructions, importNewWorkspaceUploadRule, "an existing workspace's upload is not a new-workspace upload")

	blankImportFixture(t)
	blank := callCreate(t, false, createArgs(map[string]any{"baseline_cv_id": nil, "baseline_state_id": nil, "baseline_state_serial": nil}))
	require.Equal(t, "awaiting_agent_upload", blank.Status, blank.Diagnostics)
	assert.Contains(t, blank.UploadInstructions, importNewWorkspaceUploadRule)
}

func TestSensitiveInformationIsFlaggedNotAvoidedByPath(t *testing.T) {
	q := importAuthoringDirectoryQuestion
	assert.Contains(t, q, "Any path is fine, including one inside a git repository")
	assert.NotContains(t, q, "git work tree")
	for _, want := range []string{"inline secrets", "Check without printing secret values", "flag it to the user"} {
		assert.Contains(t, importSensitiveFileRule, want)
	}
}

func TestKeepGeneratedBlocksRuleIsProviderNeutralAndFirst(t *testing.T) {
	for _, want := range []string{"Keep the generated resource and import blocks as returned", "identity form of the import block", "scoping values", "region, project, location, subscription or account", "Do not replace an identity import with an id import unless identity_support", "id import can encode scope", "neither source nor correctness of the resolved scope follows from the ID alone", "describes the target workspace's last plan"} {
		assert.Contains(t, importKeepGeneratedRule, want)
	}
	assert.Equal(t, importKeepGeneratedRule, importToolInstructions[0])
	assert.Contains(t, importConfirmationRule, "from an identity import to an id import")
	for _, next := range []string{
		importPreparedNextAction(&importPrepared{HasCurrentConfiguration: true}, false, false),
		importPreparedNextAction(&importPrepared{}, false, false),
	} {
		assert.True(t, strings.HasPrefix(next, importKeepGeneratedShort) || strings.Contains(next, importKeepGeneratedShort))
		assert.Less(t, strings.Index(next, importKeepGeneratedShort), strings.Index(next, "authoring directory"))
	}
}

func TestTargetScopeSeparatesTargetShapeFromQueryRun(t *testing.T) {
	assert.Equal(t, importKeepGeneratedRule, importToolInstructions[0])
	assert.Equal(t, importTargetScopeRule, importToolInstructions[1])
	for _, want := range []string{"describe the target workspace", "managed_schema, identity_schema", "describe the QueryRun", "no-code QueryRun", "can differ from the target's"} {
		assert.Contains(t, importTargetScopeRule, want)
	}
	existing := importPreparedNextAction(&importPrepared{HasCurrentConfiguration: true}, false, false)
	assert.Contains(t, existing, importTargetScopeShort)
	assert.Less(t, strings.Index(existing, importTargetScopeShort), strings.Index(existing, "authoring directory"))
	assert.NotContains(t, importPreparedNextAction(&importPrepared{}, false, false), importTargetScopeShort, "a new workspace has no target schema")
	assert.NotContains(t, importAgentSchemaNextAction, importTargetScopeShort)

	d := PrepareImportDefinition().Description
	for _, want := range []string{"target schema reports identity", "not that the proposed block or values are valid", "describe the QueryRun", "could not be read", "supported means"} {
		assert.Contains(t, d, want)
	}
	assert.Contains(t, importProviderMismatchRule, "is expected")
	assert.Contains(t, importLockedProviderVersionRule, "configuration_baseline_relation")
}

func TestPrepareImportAsksForTheWholeResultAndNamesCandidateFields(t *testing.T) {
	assert.Equal(t, importKeepResultRule, importToolInstructions[2])
	assert.Equal(t, importCandidateFieldsRule, importToolInstructions[3])
	for _, want := range []string{"complete prepare_import response", "unchanged carry block", "every selected candidate", "not only the first or a summary"} {
		assert.Contains(t, importKeepResultRule, want)
	}
	for _, want := range []string{"resource_object", "as the Search provider observed them", "Search-generated HCL drafts", "describe the QueryRun's source resource"} {
		assert.Contains(t, importCandidateFieldsRule, want)
	}
	d := PrepareImportDefinition().Description
	for _, want := range []string{"complete response", "not only the first candidate", "resource_object (the attributes the Search provider observed", "Search-generated HCL drafts"} {
		assert.Contains(t, d, want)
	}
	for _, next := range []string{
		importPreparedNextAction(&importPrepared{HasCurrentConfiguration: true}, false, false),
		importPreparedNextAction(&importPrepared{}, false, false),
	} {
		assert.Contains(t, next, importKeepResultShort)
		assert.Less(t, strings.Index(next, importKeepResultShort), strings.Index(next, "authoring directory"))
	}

	// The tool text guides what to keep. It does not tell the agent to avoid
	// or to repeat the call, and the retired "once per selection" is gone.
	for name, text := range map[string]string{
		"prepare_import description": d,
		"prepare_import guidance":    strings.Join(importToolInstructions, " "),
		"verify_import_plan":         VerifyImportPlanDefinition().Description,
	} {
		lower := strings.ToLower(text)
		assert.NotContains(t, lower, "once per selection", name)
		assert.NotContains(t, lower, "retry", name)
		assert.NotContains(t, lower, "do not call prepare_import again", name)
	}
}
