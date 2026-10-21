// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"fmt"
	"regexp"
	"sort"

	version "github.com/hashicorp/go-version"
	tfaddr "github.com/hashicorp/terraform-registry-address"
)

var exactLockedVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

var minimumImportBlockVersion = version.Must(version.NewVersion("1.5.0"))
var minimumResourceIdentityVersion = version.Must(version.NewVersion("1.12.0"))

func canonicalQueryProviderSource(namespace, providerType string) (string, error) {
	namespace, err := tfaddr.ParseProviderPart(namespace)
	if err != nil {
		return "", searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	providerType, err = tfaddr.ParseProviderPart(providerType)
	if err != nil {
		return "", searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	provider := tfaddr.NewProvider(tfaddr.DefaultProviderRegistryHost, namespace, providerType)
	if err := tfaddr.ValidateProviderAddress(provider.String()); err != nil {
		return "", searchDiagnosticError(queryProviderSourceUnresolvedCode)
	}
	return provider.String(), nil
}

func canonicalizeLockedProviders(locked map[string]string) (map[string]string, error) {
	sources := make([]string, 0, len(locked))
	for source := range locked {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	canonical := make(map[string]string, len(locked))
	for _, source := range sources {
		version := locked[source]
		if len(source) > 256 {
			return nil, invalidLockedProviderSource(source)
		}
		provider, err := tfaddr.ParseProviderSource(source)
		if err != nil {
			return nil, invalidLockedProviderSource(source)
		}
		if !provider.HasKnownNamespace() {
			detail := "locked_providers provider sources require a namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl"
			if safeProviderSourceForDiagnostic(source) {
				detail = fmt.Sprintf("locked_providers key %q lacks a provider namespace; use public namespace/type shorthand or the complete hostname/namespace/type address from .terraform.lock.hcl", source)
			}
			return nil, searchDiagnosticErrorWithDetail(lockedProviderSourceInvalidCode, detail)
		}
		if err := provider.Validate(); err != nil {
			return nil, invalidLockedProviderSource(source)
		}
		if len(version) > 128 || !exactLockedVersion.MatchString(version) {
			return nil, searchDiagnosticErrorWithDetail(lockVersionsInvalidCode, fmt.Sprintf("locked_providers[%q] must contain the exact selected version from .terraform.lock.hcl, not a constraint", provider.String()))
		}
		canonicalSource := provider.String()
		if previous, exists := canonical[canonicalSource]; exists && previous != version {
			return nil, searchDiagnosticErrorWithDetail(lockVersionsInvalidCode, fmt.Sprintf("multiple locked_providers entries resolve to %q but report different exact versions", canonicalSource))
		}
		canonical[canonicalSource] = version
	}
	return canonical, nil
}

func invalidLockedProviderSource(source string) error {
	detail := "locked_providers key is not a valid provider source; use public namespace/type shorthand or a complete hostname/namespace/type address from .terraform.lock.hcl"
	if safeProviderSourceForDiagnostic(source) {
		detail = fmt.Sprintf("locked_providers key %q is not a valid provider source; use public namespace/type shorthand or a complete hostname/namespace/type address from .terraform.lock.hcl", source)
	}
	return searchDiagnosticErrorWithDetail(lockedProviderSourceInvalidCode, detail)
}

func safeProviderSourceForDiagnostic(source string) bool {
	if source == "" || len(source) > 256 {
		return false
	}
	for _, r := range source {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func terraformImportSyntaxVersion(setting string) (old, known bool) {
	return terraformVersionBefore(setting, minimumImportBlockVersion)
}

func terraformVersionBefore(setting string, minimum *version.Version) (bool, bool) {
	parsed, err := version.NewSemver(setting)
	if err != nil {
		return false, false
	}
	// This is a feature-line comparison: prerelease Terraform builds from a
	// core release that contains the feature qualify even though SemVer orders
	// them before the final release. For example, 1.5.0-beta has import-block
	// syntax and 1.12.0-dev has resource identity support.
	return parsed.Core().LessThan(minimum.Core()), true
}
