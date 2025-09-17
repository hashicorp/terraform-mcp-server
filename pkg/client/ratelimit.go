// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	GlobalLimit     rate.Limit // Global requests per second
	GlobalBurst     int        // Global burst capacity
	PerSessionLimit rate.Limit // Per-session requests per second
	PerSessionBurst int        // Per-session burst capacity
}

// DefaultRateLimitConfig returns a sensible default configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		GlobalLimit:     rate.Every(time.Second / 10), // 10 requests per second
		GlobalBurst:     20,
		PerSessionLimit: rate.Every(time.Second / 5), // 5 requests per second per session
		PerSessionBurst: 10,
	}
}

// LoadRateLimitConfigFromEnv loads rate limiting configuration from environment variables
func LoadRateLimitConfigFromEnv() RateLimitConfig {
	config := DefaultRateLimitConfig()

	// Global rate limiting (format: "rps:burst")
	if globalLimit := os.Getenv("MCP_RATE_LIMIT_GLOBAL"); globalLimit != "" {
		if rps, burst := parseRateLimit(globalLimit); rps > 0 && burst > 0 {
			config.GlobalLimit = rate.Limit(rps)
			config.GlobalBurst = burst
			log.Infof("Global rate limit set to %f rps with burst %d", rps, burst)
		} else {
			log.Warnf("Invalid MCP_RATE_LIMIT_GLOBAL format, using default %f rps with burst %d", config.GlobalLimit, config.GlobalBurst)
		}
	}

	// Per-session rate limiting (format: "rps:burst")
	if sessionLimit := os.Getenv("MCP_RATE_LIMIT_SESSION"); sessionLimit != "" {
		if rps, burst := parseRateLimit(sessionLimit); rps > 0 && burst > 0 {
			config.PerSessionLimit = rate.Limit(rps)
			config.PerSessionBurst = burst
			log.Infof("Per-session rate limit set to %f rps with burst %d", rps, burst)
		} else {
			log.Warnf("Invalid MCP_RATE_LIMIT_SESSION format, using default %f rps with burst %d", config.PerSessionLimit, config.PerSessionBurst)
		}
	}

	return config
}

// parseRateLimit parses "rps:burst" format
func parseRateLimit(limit string) (float64, int) {
	parts := strings.Split(limit, ":")
	if len(parts) != 2 {
		return 0, 0
	}

	rps, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	burst, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))

	if err1 != nil || err2 != nil {
		return 0, 0
	}

	return rps, burst
}

// RateLimitMiddleware creates a comprehensive rate limiting middleware
type RateLimitMiddleware struct {
	config          RateLimitConfig
	globalLimiter   *rate.Limiter
	sessionLimiters map[string]*rate.Limiter
	mu              sync.RWMutex
	logger          *log.Logger
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(config RateLimitConfig, logger *log.Logger) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		config:          config,
		globalLimiter:   rate.NewLimiter(config.GlobalLimit, config.GlobalBurst),
		sessionLimiters: make(map[string]*rate.Limiter),
		logger:          logger,
	}
}

// getSessionLimiter gets or creates a rate limiter for a session
func (m *RateLimitMiddleware) getSessionLimiter(sessionID string) *rate.Limiter {
	m.mu.RLock()
	limiter, exists := m.sessionLimiters[sessionID]
	m.mu.RUnlock()

	if exists {
		return limiter
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := m.sessionLimiters[sessionID]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(m.config.PerSessionLimit, m.config.PerSessionBurst)
	m.sessionLimiters[sessionID] = limiter
	return limiter
}

// Middleware returns the tool handler middleware function
func (m *RateLimitMiddleware) Middleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			toolName := request.Params.Name

			// Check global rate limit
			if !m.globalLimiter.Allow() {
				m.logger.Warnf("Global rate limit exceeded for tool: %s", toolName)
				return nil, errors.New("rate limit exceeded: too many requests globally")
			}

			// Check per-session rate limit if we can get session ID from context
			if sessionID := getSessionIDFromContext(ctx); sessionID != "" {
				sessionLimiter := m.getSessionLimiter(sessionID)
				if !sessionLimiter.Allow() {
					m.logger.Warnf("Session rate limit exceeded for session: %s, tool: %s", sessionID, toolName)
					return nil, errors.New("rate limit exceeded: too many requests from this session")
				}
			}

			m.logger.Debugf("Rate limit check passed for tool: %s", toolName)
			return next(ctx, request)
		}
	}
}

// getSessionIDFromContext extracts session ID from context
// This is a helper function that tries to get session ID from the context
func getSessionIDFromContext(ctx context.Context) string {
	// Try to get session from context
	if session := server.ClientSessionFromContext(ctx); session != nil {
		return session.SessionID()
	}
	return ""
}

// CleanupSessions removes inactive session limiters to prevent memory leaks
func (m *RateLimitMiddleware) CleanupSessions(activeSessions []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	activeSet := make(map[string]bool)
	for _, sessionID := range activeSessions {
		activeSet[sessionID] = true
	}

	for sessionID := range m.sessionLimiters {
		if !activeSet[sessionID] {
			delete(m.sessionLimiters, sessionID)
			m.logger.Debugf("Cleaned up rate limiter for inactive session: %s", sessionID)
		}
	}
}
