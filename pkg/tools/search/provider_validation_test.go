// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalQueryProviderSource(t *testing.T) {
	for _, test := range []struct {
		namespace, providerType, want string
	}{
		{"hashicorp", "aws", "registry.terraform.io/hashicorp/aws"},
		{"HashiCorp", "AWS", "registry.terraform.io/hashicorp/aws"},
		{"datadog", "datadog", "registry.terraform.io/datadog/datadog"},
	} {
		t.Run(test.namespace+"/"+test.providerType, func(t *testing.T) {
			source, err := canonicalQueryProviderSource(test.namespace, test.providerType)
			require.NoError(t, err)
			require.Equal(t, test.want, source)
		})
	}
}

func TestCanonicalQueryProviderSourceRejectsInvalidCatalogPartsWithoutReflection(t *testing.T) {
	for _, test := range []struct {
		name, namespace, providerType string
	}{
		{name: "empty namespace", namespace: "", providerType: "aws"},
		{name: "namespace with slash", namespace: "unsafe/namespace", providerType: "aws"},
		{name: "type with dot", namespace: "hashicorp", providerType: "unsafe.type"},
		{name: "type with control character", namespace: "hashicorp", providerType: "unsafe\ntype"},
		{name: "reserved type prefix", namespace: "hashicorp", providerType: "terraform-example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, err := canonicalQueryProviderSource(test.namespace, test.providerType)
			require.Empty(t, source)
			require.EqualError(t, err, "query_provider_source_unresolved")
			if test.namespace != "" {
				require.NotContains(t, err.Error(), test.namespace)
			}
			if test.providerType != "" {
				require.NotContains(t, err.Error(), test.providerType)
			}
		})
	}
}

// These provider-only preflight cases do not require preparation inputs or a backend.
func TestCanonicalizeLockedProvidersSourceDiagnostics(t *testing.T) {
	for _, test := range []struct {
		source, want, version string
	}{
		{"registry.terraform.io/hashicorp/aws", "registry.terraform.io/hashicorp/aws", "6.18.0"},
		{"providers.example.com/team/aws", "providers.example.com/team/aws", "6.18.0"},
		{"hashicorp/aws", "registry.terraform.io/hashicorp/aws", "6.18.0"},
		{"datadog/datadog", "registry.terraform.io/datadog/datadog", "3.0.0"},
	} {
		t.Run(test.source, func(t *testing.T) {
			locked := map[string]string{test.source: test.version}
			canonical, err := canonicalizeLockedProviders(locked)
			require.NoError(t, err)
			require.Equal(t, map[string]string{test.want: test.version}, canonical)
			require.Equal(t, map[string]string{test.source: test.version}, locked, "caller input is not mutated")
		})
	}

	_, err := canonicalizeLockedProviders(map[string]string{"aws": "6.18.0"})
	require.EqualError(t, err, `locked_provider_source_invalid: locked_providers key "aws" lacks a provider namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl`)
	code, ok := searchDiagnosticCodeFromError(err)
	require.True(t, ok)
	require.Equal(t, lockedProviderSourceInvalidCode, code)

	locked := map[string]string{
		"hashicorp/aws":                       "6.18.0",
		"registry.terraform.io/hashicorp/aws": "6.18.0",
	}
	canonical, err := canonicalizeLockedProviders(locked)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"registry.terraform.io/hashicorp/aws": "6.18.0"}, canonical)

	locked["hashicorp/aws"] = "6.19.0"
	canonical, err = canonicalizeLockedProviders(locked)
	require.Nil(t, canonical)
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, `resolve to "registry.terraform.io/hashicorp/aws"`)
	require.ErrorContains(t, err, "different exact versions")

	_, err = canonicalizeLockedProviders(map[string]string{"unsafe\nsource": "6.18.0"})
	require.ErrorContains(t, err, "locked_provider_source_invalid")
	require.NotContains(t, err.Error(), "unsafe")
	require.NotContains(t, err.Error(), "\n")

	_, err = canonicalizeLockedProviders(map[string]string{"registry.terraform.io/hashicorp/aws": ">= 6.0"})
	require.ErrorContains(t, err, "lock_versions_invalid")
	require.ErrorContains(t, err, "exact selected version")
}

func TestCanonicalizeLockedProvidersExactVersions(t *testing.T) {
	for _, test := range []struct {
		name, version string
		valid         bool
	}{
		{"release", "6.18.0", true},
		{"zero", "0.0.0", true},
		{"prerelease", "6.18.0-beta.1", true},
		{"leading zero preserved", "06.018.0", true},
		{"regex accepted prerelease preserved", "6.18.0-beta..1", true},
		{"128 byte version", "1.2.3-" + strings.Repeat("a", 122), true},
		{"129 byte version", "1.2.3-" + strings.Repeat("a", 123), false},
		{"empty", "", false},
		{"constraint", ">= 6.0", false},
		{"pessimistic constraint", "~> 6.18", false},
		{"partial version", "6.18", false},
		{"v prefix", "v6.18.0", false},
		{"build metadata", "6.18.0+build.1", false},
		{"leading whitespace", " 6.18.0", false},
		{"trailing whitespace", "6.18.0 ", false},
		{"trailing newline", "6.18.0\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			canonical, err := canonicalizeLockedProviders(map[string]string{"hashicorp/aws": test.version})
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, map[string]string{"registry.terraform.io/hashicorp/aws": test.version}, canonical)
				return
			}
			require.Nil(t, canonical)
			require.ErrorContains(t, err, "exact selected version")
			code, ok := searchDiagnosticCodeFromError(err)
			require.True(t, ok)
			require.Equal(t, lockVersionsInvalidCode, code)
		})
	}
}

func TestCanonicalizeLockedProvidersEmptyAndInvalidSources(t *testing.T) {
	for _, locked := range []map[string]string{nil, {}} {
		canonical, err := canonicalizeLockedProviders(locked)
		require.NoError(t, err, "presence requirements belong to callers")
		require.Equal(t, map[string]string{}, canonical)
	}
	for _, source := range []string{"", "bad source/aws", "hashicorp/terraform-example", strings.Repeat("a", 257)} {
		t.Run(source, func(t *testing.T) {
			canonical, err := canonicalizeLockedProviders(map[string]string{source: "6.18.0"})
			require.Nil(t, canonical)
			require.Error(t, err)
			code, ok := searchDiagnosticCodeFromError(err)
			require.True(t, ok)
			require.Equal(t, lockedProviderSourceInvalidCode, code)
		})
	}
	_, err := canonicalizeLockedProviders(map[string]string{"z": "6.18.0", "a": "6.18.0"})
	require.ErrorContains(t, err, `key "a" lacks a provider namespace`, "diagnostics follow sorted sources")
}

func TestSafeProviderSourceForDiagnostic(t *testing.T) {
	for _, test := range []struct {
		name, source string
		safe         bool
	}{
		{"empty", "", false},
		{"shorthand", "hashicorp/aws", true},
		{"printable invalid source", "bad source/aws", true},
		{"256 bytes", strings.Repeat("a", 256), true},
		{"257 bytes", strings.Repeat("a", 257), false},
		{"newline", "unsafe\nsource", false},
		{"null", "unsafe\x00source", false},
		{"delete", "unsafe\x7fsource", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.safe, safeProviderSourceForDiagnostic(test.source))
			err := invalidLockedProviderSource(test.source)
			code, ok := searchDiagnosticCodeFromError(err)
			require.True(t, ok)
			require.Equal(t, lockedProviderSourceInvalidCode, code)
			if test.safe {
				require.Contains(t, err.Error(), test.source)
			} else {
				require.EqualError(t, err, "locked_provider_source_invalid: locked_providers key is not a valid provider source; use public namespace/type shorthand or a complete hostname/namespace/type address from .terraform.lock.hcl")
			}
		})
	}
}

func TestFoundationTerraformImportSyntaxVersionPrereleaseAndMalformed(t *testing.T) {
	for _, tt := range []struct {
		version    string
		old, known bool
	}{
		{"1.14.0", false, true},
		{"v1.17.0-dev.20261012+build.1", false, true},
		{"1.17.0-alpha.1", false, true},
		{"1.17.0-beta.1", false, true},
		{"1.17.0-rc.1", false, true},
		{"1.4.9-dev.1", true, true},
		{"1.4.9-alpha.1", true, true},
		{"1.4.9-beta.1", true, true},
		{"1.4.9-rc.1", true, true},
		{"1.5.0-alpha.1", false, true},
		{"1.5.0-alpha20230517", false, true},
		{"1.5.0-beta.1", false, true},
		{"1.5.0-beta1", false, true},
		{"1.5.0-rc.1", false, true},
		{"1.5.0-rc1", false, true},
		{"1.5.0-dev.1", false, true},
		{"1.5.0-dev", false, true},
		{"1.5.0", false, true},
		{"1.17.0 arbitrary", false, false},
		{"1.17.0-", false, false},
		{"1.17.0/secret", false, false},
		{"1.999999999999999999999999999.0", false, false},
	} {
		t.Run(tt.version, func(t *testing.T) {
			old, known := terraformImportSyntaxVersion(tt.version)
			require.Equal(t, tt.old, old)
			require.Equal(t, tt.known, known)
		})
	}
}

func TestFoundationTerraformVersionBeforeResourceIdentityPrereleases(t *testing.T) {
	for _, tt := range []struct {
		version    string
		old, known bool
	}{
		{"1.11.9-rc.1", true, true},
		{"1.12.0-alpha.1", false, true},
		{"1.12.0-beta.1", false, true},
		{"1.12.0-rc.1", false, true},
		{"1.12.0-dev.1", false, true},
		{"1.12.0", false, true},
		{"~> 1.12", false, false},
		{"latest", false, false},
	} {
		t.Run(tt.version, func(t *testing.T) {
			old, known := terraformVersionBefore(tt.version, minimumResourceIdentityVersion)
			require.Equal(t, tt.old, old)
			require.Equal(t, tt.known, known)
		})
	}
}
