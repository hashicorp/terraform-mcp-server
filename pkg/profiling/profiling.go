// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

// Package profiling runs Go's pprof endpoints on a separate port when
// MCP_PPROF_ADDR is set, so we can pull CPU and memory profiles off a running
// server without putting them on the MCP port.
package profiling

import (
	"errors"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// AddrEnv is where the pprof server listens, like 127.0.0.1:6060. Leave it
// unset and profiling stays off.
const AddrEnv = "MCP_PPROF_ADDR"

// Start runs pprof on MCP_PPROF_ADDR if it's set, otherwise it does nothing.
// If the address is bad or the port is taken we just log it and keep going
// without profiling. It's a debug tool, it shouldn't take the server down.
func Start(logger *log.Logger) {
	addr := strings.TrimSpace(os.Getenv(AddrEnv))
	if addr == "" {
		return
	}
	if _, err := start(addr, logger); err != nil {
		logger.WithError(err).Errorf("Failed to start pprof server on %s, continuing without profiling", addr)
	}
}

// start listens on addr and serves pprof in the background. It listens first
// so a bad address shows up right away instead of later.
func start(addr string, logger *log.Logger) (*http.Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	if !isLoopback(listener.Addr()) {
		logger.Warnf("pprof is listening on %s, which isn't loopback. Profiles show internals of the running server, so only expose this on a trusted network", listener.Addr())
	}

	server := &http.Server{
		Handler:           handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout here, /debug/pprof/profile and /debug/pprof/trace
		// keep streaming for however many seconds you ask for.
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.WithError(err).Error("pprof server stopped")
		}
	}()

	logger.Infof("pprof listening on http://%s/debug/pprof/", listener.Addr())
	return server, nil
}

// handler puts pprof on its own mux. Importing net/http/pprof also registers
// these on http.DefaultServeMux, but we never serve that mux, so this is the
// only place they're reachable.
func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func isLoopback(addr net.Addr) bool {
	tcpAddr, ok := addr.(*net.TCPAddr)
	return ok && tcpAddr.IP.IsLoopback()
}
