// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxImportEvidenceBytes = 4 * 1024 * 1024
const maxImportSchemaBytes = 32 * 1024 * 1024

type importEvidenceError struct{ Code string }

func (e *importEvidenceError) Error() string  { return e.Code }
func importEvidenceFailure(code string) error { return &importEvidenceError{Code: code} }
func importEvidenceDigest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

// Nil SDK errors for empty 204/307 responses are not acquired schema evidence.
func importSchemaHTTPStatus(status int) string {
	switch status {
	case http.StatusOK:
		return ""
	case http.StatusNoContent:
		return "schema_source_pending"
	case http.StatusTemporaryRedirect:
		return "schema_download_required"
	case http.StatusUnauthorized, http.StatusForbidden:
		return "schema_source_access_denied"
	case http.StatusNotFound:
		return "schema_source_unavailable"
	case http.StatusUnprocessableEntity:
		return "schema_source_cli_unsupported"
	default:
		return "schema_source_http_error"
	}
}

// Exact JSON numbers, duplicate-key rejection and bounded bytes/depth protect
// current backend evidence reads; decoding does not authenticate origin.
func decodeImportEvidenceJSON(raw []byte, target any) error {
	return decodeImportEvidenceJSONLimit(raw, target, maxImportEvidenceBytes)
}

func decodeImportEvidenceJSONLimit(raw []byte, target any, limit int) error {
	if len(raw) > limit {
		return importEvidenceFailure("evidence_size_limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return importEvidenceFailure("evidence_depth_limit")
		}
		token, err := d.Token()
		if err != nil {
			return importEvidenceFailure("evidence_json_invalid")
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter != '{' && delimiter != '[' {
				return importEvidenceFailure("evidence_json_invalid")
			}
			keys := map[string]bool{}
			for d.More() {
				if delimiter == '{' {
					key, err := d.Token()
					name, ok := key.(string)
					if err != nil || !ok || keys[name] {
						return importEvidenceFailure("evidence_json_invalid")
					}
					keys[name] = true
				}
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			if _, err := d.Token(); err != nil {
				return importEvidenceFailure("evidence_json_invalid")
			}
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return importEvidenceFailure("evidence_json_invalid")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return importEvidenceFailure("evidence_json_invalid")
	}
	return nil
}
