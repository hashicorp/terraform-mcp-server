// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package profiling

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := log.New()
	logger.SetOutput(&buf)
	return logger, &buf
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestStartServesPprof(t *testing.T) {
	logger, logs := testLogger()
	server, err := start("127.0.0.1:0", logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	// start doesn't hand back the listener, so read the port from the log line
	require.Contains(t, logs.String(), "pprof listening on http://127.0.0.1:")
	addr := addrFromLog(t, logs.String())

	t.Run("index lists the profiles", func(t *testing.T) {
		status, body := get(t, "http://"+addr+"/debug/pprof/")
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, "goroutine")
		assert.Contains(t, body, "heap")
	})

	t.Run("named profiles work", func(t *testing.T) {
		status, body := get(t, "http://"+addr+"/debug/pprof/goroutine?debug=1")
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, "goroutine profile")
	})

	t.Run("cpu profile works with the write timeout set", func(t *testing.T) {
		status, _ := get(t, "http://"+addr+"/debug/pprof/profile?seconds=1")
		assert.Equal(t, http.StatusOK, status)
	})

	t.Run("loopback doesn't warn", func(t *testing.T) {
		assert.NotContains(t, logs.String(), "isn't loopback")
	})
}

func TestStartBadAddress(t *testing.T) {
	logger, _ := testLogger()
	_, err := start("not-an-address", logger)
	assert.Error(t, err)
}

func TestStartPortInUse(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })

	logger, _ := testLogger()
	_, err = start(taken.Addr().String(), logger)
	assert.Error(t, err)
}

func TestStartIsOffByDefault(t *testing.T) {
	t.Setenv(AddrEnv, "")
	logger, logs := testLogger()
	Start(logger)
	assert.Empty(t, logs.String())
}

func TestStartLogsInsteadOfFailing(t *testing.T) {
	t.Setenv(AddrEnv, "not-an-address")
	logger, logs := testLogger()
	assert.NotPanics(t, func() { Start(logger) })
	assert.Contains(t, logs.String(), "continuing without profiling")
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"10.0.0.5", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			assert.Equal(t, tt.want, isLoopback(&net.TCPAddr{IP: net.ParseIP(tt.ip)}))
		})
	}
}

// addrFromLog pulls host:port out of the "pprof listening on" log line.
func addrFromLog(t *testing.T, logs string) string {
	t.Helper()
	const prefix = "pprof listening on http://"
	i := bytes.Index([]byte(logs), []byte(prefix))
	require.NotEqual(t, -1, i)
	rest := logs[i+len(prefix):]
	end := bytes.Index([]byte(rest), []byte("/debug/pprof/"))
	require.NotEqual(t, -1, end)
	return rest[:end]
}
