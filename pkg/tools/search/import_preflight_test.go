// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareImportInputsRequireExactLockAndExplicitSelection(t *testing.T) {
	input := PrepareImportInput{OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example", LockedProviders: map[string]string{"registry.terraform.io/hashicorp/aws": "6.66.0"}}
	input.Selections = []ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}}
	require.True(t, validNormalizedPrepareImportInput(input))
	input.LockedProviders["registry.terraform.io/hashicorp/aws"] = ">= 6.0"
	require.False(t, validNormalizedPrepareImportInput(input))
	input.LockedProviders["registry.terraform.io/hashicorp/aws"] = "6.66.0"
	input.Selections = append(input.Selections, input.Selections[0])
	require.False(t, validNormalizedPrepareImportInput(input))
}

func TestPrepareImportPreservesTerraformDomainValidation(t *testing.T) {
	validMarker := "qrm1-" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name           string
		providerSource string
		marker         string
		managedType    string
	}{
		{name: "invalid provider source", providerSource: "bad source/aws", marker: validMarker, managedType: "aws_s3_bucket"},
		{name: "invalid query result marker", providerSource: "registry.terraform.io/hashicorp/aws", marker: "opaque-result", managedType: "aws_s3_bucket"},
		{name: "invalid managed type", providerSource: "registry.terraform.io/hashicorp/aws", marker: validMarker, managedType: "aws-s3-bucket"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := PrepareImportInput{
				OrganizationName: "org",
				WorkspaceName:    "target",
				QueryRunID:       "opaque query run id",
				Selections:       []ImportSelection{{QueryResultMarker: tc.marker, ManagedType: tc.managedType}},
				LockedProviders:  map[string]string{tc.providerSource: "6.66.0"},
			}
			require.False(t, validNormalizedPrepareImportInput(input))
		})
	}

}

func TestPrepareImportProviderSourceDiagnostics(t *testing.T) {
	validMarker := "qrm1-" + strings.Repeat("a", 64)
	input := PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections: []ImportSelection{{QueryResultMarker: validMarker, ManagedType: "aws_s3_bucket"}},
	}

	input.LockedProviders = map[string]string{"registry.terraform.io/hashicorp/aws": "6.18.0"}
	require.NoError(t, ValidatePrepareImportInput(input))
	input.LockedProviders = map[string]string{"providers.example.com/team/aws": "6.18.0"}
	require.NoError(t, ValidatePrepareImportInput(input), "custom registry hosts remain valid")

	input.LockedProviders = map[string]string{"hashicorp/aws": "6.18.0"}
	require.NoError(t, ValidatePrepareImportInput(input), "public registry shorthand is canonicalizable")

	input.LockedProviders = map[string]string{"aws": "6.18.0"}
	err := ValidatePrepareImportInput(input)
	require.EqualError(t, err, `locked_provider_source_invalid: locked_providers key "aws" lacks a provider namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl`)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, lockedProviderSourceInvalidCode, code)

	input.LockedProviders = map[string]string{
		"hashicorp/aws":                       "6.18.0",
		"registry.terraform.io/hashicorp/aws": "6.18.0",
	}
	require.NoError(t, ValidatePrepareImportInput(input))

	input.LockedProviders["hashicorp/aws"] = "6.19.0"
	err = ValidatePrepareImportInput(input)
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, `resolve to "registry.terraform.io/hashicorp/aws"`)
	require.ErrorContains(t, err, "different exact versions")

	input.LockedProviders = map[string]string{"unsafe\nsource": "6.18.0"}
	err = ValidatePrepareImportInput(input)
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.NotContains(t, err.Error(), "unsafe")
	require.NotContains(t, err.Error(), "\n")

	input.LockedProviders = map[string]string{"registry.terraform.io/hashicorp/aws": ">= 6.0"}
	err = ValidatePrepareImportInput(input)
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, "exact selected version")
}

func TestPrepareImportInvalidSourceFailsBeforeClientUse(t *testing.T) {
	input := PrepareImportInput{
		OrganizationName: "org", WorkspaceName: "target", QueryRunID: "qry-example",
		Selections:      []ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}},
		LockedProviders: map[string]string{"aws": "6.18.0"},
	}
	result, err := ReadPrepareImport(context.Background(), nil, input)
	require.Nil(t, result)
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.ErrorContains(t, err, "lacks a provider namespace")
}

func TestImportInputsAndResultsHaveNoApplicationSizeLimit(t *testing.T) {
	selections := make([]ImportSelection, 101)
	for i := range selections {
		selections[i] = ImportSelection{QueryResultMarker: fmt.Sprintf("qrm1-%064x", i), ManagedType: "aws_s3_bucket"}
	}
	input := PrepareImportInput{
		OrganizationName: "org",
		WorkspaceName:    "target",
		QueryRunID:       "qry-example",
		Selections:       selections,
		LockedProviders:  map[string]string{"registry.terraform.io/hashicorp/aws": "6.66.0"},
	}
	require.True(t, validNormalizedPrepareImportInput(input))

	text, err := FormatImportResult(PrepareImportResult{Status: "prepared", NextAction: strings.Repeat("x", 256<<10)})
	require.NoError(t, err)
	require.Contains(t, text, `"status":"prepared"`)
}

func TestImportInputDoesNotAssumeHCPIdentifierFormats(t *testing.T) {
	input := PrepareImportInput{
		OrganizationName: "organization.example",
		WorkspaceName:    "workspace/name",
		WorkspaceID:      "opaque workspace id",
		QueryRunID:       "opaque query run id",
		Selections:       []ImportSelection{{QueryResultMarker: "qrm1-" + strings.Repeat("a", 64), ManagedType: "aws_s3_bucket"}},
		LockedProviders:  map[string]string{"registry.terraform.io/hashicorp/aws": "6.66.0"},
	}
	require.True(t, validNormalizedPrepareImportInput(input))
}

func TestImportArgumentDecodingHasNoApplicationSizeOrDepthLimit(t *testing.T) {
	type arguments struct {
		Value any `json:"value"`
	}
	deep := any("value")
	for range 65 {
		deep = []any{deep}
	}
	raw, err := json.Marshal(map[string]any{"value": map[string]any{"large": strings.Repeat("x", 512<<10), "deep": deep}})
	require.NoError(t, err)

	var decoded arguments
	require.NoError(t, decodeImportArguments(json.RawMessage(raw), &decoded))
	require.NotNil(t, decoded.Value)
}

func TestImportArgumentDecodingRetainsStrictStructureChecks(t *testing.T) {
	type arguments struct {
		Value string `json:"value"`
	}
	var decoded arguments
	require.EqualError(t, decodeImportArguments(json.RawMessage(`{"value":"one","value":"two"}`), &decoded), "query_evidence_invalid")
	require.EqualError(t, decodeImportArguments(json.RawMessage(`{"value":"one","unknown":true}`), &decoded), "import_arguments_invalid")
}

func TestImportToolOutputsAddContextAndPreserveDiagnosticCodes(t *testing.T) {
	_, _, err := PrepareImportToolOutput(context.Background(), nil, PrepareImportInput{})
	require.EqualError(t, err, "preparing the import selection: prepare_input_invalid")
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, prepareInputInvalidCode, code)

}

func TestPrepareImportBlockedActionsAreStateSpecific(t *testing.T) {
	tests := []struct {
		code, required, forbidden string
	}{
		{"target_mismatch", "intended target workspace", "change workspace mode"},
		{"target_execution_mode_not_hcp", "supported HCP execution mode", "change workspace mode automatically"},
		{"target_terraform_below_floor", "Terraform-version compatibility", "change the workspace version"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			action := prepareImportBlockedAction(test.code)
			require.Contains(t, action, test.required)
			if test.code == "target_mismatch" {
				require.NotContains(t, action, test.forbidden)
			} else {
				require.Contains(t, action, "do not")
				require.Contains(t, action, test.forbidden)
			}
			require.NotContains(t, action, "terraform apply")
		})
	}
}
