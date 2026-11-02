// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package mcpofficial

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/tools/middleware"
)

// UnauthorizedStatus sends a 401 when a tool call fails because HCPT or TFE
// rejected the token. Tool errors normally come back as a 200 with isError set,
// so without this a client has to read the body to know the token expired. The
// MCP auth spec says expired or invalid tokens should get a 401.
//
// It checks the first write from the SDK, which for a tool call is the result.
// The headers haven't gone out at that point, so if APIErrors labeled the
// result unauthorized we can still swap the 200 for a 401. If the tool already
// streamed something, like an elicitation prompt, the 200 is out and the client
// just gets the error_code in _meta.
func UnauthorizedStatus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&unauthorizedStatusWriter{ResponseWriter: w}, r)
	})
}

type unauthorizedStatusWriter struct {
	http.ResponseWriter
	// pendingOK is set when the handler asked for a 200 but nothing has been
	// written yet, so we can still change it.
	pendingOK   bool
	wroteHeader bool
}

func (w *unauthorizedStatusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	if code == http.StatusOK {
		w.pendingOK = true
		return
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *unauthorizedStatusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		if isUnauthorizedToolResult(b) {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			w.ResponseWriter.WriteHeader(http.StatusUnauthorized)
		} else if w.pendingOK {
			w.ResponseWriter.WriteHeader(http.StatusOK)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *unauthorizedStatusWriter) Flush() {
	if !w.wroteHeader && w.pendingOK {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *unauthorizedStatusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// isUnauthorizedToolResult checks if written is a tool result that APIErrors
// labeled unauthorized. It's either an SSE event or a plain JSON body.
func isUnauthorizedToolResult(b []byte) bool {
	payload := sseData(b)
	var msg struct {
		Result *struct {
			IsError bool           `json:"isError"`
			Meta    map[string]any `json:"_meta"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil || msg.Result == nil {
		return false
	}
	return msg.Result.IsError && msg.Result.Meta[middleware.MetaErrorCode] == middleware.APIErrorUnauthorized
}

// sseData pulls the data out of an SSE event. If written isn't one it's
// returned as is.
func sseData(b []byte) []byte {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return trimmed
	}
	var data [][]byte
	for _, line := range bytes.Split(b, []byte("\n")) {
		if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			data = append(data, bytes.TrimPrefix(value, []byte(" ")))
		}
	}
	return bytes.Join(data, []byte("\n"))
}
