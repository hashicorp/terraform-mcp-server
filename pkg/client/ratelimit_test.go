// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

func TestRateLimitMiddleware(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in tests

	// Create a very restrictive rate limit for testing
	config := RateLimitConfig{
		GlobalLimit:     rate.Every(time.Second), // 1 request per second
		GlobalBurst:     1,
		PerSessionLimit: rate.Every(time.Second), // 1 request per second per session
		PerSessionBurst: 1,
	}

	middleware := NewRateLimitMiddleware(config, logger)
	
	// Create a mock handler that always succeeds
	mockHandler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent("success"),
			},
		}, nil
	}

	// Wrap the handler with rate limiting middleware
	rateLimitedHandler := middleware.Middleware()(mockHandler)

	// Create a test request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "test_tool",
		},
	}

	ctx := context.Background()

	// First request should succeed
	result, err := rateLimitedHandler(ctx, request)
	if err != nil {
		t.Fatalf("First request should succeed, got error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Second request should be rate limited
	_, err = rateLimitedHandler(ctx, request)
	if err == nil {
		t.Fatal("Second request should be rate limited")
	}
	if err.Error() != "rate limit exceeded: too many requests globally" {
		t.Fatalf("Expected global rate limit error, got: %v", err)
	}
}

func TestLoadRateLimitConfigFromEnv(t *testing.T) {
	// Test default config
	config := LoadRateLimitConfigFromEnv()
	
	if config.GlobalLimit != rate.Every(time.Second/10) {
		t.Errorf("Expected default global limit of 10 RPS, got %v", config.GlobalLimit)
	}
	
	if config.GlobalBurst != 20 {
		t.Errorf("Expected default global burst of 20, got %d", config.GlobalBurst)
	}
}

func TestParseRateLimit(t *testing.T) {
	tests := []struct {
		input       string
		expectedRPS float64
		expectedBurst int
	}{
		{"10:20", 10.0, 20},
		{"5.5:15", 5.5, 15},
		{"1:1", 1.0, 1},
		{"invalid", 0, 0},
		{"10", 0, 0},
		{"10:20:30", 0, 0},
		{"", 0, 0},
	}

	for _, test := range tests {
		rps, burst := parseRateLimit(test.input)
		if rps != test.expectedRPS || burst != test.expectedBurst {
			t.Errorf("parseRateLimit(%q) = (%v, %v), expected (%v, %v)", 
				test.input, rps, burst, test.expectedRPS, test.expectedBurst)
		}
	}
}

func TestLoadRateLimitConfigFromEnvWithCustomValues(t *testing.T) {
	// Set environment variables
	t.Setenv("MCP_RATE_LIMIT_GLOBAL", "15:30")
	t.Setenv("MCP_RATE_LIMIT_SESSION", "8:16")

	config := LoadRateLimitConfigFromEnv()

	if config.GlobalLimit != rate.Limit(15) {
		t.Errorf("Expected global limit of 15 RPS, got %v", config.GlobalLimit)
	}
	
	if config.GlobalBurst != 30 {
		t.Errorf("Expected global burst of 30, got %d", config.GlobalBurst)
	}

	if config.PerSessionLimit != rate.Limit(8) {
		t.Errorf("Expected session limit of 8 RPS, got %v", config.PerSessionLimit)
	}
	
	if config.PerSessionBurst != 16 {
		t.Errorf("Expected session burst of 16, got %d", config.PerSessionBurst)
	}
}
