// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp_terraform

import "time"

// OrganizationResponse represents the API response structure
type OrganizationResponse struct {
	Data  []Organization     `json:"data"`
	Links PaginationLinks    `json:"links"`
	Meta  PaginationMetadata `json:"meta"`
}

// Organization represents a single HCP Terraform organization
type Organization struct {
	ID            string                    `json:"id"`
	Type          string                    `json:"type"`
	Attributes    OrganizationAttributes    `json:"attributes"`
	Relationships OrganizationRelationships `json:"relationships,omitempty"`
	Links         OrganizationLinks         `json:"links,omitempty"`
}

// OrganizationAttributes contains organization details
type OrganizationAttributes struct {
	Name                       string                  `json:"name"`
	Email                      string                  `json:"email"`
	CreatedAt                  time.Time               `json:"created-at"`
	ExternalID                 string                  `json:"external-id"`
	CollaboratorAuthPolicy     string                  `json:"collaborator-auth-policy"`
	SessionTimeout             *int                    `json:"session-timeout"`
	SessionRemember            *int                    `json:"session-remember"`
	PlanExpired                bool                    `json:"plan-expired"`
	PlanExpiresAt              *time.Time              `json:"plan-expires-at"`
	PlanIsTrial                bool                    `json:"plan-is-trial"`
	PlanIsEnterprise           bool                    `json:"plan-is-enterprise"`
	PlanIdentifier             string                  `json:"plan-identifier"`
	CostEstimationEnabled      bool                    `json:"cost-estimation-enabled"`
	Permissions                OrganizationPermissions `json:"permissions"`
	TwoFactorConformant        bool                    `json:"two-factor-conformant"`
	AssessmentsEnforced        bool                    `json:"assessments-enforced"`
	DefaultExecutionMode       string                  `json:"default-execution-mode"`
	FairRunQueuingEnabled      bool                    `json:"fair-run-queuing-enabled"`
	SAMLEnabled                bool                    `json:"saml-enabled"`
	OwnersTeamSAMLRoleID       *string                 `json:"owners-team-saml-role-id"`
	AllowForceDeleteWorkspaces bool                    `json:"allow-force-delete-workspaces"`
}

// OrganizationPermissions contains user permissions for the organization
type OrganizationPermissions struct {
	CanUpdate                bool `json:"can-update"`
	CanDestroy               bool `json:"can-destroy"`
	CanAccessViaTeams        bool `json:"can-access-via-teams"`
	CanCreateModule          bool `json:"can-create-module"`
	CanCreateTeam            bool `json:"can-create-team"`
	CanCreateWorkspace       bool `json:"can-create-workspace"`
	CanManageUsers           bool `json:"can-manage-users"`
	CanManageSubscription    bool `json:"can-manage-subscription"`
	CanManageSSO             bool `json:"can-manage-sso"`
	CanUpdateOAuth           bool `json:"can-update-oauth"`
	CanUpdateSentinel        bool `json:"can-update-sentinel"`
	CanUpdateSSHKeys         bool `json:"can-update-ssh-keys"`
	CanUpdateAPIToken        bool `json:"can-update-api-token"`
	CanTraverse              bool `json:"can-traverse"`
	CanStartTrial            bool `json:"can-start-trial"`
	CanUpdateAgentPools      bool `json:"can-update-agent-pools"`
	CanManageTags            bool `json:"can-manage-tags"`
	CanManageVarsets         bool `json:"can-manage-varsets"`
	CanReadVarsets           bool `json:"can-read-varsets"`
	CanManagePublicProviders bool `json:"can-manage-public-providers"`
	CanCreateProvider        bool `json:"can-create-provider"`
	CanManagePublicModules   bool `json:"can-manage-public-modules"`
	CanCreateProject         bool `json:"can-create-project"`
	CanManageCustomProviders bool `json:"can-manage-custom-providers"`
	CanManageRunTasks        bool `json:"can-manage-run-tasks"`
	CanReadRunTasks          bool `json:"can-read-run-tasks"`
}

// OrganizationRelationships contains related resources
type OrganizationRelationships struct {
	DefaultAgentPool    *RelationshipData `json:"default-agent-pool,omitempty"`
	OAuthTokens         *RelationshipData `json:"oauth-tokens,omitempty"`
	AuthenticationToken *RelationshipData `json:"authentication-token,omitempty"`
	EntitlementSet      *RelationshipData `json:"entitlement-set,omitempty"`
	Subscription        *RelationshipData `json:"subscription,omitempty"`
	DataRetentionPolicy *RelationshipData `json:"data-retention-policy,omitempty"`
}

// RelationshipData represents a relationship to another resource
type RelationshipData struct {
	Data  interface{}       `json:"data,omitempty"`
	Links map[string]string `json:"links,omitempty"`
}

// OrganizationLinks contains API links for the organization
type OrganizationLinks struct {
	Self string `json:"self"`
}

// PaginationLinks contains pagination URLs
type PaginationLinks struct {
	Self  string  `json:"self"`
	First string  `json:"first"`
	Prev  *string `json:"prev"`
	Next  *string `json:"next"`
	Last  string  `json:"last"`
}

// PaginationMetadata contains pagination information
type PaginationMetadata struct {
	Pagination PaginationInfo `json:"pagination"`
}

// PaginationInfo contains detailed pagination data
type PaginationInfo struct {
	CurrentPage int  `json:"current-page"`
	PageSize    int  `json:"page-size"`
	PrevPage    *int `json:"prev-page"`
	NextPage    *int `json:"next-page"`
	TotalPages  int  `json:"total-pages"`
	TotalCount  int  `json:"total-count"`
}

// OrganizationListOptions for query parameters
type OrganizationListOptions struct {
	Query      string `url:"q,omitempty"`
	QueryEmail string `url:"q[email],omitempty"`
	QueryName  string `url:"q[name],omitempty"`
	PageNumber int    `url:"page[number],omitempty"`
	PageSize   int    `url:"page[size],omitempty"`
}

// ====================
// Workspace Types
// ====================

// WorkspaceResponse represents the API response structure for workspaces
type WorkspaceResponse struct {
	Data  []Workspace        `json:"data"`
	Links PaginationLinks    `json:"links"`
	Meta  PaginationMetadata `json:"meta"`
}

// SingleWorkspaceResponse represents the API response structure for a single workspace
type SingleWorkspaceResponse struct {
	Data Workspace `json:"data"`
}

// Workspace represents a single HCP Terraform workspace
type Workspace struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Attributes    WorkspaceAttributes    `json:"attributes"`
	Relationships WorkspaceRelationships `json:"relationships,omitempty"`
	Links         WorkspaceLinks         `json:"links,omitempty"`
}

// WorkspaceAttributes contains workspace details
type WorkspaceAttributes struct {
	Actions                     WorkspaceActions     `json:"actions"`
	AllowDestroyPlan            bool                 `json:"allow-destroy-plan"`
	AssessmentsEnabled          bool                 `json:"assessments-enabled"`
	AutoApply                   bool                 `json:"auto-apply"`
	AutoApplyRunTrigger         bool                 `json:"auto-apply-run-trigger"`
	AutoDestroyAt               *time.Time           `json:"auto-destroy-at"`
	AutoDestroyStatus           *string              `json:"auto-destroy-status"`
	AutoDestroyActivityDuration *string              `json:"auto-destroy-activity-duration"`
	InheritsProjectAutoDestroy  *bool                `json:"inherits-project-auto-destroy"`
	CreatedAt                   time.Time            `json:"created-at"`
	Description                 *string              `json:"description"`
	Environment                 string               `json:"environment"`
	ExecutionMode               string               `json:"execution-mode"`
	FileTriggersEnabled         bool                 `json:"file-triggers-enabled"`
	GlobalRemoteState           bool                 `json:"global-remote-state"`
	LatestChangeAt              time.Time            `json:"latest-change-at"`
	LastAssessmentResultAt      *time.Time           `json:"last-assessment-result-at"`
	Locked                      bool                 `json:"locked"`
	LockedReason                *string              `json:"locked-reason"`
	Name                        string               `json:"name"`
	OAuthClientName             *string              `json:"oauth-client-name"`
	Operations                  bool                 `json:"operations"`
	Permissions                 WorkspacePermissions `json:"permissions"`
	ApplyDurationAverage        *int                 `json:"apply-duration-average"`
	PlanDurationAverage         *int                 `json:"plan-duration-average"`
	PolicyCheckFailures         *int                 `json:"policy-check-failures"`
	QueueAllRuns                bool                 `json:"queue-all-runs"`
	ResourceCount               int                  `json:"resource-count"`
	RunFailures                 *int                 `json:"run-failures"`
	Source                      string               `json:"source"`
	SourceName                  *string              `json:"source-name"`
	SourceURL                   *string              `json:"source-url"`
	SpeculativeEnabled          bool                 `json:"speculative-enabled"`
	StructuredRunOutputEnabled  bool                 `json:"structured-run-output-enabled"`
	TagNames                    []string             `json:"tag-names"`
	TerraformVersion            string               `json:"terraform-version"`
	TriggerPrefixes             []string             `json:"trigger-prefixes"`
	TriggerPatterns             []string             `json:"trigger-patterns"`
	UpdatedAt                   time.Time            `json:"updated-at"`
	VCSRepo                     *WorkspaceVCSRepo    `json:"vcs-repo"`
	VCSRepoIdentifier           *string              `json:"vcs-repo-identifier"`
	WorkingDirectory            *string              `json:"working-directory"`
	WorkspaceKPIsRunsCount      *int                 `json:"workspace-kpis-runs-count"`
	SettingOverwrites           SettingOverwrites    `json:"setting-overwrites"`
	HYOKEnabled                 bool                 `json:"hyok-enabled"`
	// HCP Terraform specific fields
	UnarchivedWorkspaceChangeRequestsCount *int `json:"unarchived_workspace_change_requests_count,omitempty"`
}

// WorkspaceActions represents action permissions for a workspace
type WorkspaceActions struct {
	IsDestroyable bool `json:"is-destroyable"`
}

// WorkspacePermissions represents permissions for a workspace
type WorkspacePermissions struct {
	CanUpdate                    bool `json:"can-update"`
	CanDestroy                   bool `json:"can-destroy"`
	CanQueueRun                  bool `json:"can-queue-run"`
	CanReadRun                   bool `json:"can-read-run"`
	CanReadVariable              bool `json:"can-read-variable"`
	CanUpdateVariable            bool `json:"can-update-variable"`
	CanReadStateVersions         bool `json:"can-read-state-versions"`
	CanReadStateOutputs          bool `json:"can-read-state-outputs"`
	CanCreateStateVersions       bool `json:"can-create-state-versions"`
	CanQueueApply                bool `json:"can-queue-apply"`
	CanLock                      bool `json:"can-lock"`
	CanUnlock                    bool `json:"can-unlock"`
	CanForceUnlock               bool `json:"can-force-unlock"`
	CanReadSettings              bool `json:"can-read-settings"`
	CanManageTags                bool `json:"can-manage-tags"`
	CanManageRunTasks            bool `json:"can-manage-run-tasks"`
	CanForceDelete               bool `json:"can-force-delete"`
	CanManageAssessments         bool `json:"can-manage-assessments"`
	CanManageEphemeralWorkspaces bool `json:"can-manage-ephemeral-workspaces"`
	CanReadAssessmentResults     bool `json:"can-read-assessment-results"`
	CanQueueDestroy              bool `json:"can-queue-destroy"`
}

// WorkspaceVCSRepo represents VCS repository configuration for a workspace
type WorkspaceVCSRepo struct {
	Branch                  string  `json:"branch"`
	DisplayIdentifier       string  `json:"display-identifier"`
	Identifier              string  `json:"identifier"`
	IngressSubmodules       bool    `json:"ingress-submodules"`
	OAuthTokenID            *string `json:"oauth-token-id"`
	GitHubAppInstallationID *string `json:"github-app-installation-id"`
	RepositoryHTTPURL       string  `json:"repository-http-url"`
	ServiceProvider         string  `json:"service-provider"`
	TagsRegex               *string `json:"tags-regex"`
	WebhookURL              string  `json:"webhook-url"`
}

// SettingOverwrites represents setting overwrites for a workspace
type SettingOverwrites struct {
	ExecutionMode bool `json:"execution-mode"`
	AgentPool     bool `json:"agent-pool"`
}

// WorkspaceRelationships represents relationships for a workspace
type WorkspaceRelationships struct {
	AgentPool                   *WorkspaceRelationshipData   `json:"agent-pool,omitempty"`
	CurrentConfigurationVersion *WorkspaceRelationshipData   `json:"current-configuration-version,omitempty"`
	CurrentRun                  *WorkspaceRelationshipData   `json:"current-run,omitempty"`
	EffectiveTagBindings        *WorkspaceRelationshipLinks  `json:"effective-tag-bindings,omitempty"`
	LatestRun                   *WorkspaceRelationshipData   `json:"latest-run,omitempty"`
	CurrentStateVersion         *WorkspaceRelationshipData   `json:"current-state-version,omitempty"`
	CurrentAssessmentResult     *WorkspaceRelationshipData   `json:"current-assessment-result,omitempty"`
	Organization                *WorkspaceRelationshipData   `json:"organization,omitempty"`
	Outputs                     *WorkspaceOutputRelationship `json:"outputs,omitempty"`
	Project                     *WorkspaceRelationshipData   `json:"project,omitempty"`
	Readme                      *WorkspaceRelationshipData   `json:"readme,omitempty"`
	RemoteStateConsumers        *WorkspaceRelationshipLinks  `json:"remote-state-consumers,omitempty"`
	SSHKey                      *WorkspaceRelationshipData   `json:"ssh-key,omitempty"`
	LockedBy                    *WorkspaceRelationshipData   `json:"locked-by,omitempty"`
	TagBindings                 *WorkspaceRelationshipLinks  `json:"tag-bindings,omitempty"`
	Vars                        *WorkspaceVarsRelationship   `json:"vars,omitempty"`
}

// WorkspaceRelationshipData represents a single relationship data item
type WorkspaceRelationshipData struct {
	Data  *RelationshipDataItem `json:"data"`
	Links *RelationshipLinks    `json:"links,omitempty"`
}

// WorkspaceRelationshipLinks represents relationship links
type WorkspaceRelationshipLinks struct {
	Links *RelationshipLinks `json:"links,omitempty"`
}

// WorkspaceOutputRelationship represents workspace output relationships
type WorkspaceOutputRelationship struct {
	Data  []RelationshipDataItem `json:"data"`
	Links *RelationshipLinks     `json:"links,omitempty"`
}

// WorkspaceVarsRelationship represents workspace variable relationships
type WorkspaceVarsRelationship struct {
	Data []RelationshipDataItem `json:"data"`
}

// RelationshipDataItem represents a single relationship data item
type RelationshipDataItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// RelationshipLinks represents relationship links
type RelationshipLinks struct {
	Self          string `json:"self,omitempty"`
	Related       string `json:"related,omitempty"`
	InheritedFrom string `json:"inherited-from,omitempty"`
}

// WorkspaceLinks represents links for a workspace
type WorkspaceLinks struct {
	Self     string `json:"self"`
	SelfHTML string `json:"self-html"`
}

// WorkspaceListOptions represents query parameters for listing workspaces
type WorkspaceListOptions struct {
	PageNumber             int      `url:"page[number],omitempty"`
	PageSize               int      `url:"page[size],omitempty"`
	SearchName             string   `url:"search[name],omitempty"`
	SearchTags             []string `url:"search[tags],omitempty"`
	SearchExcludeTags      []string `url:"search[exclude-tags],omitempty"`
	SearchWildcardName     string   `url:"search[wildcard-name],omitempty"`
	Sort                   string   `url:"sort,omitempty"`
	FilterProjectID        string   `url:"filter[project][id],omitempty"`
	FilterCurrentRunStatus string   `url:"filter[current-run][status],omitempty"`
	FilterTaggedKeys       []string `url:"filter[tagged][key],omitempty"`
	FilterTaggedValues     []string `url:"filter[tagged][value],omitempty"`
}

// WorkspaceCreateRequest represents the request body for creating a workspace
type WorkspaceCreateRequest struct {
	Data WorkspaceCreateData `json:"data"`
}

// WorkspaceCreateData represents the data portion of workspace creation
type WorkspaceCreateData struct {
	Type          string                        `json:"type"`
	Attributes    WorkspaceCreateAttributes     `json:"attributes"`
	Relationships *WorkspaceCreateRelationships `json:"relationships,omitempty"`
}

// WorkspaceCreateAttributes represents attributes for workspace creation
type WorkspaceCreateAttributes struct {
	Name                        string                  `json:"name"`
	AgentPoolID                 *string                 `json:"agent-pool-id,omitempty"`
	AllowDestroyPlan            *bool                   `json:"allow-destroy-plan,omitempty"`
	AssessmentsEnabled          *bool                   `json:"assessments-enabled,omitempty"`
	AutoApply                   *bool                   `json:"auto-apply,omitempty"`
	AutoApplyRunTrigger         *bool                   `json:"auto-apply-run-trigger,omitempty"`
	AutoDestroyAt               *string                 `json:"auto-destroy-at,omitempty"`
	AutoDestroyActivityDuration *string                 `json:"auto-destroy-activity-duration,omitempty"`
	Description                 *string                 `json:"description,omitempty"`
	ExecutionMode               *string                 `json:"execution-mode,omitempty"`
	FileTriggersEnabled         *bool                   `json:"file-triggers-enabled,omitempty"`
	GlobalRemoteState           *bool                   `json:"global-remote-state,omitempty"`
	Operations                  *bool                   `json:"operations,omitempty"`
	QueueAllRuns                *bool                   `json:"queue-all-runs,omitempty"`
	SourceName                  *string                 `json:"source-name,omitempty"`
	SourceURL                   *string                 `json:"source-url,omitempty"`
	SpeculativeEnabled          *bool                   `json:"speculative-enabled,omitempty"`
	TerraformVersion            *string                 `json:"terraform-version,omitempty"`
	TriggerPatterns             []string                `json:"trigger-patterns,omitempty"`
	TriggerPrefixes             []string                `json:"trigger-prefixes,omitempty"`
	VCSRepo                     *WorkspaceCreateVCSRepo `json:"vcs-repo,omitempty"`
	WorkingDirectory            *string                 `json:"working-directory,omitempty"`
	SettingOverwrites           *SettingOverwrites      `json:"setting-overwrites,omitempty"`
	HYOKEnabled                 *bool                   `json:"hyok-enabled,omitempty"`
}

// WorkspaceCreateVCSRepo represents VCS repository configuration for workspace creation
type WorkspaceCreateVCSRepo struct {
	Branch                  *string `json:"branch,omitempty"`
	Identifier              string  `json:"identifier"`
	IngressSubmodules       *bool   `json:"ingress-submodules,omitempty"`
	OAuthTokenID            *string `json:"oauth-token-id,omitempty"`
	GitHubAppInstallationID *string `json:"github-app-installation-id,omitempty"`
	TagsRegex               *string `json:"tags-regex,omitempty"`
}

// WorkspaceCreateRelationships represents relationships for workspace creation
type WorkspaceCreateRelationships struct {
	Project     *WorkspaceCreateRelationshipProject     `json:"project,omitempty"`
	TagBindings *WorkspaceCreateRelationshipTagBindings `json:"tag-bindings,omitempty"`
}

// WorkspaceCreateRelationshipProject represents project relationship for workspace creation
type WorkspaceCreateRelationshipProject struct {
	Data RelationshipDataItem `json:"data"`
}

// WorkspaceCreateRelationshipTagBindings represents tag bindings relationship for workspace creation
type WorkspaceCreateRelationshipTagBindings struct {
	Data []WorkspaceTagBinding `json:"data"`
}

// WorkspaceTagBinding represents a tag binding for workspace creation
type WorkspaceTagBinding struct {
	Type       string                   `json:"type"`
	Attributes WorkspaceTagBindingAttrs `json:"attributes"`
}

// WorkspaceTagBindingAttrs represents tag binding attributes
type WorkspaceTagBindingAttrs struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// WorkspaceUpdateRequest represents the request body for updating a workspace
type WorkspaceUpdateRequest struct {
	Data WorkspaceUpdateData `json:"data"`
}

// WorkspaceUpdateData represents the data portion of workspace update
type WorkspaceUpdateData struct {
	Type          string                        `json:"type"`
	Attributes    *WorkspaceUpdateAttributes    `json:"attributes,omitempty"`
	Relationships *WorkspaceCreateRelationships `json:"relationships,omitempty"`
}

// WorkspaceUpdateAttributes represents attributes for workspace update
type WorkspaceUpdateAttributes struct {
	Name                        *string                 `json:"name,omitempty"`
	AgentPoolID                 *string                 `json:"agent-pool-id,omitempty"`
	AllowDestroyPlan            *bool                   `json:"allow-destroy-plan,omitempty"`
	AssessmentsEnabled          *bool                   `json:"assessments-enabled,omitempty"`
	AutoApply                   *bool                   `json:"auto-apply,omitempty"`
	AutoApplyRunTrigger         *bool                   `json:"auto-apply-run-trigger,omitempty"`
	AutoDestroyAt               *string                 `json:"auto-destroy-at,omitempty"`
	AutoDestroyActivityDuration *string                 `json:"auto-destroy-activity-duration,omitempty"`
	Description                 *string                 `json:"description,omitempty"`
	ExecutionMode               *string                 `json:"execution-mode,omitempty"`
	FileTriggersEnabled         *bool                   `json:"file-triggers-enabled,omitempty"`
	GlobalRemoteState           *bool                   `json:"global-remote-state,omitempty"`
	Operations                  *bool                   `json:"operations,omitempty"`
	QueueAllRuns                *bool                   `json:"queue-all-runs,omitempty"`
	SpeculativeEnabled          *bool                   `json:"speculative-enabled,omitempty"`
	TerraformVersion            *string                 `json:"terraform-version,omitempty"`
	TriggerPatterns             []string                `json:"trigger-patterns,omitempty"`
	TriggerPrefixes             []string                `json:"trigger-prefixes,omitempty"`
	VCSRepo                     *WorkspaceCreateVCSRepo `json:"vcs-repo,omitempty"`
	WorkingDirectory            *string                 `json:"working-directory,omitempty"`
	SettingOverwrites           *SettingOverwrites      `json:"setting-overwrites,omitempty"`
}

// WorkspaceLockRequest represents the request body for locking a workspace
type WorkspaceLockRequest struct {
	Reason string `json:"reason"`
}

// WorkspaceUnlockRequest represents the request body for unlocking a workspace
type WorkspaceUnlockRequest struct {
	ForceUnlock *bool `json:"force-unlock,omitempty"`
}

// WorkspaceLockResponse represents the response when locking/unlocking a workspace
type WorkspaceLockResponse struct {
	Data WorkspaceLockData `json:"data"`
}

// WorkspaceLockData represents workspace lock data in the response
type WorkspaceLockData struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Attributes WorkspaceLockAttributes `json:"attributes"`
}

// WorkspaceLockAttributes contains the attributes of a workspace lock
type WorkspaceLockAttributes struct {
	Locked   bool    `json:"locked"`
	LockedBy *string `json:"locked-by,omitempty"`
	LockedAt *string `json:"locked-at,omitempty"`
}

// ====================
// Variable Types
// ====================

// Variable represents a workspace variable in HCP Terraform
type Variable struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Attributes    VariableAttributes     `json:"attributes"`
	Links         *VariableLinks         `json:"links,omitempty"`
	Relationships *VariableRelationships `json:"relationships,omitempty"`
}

// VariableAttributes contains the attributes of a variable
type VariableAttributes struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Category    string `json:"category"` // "terraform" or "env"
	HCL         bool   `json:"hcl"`
	Sensitive   bool   `json:"sensitive"`
	VersionID   string `json:"version-id"`
}

// VariableLinks contains links related to a variable
type VariableLinks struct {
	Self string `json:"self"`
}

// VariableRelationships contains relationships for a variable
type VariableRelationships struct {
	Configurable *VariableConfigurable `json:"configurable,omitempty"`
}

// VariableConfigurable represents the workspace this variable belongs to
type VariableConfigurable struct {
	Data  RelationshipDataItem `json:"data"`
	Links *RelationshipLinks   `json:"links,omitempty"`
}

// VariableResponse represents a list of variables from the API
type VariableResponse struct {
	Data []Variable          `json:"data"`
	Meta *PaginationMetadata `json:"meta,omitempty"`
}

// SingleVariableResponse represents a single variable response from the API
type SingleVariableResponse struct {
	Data Variable `json:"data"`
}

// VariableCreateRequest represents the request body for creating a variable
type VariableCreateRequest struct {
	Data VariableCreateData `json:"data"`
}

// VariableCreateData contains the data for creating a variable
type VariableCreateData struct {
	Type       string                   `json:"type"`
	Attributes VariableCreateAttributes `json:"attributes"`
}

// VariableCreateAttributes contains the attributes for creating a variable
type VariableCreateAttributes struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"` // "terraform" or "env"
	HCL         bool   `json:"hcl"`
	Sensitive   bool   `json:"sensitive"`
}

// VariableUpdateRequest represents the request body for updating a variable
type VariableUpdateRequest struct {
	Data VariableUpdateData `json:"data"`
}

// VariableUpdateData contains the data for updating a variable
type VariableUpdateData struct {
	ID         string                    `json:"id"`
	Type       string                    `json:"type"`
	Attributes *VariableUpdateAttributes `json:"attributes,omitempty"`
}

// VariableUpdateAttributes contains the attributes for updating a variable
type VariableUpdateAttributes struct {
	Key         *string `json:"key,omitempty"`
	Value       *string `json:"value,omitempty"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"` // "terraform" or "env"
	HCL         *bool   `json:"hcl,omitempty"`
	Sensitive   *bool   `json:"sensitive,omitempty"`
}

// BulkVariableCreateRequest represents the request body for creating multiple variables
type BulkVariableCreateRequest struct {
	Data []VariableCreateData `json:"data"`
}

// BulkVariableCreateResponse represents the response for creating multiple variables
type BulkVariableCreateResponse struct {
	Data []Variable `json:"data"`
}

// ConfigurationVersion represents a configuration version in HCP Terraform
type ConfigurationVersion struct {
	ID            string                             `json:"id"`
	Type          string                             `json:"type"` // "configuration-versions"
	Attributes    ConfigurationVersionAttributes     `json:"attributes"`
	Relationships *ConfigurationVersionRelationships `json:"relationships,omitempty"`
	Links         *ConfigurationVersionLinks         `json:"links,omitempty"`
}

// ConfigurationVersionAttributes contains the attributes of a configuration version
type ConfigurationVersionAttributes struct {
	AutoQueueRuns    bool             `json:"auto-queue-runs"`
	Error            *string          `json:"error"`
	ErrorMessage     *string          `json:"error-message"`
	Source           string           `json:"source"`
	Speculative      bool             `json:"speculative"`
	Status           string           `json:"status"`
	StatusTimestamps StatusTimestamps `json:"status-timestamps"`
	UploadURL        *string          `json:"upload-url"`
	ProvisionallyAt  *string          `json:"provisionally-at"`
	ChangedFiles     []string         `json:"changed-files"`
	CommitSHA        *string          `json:"commit-sha"`
	CommitURL        *string          `json:"commit-url"`
	CreatedAt        string           `json:"created-at"`
	UpdatedAt        string           `json:"updated-at"`
}

// StatusTimestamps contains timestamps for configuration version status changes
type StatusTimestamps struct {
	UploadedAt *string `json:"uploaded-at"`
	ArchivedAt *string `json:"archived-at"`
	FinishedAt *string `json:"finished-at"`
	QueuedAt   *string `json:"queued-at"`
	StartedAt  *string `json:"started-at"`
}

// ConfigurationVersionRelationships contains relationships for a configuration version
type ConfigurationVersionRelationships struct {
	IngressAttributes *RelationshipData `json:"ingress-attributes,omitempty"`
}

// ConfigurationVersionLinks contains links for a configuration version
type ConfigurationVersionLinks struct {
	Download *string `json:"download,omitempty"`
}

// ConfigurationVersionResponse represents the API response for a single configuration version
type ConfigurationVersionResponse struct {
	Data     ConfigurationVersion     `json:"data"`
	Included []map[string]interface{} `json:"included,omitempty"`
}

// ConfigurationVersionsResponse represents the API response for multiple configuration versions
type ConfigurationVersionsResponse struct {
	Data []ConfigurationVersion `json:"data"`
	Meta *PaginationMetadata    `json:"meta,omitempty"`
}

// ConfigurationVersionCreateRequest represents the request to create a configuration version
type ConfigurationVersionCreateRequest struct {
	Data ConfigurationVersionCreateData `json:"data"`
}

// ConfigurationVersionCreateData contains the data for creating a configuration version
type ConfigurationVersionCreateData struct {
	Type       string                               `json:"type"` // "configuration-versions"
	Attributes ConfigurationVersionCreateAttributes `json:"attributes"`
}

// ConfigurationVersionCreateAttributes contains the attributes for creating a configuration version
type ConfigurationVersionCreateAttributes struct {
	AutoQueueRuns bool `json:"auto-queue-runs"`
	Speculative   bool `json:"speculative"`
}

// StateVersion represents a state version in HCP Terraform
type StateVersion struct {
	ID            string                     `json:"id"`
	Type          string                     `json:"type"` // "state-versions"
	Attributes    StateVersionAttributes     `json:"attributes"`
	Relationships *StateVersionRelationships `json:"relationships,omitempty"`
	Links         *StateVersionLinks         `json:"links,omitempty"`
}

// StateVersionAttributes contains the attributes of a state version
type StateVersionAttributes struct {
	CreatedAt              string  `json:"created-at"`
	DownloadURL            *string `json:"download-url"`
	HostedStateDownloadURL *string `json:"hosted-state-download-url"`
	Serial                 int     `json:"serial"`
	Size                   int     `json:"size"`
	VCSCommitSHA           *string `json:"vcs-commit-sha"`
	VCSCommitURL           *string `json:"vcs-commit-url"`
	Lineage                *string `json:"lineage"`
	Status                 string  `json:"status"`
	TerraformVersion       string  `json:"terraform-version"`
	UpdatedAt              string  `json:"updated-at"`
	JSONStateSizeBytes     *int    `json:"json-state-size-bytes"`
	HasStateData           bool    `json:"has-state-data"`
}

// StateVersionRelationships contains relationships for a state version
type StateVersionRelationships struct {
	CreatedBy *RelationshipData `json:"created-by,omitempty"`
	Run       *RelationshipData `json:"run,omitempty"`
	Workspace *RelationshipData `json:"workspace,omitempty"`
	Outputs   *RelationshipData `json:"outputs,omitempty"`
}

// StateVersionLinks contains links for a state version
type StateVersionLinks struct {
	Download *string `json:"download,omitempty"`
}

// StateVersionResponse represents the API response for a single state version
type StateVersionResponse struct {
	Data     StateVersion             `json:"data"`
	Included []map[string]interface{} `json:"included,omitempty"`
}

// StateVersionsResponse represents the API response for multiple state versions
type StateVersionsResponse struct {
	Data []StateVersion      `json:"data"`
	Meta *PaginationMetadata `json:"meta,omitempty"`
}

// StateVersionCreateRequest represents the request to create a state version
type StateVersionCreateRequest struct {
	Data StateVersionCreateData `json:"data"`
}

// StateVersionCreateData contains the data for creating a state version
type StateVersionCreateData struct {
	Type       string                       `json:"type"` // "state-versions"
	Attributes StateVersionCreateAttributes `json:"attributes"`
}

// StateVersionCreateAttributes contains the attributes for creating a state version
type StateVersionCreateAttributes struct {
	Serial  int     `json:"serial"`
	MD5     string  `json:"md5"`
	Lineage *string `json:"lineage,omitempty"`
	State   string  `json:"state"` // Base64 encoded state content
}

// TagBinding represents a tag binding in HCP Terraform
type TagBinding struct {
	ID            string                   `json:"id"`
	Type          string                   `json:"type"` // "tag-bindings"
	Attributes    TagBindingAttributes     `json:"attributes"`
	Relationships *TagBindingRelationships `json:"relationships,omitempty"`
}

// TagBindingAttributes contains the attributes of a tag binding
type TagBindingAttributes struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TagBindingRelationships contains relationships for a tag binding
type TagBindingRelationships struct {
	Tag      *RelationshipData `json:"tag,omitempty"`
	Taggable *RelationshipData `json:"taggable,omitempty"`
}

// TagBindingsResponse represents the API response for tag bindings
type TagBindingsResponse struct {
	Data []TagBinding        `json:"data"`
	Meta *PaginationMetadata `json:"meta,omitempty"`
}

// TagBindingCreateRequest represents the request to create tag bindings
type TagBindingCreateRequest struct {
	Data []TagBindingCreateData `json:"data"`
}

// TagBindingCreateData contains the data for creating a tag binding
type TagBindingCreateData struct {
	Type       string                     `json:"type"` // "tag-bindings"
	Attributes TagBindingCreateAttributes `json:"attributes"`
}

// TagBindingCreateAttributes contains the attributes for creating a tag binding
type TagBindingCreateAttributes struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TagBindingCreateResponse represents the response for creating tag bindings
type TagBindingCreateResponse struct {
	Data []TagBinding `json:"data"`
}

// TagBindingUpdateRequest represents the request to update tag bindings
type TagBindingUpdateRequest struct {
	Data []TagBindingUpdateData `json:"data"`
}

// TagBindingUpdateData contains the data for updating a tag binding
type TagBindingUpdateData struct {
	ID         string                     `json:"id"`
	Type       string                     `json:"type"` // "tag-bindings"
	Attributes TagBindingUpdateAttributes `json:"attributes"`
}

// TagBindingUpdateAttributes contains the attributes for updating a tag binding
type TagBindingUpdateAttributes struct {
	Value string `json:"value"`
}

// ====================
// Remote State Consumer Types
// ====================

// RemoteStateConsumer represents a workspace that can access another workspace's state
type RemoteStateConsumer struct {
	ID            string                            `json:"id"`
	Type          string                            `json:"type"` // "workspaces"
	Attributes    RemoteStateConsumerAttributes     `json:"attributes"`
	Relationships *RemoteStateConsumerRelationships `json:"relationships,omitempty"`
}

// RemoteStateConsumerAttributes contains the attributes of a remote state consumer
type RemoteStateConsumerAttributes struct {
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	Environment      *string `json:"environment,omitempty"`
	AutoApply        *bool   `json:"auto-apply,omitempty"`
	Locked           bool    `json:"locked"`
	TerraformVersion *string `json:"terraform-version,omitempty"`
}

// RemoteStateConsumerRelationships contains relationships for a remote state consumer
type RemoteStateConsumerRelationships struct {
	Organization *RelationshipData `json:"organization,omitempty"`
	Project      *RelationshipData `json:"project,omitempty"`
}

// RemoteStateConsumersResponse represents the API response for remote state consumers
type RemoteStateConsumersResponse struct {
	Data []RemoteStateConsumer `json:"data"`
	Meta *PaginationMetadata   `json:"meta,omitempty"`
}

// RemoteStateConsumerRequest represents the request to add/remove remote state consumers
type RemoteStateConsumerRequest struct {
	Data []RemoteStateConsumerData `json:"data"`
}

// RemoteStateConsumerData contains the data for adding/removing remote state consumers
type RemoteStateConsumerData struct {
	Type string `json:"type"` // "workspaces"
	ID   string `json:"id"`   // workspace ID
}

// ====================
// Run Types
// ====================

// RunResponse represents the API response for listing runs
type RunResponse struct {
	Data  []Run              `json:"data"`
	Links PaginationLinks    `json:"links"`
	Meta  PaginationMetadata `json:"meta"`
}

// SingleRunResponse represents the API response for a single run
type SingleRunResponse struct {
	Data     Run           `json:"data"`
	Included []interface{} `json:"included,omitempty"`
}

// Run represents a single HCP Terraform run
type Run struct {
	ID            string           `json:"id"`
	Type          string           `json:"type"`
	Attributes    RunAttributes    `json:"attributes"`
	Relationships RunRelationships `json:"relationships,omitempty"`
	Links         RunLinks         `json:"links,omitempty"`
}

// RunAttributes contains run details
type RunAttributes struct {
	Status                string                 `json:"status"`
	StatusTimestamps      RunStatusTimestamps    `json:"status-timestamps"`
	Message               *string                `json:"message"`
	Source                string                 `json:"source"`
	IsDestroy             bool                   `json:"is-destroy"`
	Refresh               bool                   `json:"refresh"`
	RefreshOnly           bool                   `json:"refresh-only"`
	PlanOnly              bool                   `json:"plan-only"`
	AutoApply             bool                   `json:"auto-apply"`
	CreatedAt             time.Time              `json:"created-at"`
	HasChanges            *bool                  `json:"has-changes"`
	Actions               RunActions             `json:"actions"`
	Permissions           RunPermissions         `json:"permissions"`
	TargetAddrs           []string               `json:"target-addrs,omitempty"`
	ReplaceAddrs          []string               `json:"replace-addrs,omitempty"`
	Variables             []interface{}          `json:"variables,omitempty"`
	ErrorText             *string                `json:"error-text,omitempty"`
	PositionInQueue       *int                   `json:"position-in-queue,omitempty"`
	TerraformVersion      string                 `json:"terraform-version"`
	AllowEmptyApply       bool                   `json:"allow-empty-apply"`
	AllowConfigGeneration bool                   `json:"allow-config-generation"`
}

// RunStatusTimestamps contains timestamps for different run statuses
type RunStatusTimestamps struct {
	QueuedAt         *time.Time `json:"queued-at,omitempty"`
	PlanQueueableAt  *time.Time `json:"plan-queueable-at,omitempty"`
	PlanningAt       *time.Time `json:"planning-at,omitempty"`
	PlannedAt        *time.Time `json:"planned-at,omitempty"`
	ConfirmedAt      *time.Time `json:"confirmed-at,omitempty"`
	ApplyQueueableAt *time.Time `json:"apply-queueable-at,omitempty"`
	ApplyingAt       *time.Time `json:"applying-at,omitempty"`
	AppliedAt        *time.Time `json:"applied-at,omitempty"`
	DiscardedAt      *time.Time `json:"discarded-at,omitempty"`
	ErroredAt        *time.Time `json:"errored-at,omitempty"`
	CanceledAt       *time.Time `json:"canceled-at,omitempty"`
	ForceCanceledAt  *time.Time `json:"force-canceled-at,omitempty"`
}

// RunActions contains available actions for the run
type RunActions struct {
	IsCancelable      bool `json:"is-cancelable"`
	IsConfirmable     bool `json:"is-confirmable"`
	IsDiscardable     bool `json:"is-discardable"`
	IsForceCancelable bool `json:"is-force-cancelable"`
}

// RunPermissions contains user permissions for the run
type RunPermissions struct {
	CanApply               bool `json:"can-apply"`
	CanCancel              bool `json:"can-cancel"`
	CanDiscard             bool `json:"can-discard"`
	CanForceCancel         bool `json:"can-force-cancel"`
	CanForceExecute        bool `json:"can-force-execute"`
	CanOverridePolicyCheck bool `json:"can-override-policy-check"`
}

// RunRelationships contains run relationships
type RunRelationships struct {
	Workspace            *RelationshipData `json:"workspace,omitempty"`
	ConfigurationVersion *RelationshipData `json:"configuration-version,omitempty"`
	Plan                 *RelationshipData `json:"plan,omitempty"`
	Apply                *RelationshipData `json:"apply,omitempty"`
	CreatedBy            *RelationshipData `json:"created-by,omitempty"`
	Comments             *RelationshipData `json:"comments,omitempty"`
	RunEvents            *RelationshipData `json:"run-events,omitempty"`
	TaskStages           *RelationshipData `json:"task-stages,omitempty"`
	PolicyChecks         *RelationshipData `json:"policy-checks,omitempty"`
	CostEstimate         *RelationshipData `json:"cost-estimate,omitempty"`
}

// RunLinks contains run-related links
type RunLinks struct {
	Self string `json:"self"`
}

// RunCreateRequest represents the request to create a new run
type RunCreateRequest struct {
	Data RunCreateData `json:"data"`
}

// RunCreateData contains the data for creating a run
type RunCreateData struct {
	Type          string                 `json:"type"`
	Attributes    RunCreateAttributes    `json:"attributes"`
	Relationships RunCreateRelationships `json:"relationships"`
}

// RunCreateAttributes contains the attributes for creating a run
type RunCreateAttributes struct {
	Message               *string                `json:"message,omitempty"`
	IsDestroy             bool                   `json:"is-destroy"`
	Refresh               bool                   `json:"refresh"`
	RefreshOnly           bool                   `json:"refresh-only"`
	PlanOnly              bool                   `json:"plan-only"`
	AutoApply             *bool                  `json:"auto-apply,omitempty"`
	TargetAddrs           []string               `json:"target-addrs,omitempty"`
	ReplaceAddrs          []string               `json:"replace-addrs,omitempty"`
	Variables             map[string]interface{} `json:"variables,omitempty"`
	AllowEmptyApply       *bool                  `json:"allow-empty-apply,omitempty"`
	AllowConfigGeneration *bool                  `json:"allow-config-generation,omitempty"`
	TerraformVersion      *string                `json:"terraform-version,omitempty"`
}

// RunCreateRelationships contains the relationships for creating a run
type RunCreateRelationships struct {
	Workspace            RelationshipDataItem  `json:"workspace"`
	ConfigurationVersion *RelationshipDataItem `json:"configuration-version,omitempty"`
}

// RunListOptions contains options for listing runs
type RunListOptions struct {
	PageSize   int      `json:"page[size],omitempty"`
	PageNumber int      `json:"page[number],omitempty"`
	Status     string   `json:"filter[status],omitempty"`
	Operation  string   `json:"filter[operation],omitempty"`
	Source     string   `json:"filter[source],omitempty"`
	Include    []string `json:"include,omitempty"`
}

// RunActionRequest represents a request to perform an action on a run
type RunActionRequest struct {
	Comment *string `json:"comment,omitempty"`
}

// ====================
// Plan Types
// ====================

// SinglePlanResponse represents an API response for a single plan
type SinglePlanResponse struct {
	Data     Plan          `json:"data"`
	Included []interface{} `json:"included,omitempty"`
}

// Plan represents a single HCP Terraform plan
type Plan struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Attributes    PlanAttributes    `json:"attributes"`
	Relationships PlanRelationships `json:"relationships,omitempty"`
	Links         PlanLinks         `json:"links,omitempty"`
}

// PlanAttributes contains plan details
type PlanAttributes struct {
	Status               string               `json:"status"`
	StatusTimestamps     PlanStatusTimestamps `json:"status-timestamps"`
	HasChanges           bool                 `json:"has-changes"`
	ResourceAdditions    int                  `json:"resource-additions"`
	ResourceChanges      int                  `json:"resource-changes"`
	ResourceDestructions int                  `json:"resource-destructions"`
	ErrorMessage         *string              `json:"error-message,omitempty"`
	LogReadURL           *string              `json:"log-read-url,omitempty"`
	ExecutionMode        string               `json:"execution-mode"`
}

// PlanStatusTimestamps contains timestamps for different plan statuses
type PlanStatusTimestamps struct {
	QueuedAt   *time.Time `json:"queued-at,omitempty"`
	PendingAt  *time.Time `json:"pending-at,omitempty"`
	RunningAt  *time.Time `json:"running-at,omitempty"`
	FinishedAt *time.Time `json:"finished-at,omitempty"`
	ErroredAt  *time.Time `json:"errored-at,omitempty"`
	CanceledAt *time.Time `json:"canceled-at,omitempty"`
}

// PlanRelationships contains plan relationship data
type PlanRelationships struct {
	Workspace struct {
		Data RelationshipData `json:"data"`
	} `json:"workspace,omitempty"`
}

// PlanLinks contains plan-related links
type PlanLinks struct {
	Self string `json:"self,omitempty"`
}

// ====================
// Apply Types
// ====================

// SingleApplyResponse represents an API response for a single apply
type SingleApplyResponse struct {
	Data     Apply         `json:"data"`
	Included []interface{} `json:"included,omitempty"`
}

// Apply represents a single HCP Terraform apply
type Apply struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Attributes    ApplyAttributes    `json:"attributes"`
	Relationships ApplyRelationships `json:"relationships,omitempty"`
	Links         ApplyLinks         `json:"links,omitempty"`
}

// ApplyAttributes contains apply details
type ApplyAttributes struct {
	Status               string                `json:"status"`
	StatusTimestamps     ApplyStatusTimestamps `json:"status-timestamps"`
	ResourceAdditions    int                   `json:"resource-additions"`
	ResourceChanges      int                   `json:"resource-changes"`
	ResourceDestructions int                   `json:"resource-destructions"`
	ErrorMessage         *string               `json:"error-message,omitempty"`
	LogReadURL           *string               `json:"log-read-url,omitempty"`
	ExecutionMode        string                `json:"execution-mode"`
}

// ApplyStatusTimestamps contains timestamps for different apply statuses
type ApplyStatusTimestamps struct {
	QueuedAt   *time.Time `json:"queued-at,omitempty"`
	PendingAt  *time.Time `json:"pending-at,omitempty"`
	RunningAt  *time.Time `json:"running-at,omitempty"`
	FinishedAt *time.Time `json:"finished-at,omitempty"`
	ErroredAt  *time.Time `json:"errored-at,omitempty"`
	CanceledAt *time.Time `json:"canceled-at,omitempty"`
}

// ApplyRelationships contains apply relationship data
type ApplyRelationships struct {
	Workspace struct {
		Data RelationshipData `json:"data"`
	} `json:"workspace,omitempty"`
}

// ApplyLinks contains apply-related links
type ApplyLinks struct {
	Self string `json:"self,omitempty"`
}
