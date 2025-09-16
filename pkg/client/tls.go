// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"
)

type TLSConfig struct {
	CertFile string
	KeyFile  string
	Config   *tls.Config
}

// GetTLSConfigFromEnv loads TLS cert/key file paths from environment variables
func GetTLSConfigFromEnv() (*TLSConfig, error) {
	certFile := os.Getenv("MCP_TLS_CERT_FILE")
	keyFile := os.Getenv("MCP_TLS_KEY_FILE")

	if certFile == "" && keyFile == "" {
		return nil, nil
	}

	if certFile == "" {
		return nil, fmt.Errorf("MCP_TLS_CERT_FILE is required when MCP_TLS_KEY_FILE is set")
	}

	if keyFile == "" {
		return nil, fmt.Errorf("MCP_TLS_KEY_FILE is required when MCP_TLS_CERT_FILE is set")
	}

	// Validate certificate files exist and are readable
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("TLS certificate file does not exist: %s", certFile)
	} else if err != nil {
		return nil, fmt.Errorf("cannot access TLS certificate file %s: %w", certFile, err)
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("TLS key file does not exist: %s", keyFile)
	} else if err != nil {
		return nil, fmt.Errorf("cannot access TLS key file %s: %w", keyFile, err)
	}

	// Validate certificate and key can be loaded
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		return nil, fmt.Errorf("invalid TLS certificate/key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
			tls.X25519MLKEM768,
		},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}

	return &TLSConfig{
		Config:   tlsConfig,
		CertFile: certFile,
		KeyFile:  keyFile,
	}, nil
}

func IsLocalHost(host string) bool {
	h := strings.ToLower(host)
	return h == "localhost" ||
		h == "127.0.0.1" ||
		h == "::1" ||
		h == "[::1]" ||
		h == "0.0.0.0"
}
