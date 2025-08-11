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
