// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package utils

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/go-tfe"
	log "github.com/sirupsen/logrus"
)

const PROVIDER_BASE_PATH = "registry://providers"

// ExtractProviderNameAndVersion parses a provider URI and extracts the provider namespace, name and version.
// The URI is expected to have at least 5 segments separated by '/', if invalid, an error is returned.
// Example format: registry://providers/<provider_namespace>/namespace/<provider_name>/version/<provider_version>
func ExtractProviderNameAndVersion(uri string) (string, string, string, error) {
	parts := strings.Split(uri, "/")
	if len(parts) < 5 {
		return "", "", "", fmt.Errorf("invalid provider URI format")
	}
	return parts[len(parts)-5], parts[len(parts)-3], parts[len(parts)-1], nil
}

func ConstructProviderVersionURI(providerNamespace string, providerName string, providerVersion string) string {
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

func IsValidProviderDocumentType(providerDocumentType string) bool {
	validTypes := []string{"resources", "data-sources", "functions", "guides", "overview", "actions", "list-resources"}
	return slices.Contains(validTypes, providerDocumentType)
}

// LogAndReturnError logs the error with context and returns a formatted error.
func LogAndReturnError(logger *log.Logger, context string, err error) error {
	err = fmt.Errorf("%s, %w", context, err)
	if logger != nil {
		logger.Errorf("Error in %s, %v", context, err)
	}
	return err
}

func IsV2ProviderDocumentType(dataType string) bool {
	v2Categories := []string{"guides", "functions", "overview", "actions", "list-resources"}
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

// GetEnv retrieves the value of an environment variable or returns a fallback value if not set
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// This function is used for custom GET requests using the TFE client.
func MakeCustomGetRequestRaw(ctx context.Context, client *tfe.Client, path string, additionalQueryParams map[string][]string) ([]byte, error) {
	req, err := client.NewRequestWithAdditionalQueryParams("GET", path, nil, additionalQueryParams)
	if err != nil {
		return nil, err
	}

	respBody, err := req.DoRaw(ctx)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	body, err := io.ReadAll(respBody)
	if err != nil {
		return nil, err
	}

	return body, nil
}
