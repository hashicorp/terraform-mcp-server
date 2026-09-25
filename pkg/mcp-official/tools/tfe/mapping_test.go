// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertMapsAllAttributes fails when upstream has a jsonapi attr or primary field
// that ours doesn't. relations are skipped since we flatten those on purpose.
// skip is for fields we're leaving out intentionally, value is the reason.
// this will help us catch drift when go-tfe adds a field we haven't mapped yet.
func assertMapsAllAttributes(t *testing.T, source, target reflect.Type, skip map[string]string) {
	t.Helper()

	targetFields := make(map[string]bool, target.NumField())
	for i := 0; i < target.NumField(); i++ {
		targetFields[target.Field(i).Name] = true
	}

	for i := 0; i < source.NumField(); i++ {
		field := source.Field(i)
		tag := field.Tag.Get("jsonapi")
		if !strings.HasPrefix(tag, "attr,") && !strings.HasPrefix(tag, "primary,") {
			continue
		}
		if _, skipped := skip[field.Name]; skipped {
			continue
		}
		assert.True(t, targetFields[field.Name],
			"%s.%s is not mapped in %s; add it or add it to the skip list with a reason",
			source.Name(), field.Name, target.Name())
	}
}
