// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	log "github.com/sirupsen/logrus"
)

const PROVIDER_BASE_PATH = "registry://providers"

func ExtractProviderNameAndVersion(uri string) (string, string, string) {
	uri = strings.TrimPrefix(uri, fmt.Sprintf("%s/", PROVIDER_BASE_PATH))
	parts := strings.Split(uri, "/")
	return parts[0], parts[2], parts[4]
}

func ConstructProviderVersionURI(providerNamespace interface{}, providerName string, providerVersion interface{}) string {
	return fmt.Sprintf("%s/%s/providers/%s/versions/%s", PROVIDER_BASE_PATH, providerNamespace, providerName, providerVersion)
}

// ContainsSlug checks if the sourceName string contains the slug string anywhere within it.
// It safely handles potential regex metacharacters in the slug.
func ContainsSlug(sourceName string, slug string) (bool, error) {
	// Use regexp.QuoteMeta to escape any special regex characters in the slug.
	// This ensures the slug is treated as a literal string in the pattern.
	escapedSlug := regexp.QuoteMeta(slug)

	// Construct the regex pattern dynamically: ".*" + escapedSlug + ".*"
	// This pattern means "match any characters, then the escaped slug, then any characters".
	pattern := ".*" + escapedSlug + ".*"

	// regexp.MatchString compiles and runs the regex against the sourceName.
	// It returns true if a match is found, false otherwise.
	// It also returns an error if the pattern is invalid (unlikely here due to QuoteMeta).
	matched, err := regexp.MatchString(pattern, sourceName)
	if err != nil {
		fmt.Printf("Error compiling or matching regex pattern '%s': %v\n", pattern, err)
		return false, err // Propagate the error
	}

	return matched, nil
}

// IsValidProviderVersionFormat checks if the provider version format is valid.
func IsValidProviderVersionFormat(version string) bool {
	// Example regex for semantic versioning (e.g., "1.0.0", "1.0.0-beta").
	semverRegex := `^v?(\d+\.\d+\.\d+(-[a-zA-Z0-9]+)?)$`
	matched, _ := regexp.MatchString(semverRegex, version)
	return matched
}

func IsValidProviderDataType(providerDataType string) bool {
	validTypes := []string{"resources", "data-sources", "functions", "guides", "overview"}
	return slices.Contains(validTypes, providerDataType)
}

// LogAndReturnError logs the error with context and returns a formatted error.
func LogAndReturnError(logger *log.Logger, context string, err error) error {
	err = fmt.Errorf("%s, %w", context, err)
	if logger != nil {
		logger.Errorf("Error in %s, %v", context, err)
	}
	return err
}

func IsV2ProviderDataType(dataType string) bool {
	v2Categories := []string{"guides", "functions", "overview"}
	return slices.Contains(v2Categories, dataType)
}

func ExtractReadme(readme string) string {
	if readme == "" {
		return ""
	}

	var builder strings.Builder
	headerFound := false
	strArr := strings.Split(readme, "\n")
	headerRegex := regexp.MustCompile(`^#+\s?`)
	for _, str := range strArr {
		matched := headerRegex.MatchString(str)
		if matched {
			if headerFound {
				break
			}
			headerFound = true
		}
		builder.WriteString(str)
		builder.WriteString("\n")
	}

	return strings.TrimSuffix(builder.String(), "\n")
}
