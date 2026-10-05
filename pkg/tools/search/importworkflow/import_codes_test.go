// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEveryImportCodeIsRegistered(t *testing.T) {
	emitted := emittedImportCodes(t)
	for _, code := range sortedKeys(emitted) {
		assert.Contains(t, importCodes, code, "%s is emitted in %s but missing from importCodes", code, emitted[code][0])
	}
	for code, meaning := range importCodes {
		assert.NotEmpty(t, meaning, code)
		assert.Contains(t, emitted, code, "%s is in importCodes but no longer emitted", code)
	}
}
