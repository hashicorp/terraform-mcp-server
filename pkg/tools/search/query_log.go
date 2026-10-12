// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hashicorp/go-tfe"
)

// Query logs are complete only after reading the entire bounded artifact.
// A ranged go-tfe LogReader does one body.Read per HTTP request, even when
// more data is available in that response. Reading the entire response body
// here avoids making another request for every small transport read.
const maxQueryLogRequests = 256
const maxQueryLogReadTime = 30 * time.Second

func readFastQueryLog(ctx context.Context, c *tfe.Client, location string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, maxQueryLogReadTime)
	defer cancel()
	u, err := url.Parse(location)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
		return nil, queryEvidenceError("query_log_unavailable")
	}
	// Allow HTTP only for a loopback HCP/TFE endpoint (local test deployments).
	// Do not let a remote API supply an unencrypted log capability.
	base := c.BaseURL()
	if u.Scheme == "http" && (base.Scheme != "http" || u.Host != base.Host || u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
		return nil, queryEvidenceError("query_log_unavailable")
	}
	const rawLimit = maxQueryEvidenceBytes + 3 // STX, ETX, and one overflow byte
	var raw []byte
	for range maxQueryLogRequests {
		remaining := rawLimit - len(raw)
		if remaining <= 0 {
			return nil, queryEvidenceError("query_evidence_size_limit")
		}
		query := u.Query()
		query.Set("limit", strconv.Itoa(remaining))
		query.Set("offset", strconv.Itoa(len(raw)))
		u.RawQuery = query.Encode()
		req, err := c.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, queryEvidenceError("query_log_unavailable")
		}
		// NewRequest attaches the API bearer, optional shared secret and client
		// forwarding headers. Signed log capabilities need none of them.
		for header := range req.Header {
			delete(req.Header, header)
		}
		buf := &queryBoundedWriter{limit: remaining}
		if err := req.Do(ctx, buf); err != nil {
			if buf.overLimit {
				return nil, queryEvidenceError("query_evidence_size_limit")
			}
			return nil, queryEvidenceError("query_log_unavailable")
		}
		chunk := buf.buffer.Bytes()
		if len(chunk) == 0 {
			if len(raw) == 0 || raw[0] == 2 && raw[len(raw)-1] != 3 {
				return nil, queryEvidenceError("query_log_unavailable")
			}
			return stripQueryLogMarkers(raw), nil
		}
		raw = append(raw, chunk...)
		if len(raw) >= rawLimit {
			return nil, queryEvidenceError("query_evidence_size_limit")
		}
		if raw[0] == 2 && raw[len(raw)-1] == 3 {
			return stripQueryLogMarkers(raw), nil
		}
	}
	return nil, queryEvidenceError("query_log_unavailable")
}

func stripQueryLogMarkers(data []byte) []byte {
	if len(data) == 0 || data[0] != 2 {
		return data
	}
	data = data[1:]
	if len(data) > 0 && data[len(data)-1] == 3 {
		return data[:len(data)-1]
	}
	return data
}
