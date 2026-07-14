// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"
)

const OrganizationAllowlistEnv = "MCP_ORGANIZATION_ALLOWLIST"

const (
	// RemoteIPMethodEnv selects how the client IP is sourced for X-Forwarded-For.
	// The valid values are: "RemoteAddr" (default), "X-Real-IP", "X-Forwarded-For".
	RemoteIPMethodEnv = "MCP_REMOTE_IP_METHOD"

	// XFFTrustedHopsEnv sets the number of trusted proxy hops, counted from the
	// right of the X-Forwarded-For chain. Only used with the X-Forwarded-For method.
	XFFTrustedHopsEnv = "MCP_XFF_TRUSTED_HOPS"

	RemoteIPMethodRemoteAddr = "RemoteAddr"
	RemoteIPMethodXRealIP    = "X-Real-IP"
	RemoteIPMethodXFF        = "X-Forwarded-For"
)

var ErrMalformedOrganizationAllowlist = errors.New("malformed organization allowlist")

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins []string
	Mode           string // "strict", "development", "disabled"
}

// ParseOrganizationAllowlistCSV parses a CSV list of HCP Terraform organization names.
func ParseOrganizationAllowlistCSV(allowlistCSV string) ([]string, error) {
	var allowlist []string
	for _, organizationName := range strings.Split(allowlistCSV, ",") {
		organizationName = strings.TrimSpace(organizationName)
		if organizationName != "" {
			allowlist = append(allowlist, strings.ToLower(organizationName))
		}
	}
	if len(allowlist) == 0 {
		return nil, ErrMalformedOrganizationAllowlist
	}
	return allowlist, nil
}

// LoadCORSConfigFromEnv loads CORS configuration from environment variables
func LoadCORSConfigFromEnv() CORSConfig {
	originsStr := os.Getenv("MCP_ALLOWED_ORIGINS")
	mode := os.Getenv("MCP_CORS_MODE")

	// Default to strict mode if not specified
	if mode == "" {
		mode = "strict"
	}

	var origins []string
	if originsStr != "" {
		origins = strings.Split(originsStr, ",")
		// Trim spaces
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
	}

	return CORSConfig{
		AllowedOrigins: origins,
		Mode:           mode,
	}
}

// isOriginAllowed checks if the given origin is allowed based on the configuration
func isOriginAllowed(origin string, allowedOrigins []string, mode string) bool {
	// If mode is disabled, allow all origins
	if mode == "disabled" {
		return true
	}

	// Check if origin is in the allowed list
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}

	// In development mode, also allow localhost origins
	if mode == "development" {
		if strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "https://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "https://127.0.0.1:") ||
			strings.HasPrefix(origin, "http://[::1]:") ||
			strings.HasPrefix(origin, "https://[::1]:") {
			return true
		}
	}

	return false
}

// securityHandler wraps the StreamableHTTP handler with origin validation
type securityHandler struct {
	handler        http.Handler
	allowedOrigins []string
	corsMode       string
	logger         *log.Logger
}

// ServeHTTP implements the http.Handler interface
func (h *securityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate Origin header
	origin := r.Header.Get("Origin")
	if origin != "" {
		if !isOriginAllowed(origin, h.allowedOrigins, h.corsMode) {
			h.logger.Warnf("Rejected request from unauthorized origin: %s (CORS mode: %s)", origin, h.corsMode)
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// Log allowed origins at debug level to avoid too much noise in production
		h.logger.Debugf("Allowed request from origin: %s", origin)

		// If we have a valid origin, add CORS headers
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id, Authorization")
	}

	// Handle OPTIONS requests for CORS preflight
	if r.Method == http.MethodOptions {
		h.logger.Debugf("Handling OPTIONS preflight request from origin: %s", origin)
		w.WriteHeader(http.StatusOK)
		return
	}

	// If origin is valid or not present, delegate to the wrapped handler
	h.handler.ServeHTTP(w, r)
}

// NewSecurityHandler creates a new security handler
func NewSecurityHandler(handler http.Handler, allowedOrigins []string, corsMode string, logger *log.Logger) http.Handler {
	return &securityHandler{
		handler:        handler,
		allowedOrigins: allowedOrigins,
		corsMode:       corsMode,
		logger:         logger,
	}
}

type organizationLister interface {
	List(ctx context.Context, options *tfe.OrganizationListOptions) (*tfe.OrganizationList, error)
}

// OrganizationAllowlistMiddleware rejects HTTP requests whose bearer token cannot access an allowlisted organization.
func OrganizationAllowlistMiddleware(allowlist []string, logger *log.Logger) func(http.Handler) http.Handler {
	allowedOrganizations := make(map[string]struct{}, len(allowlist))
	for _, organizationName := range allowlist {
		organizationName = strings.TrimSpace(strings.ToLower(organizationName))
		if organizationName != "" {
			allowedOrganizations[organizationName] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedOrganizations) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			if r.Header.Get(TerraformAddress) != "" || r.URL.Query().Get(TerraformAddress) != "" {
				if logger != nil {
					logger.Warn("Rejecting request: Terraform address cannot be specified via header or query parameter")
				}
				http.Error(w, "Cannot specify Terraform address via header or query parameter", http.StatusForbidden)
				return
			}

			token := strings.TrimSpace(getTokenFromAuthHeader(r))
			if token == "" {
				logger.Warn("Rejecting request: organization allowlist requires Authorization bearer token")
				http.Error(w, "Authorization bearer token is required", http.StatusUnauthorized)
				return
			}

			lister, err := organizationListerForRequest(r.Context(), token, logger)
			if err != nil {
				logger.WithError(err).Error("Failed to initialize organization allowlist client")
				http.Error(w, "Failed to validate organization allowlist", http.StatusBadGateway)
				return
			}

			allowed, err := tokenHasAllowedOrganization(r.Context(), lister, allowedOrganizations)
			if err != nil {
				if errors.Is(err, tfe.ErrUnauthorized) {
					logger.Warn("Rejecting request: Terraform token is unauthorized")
					http.Error(w, "Terraform token is unauthorized", http.StatusUnauthorized)
					return
				}
				logger.WithError(err).Error("Failed to validate organization membership for supplied authorization token")
				http.Error(w, "Failed to validate organization membership for supplied authorization token", http.StatusBadGateway)
				return
			}
			if !allowed {
				logger.Warn("Rejecting request: Supplied authorization token does not have access to any organizations allowed by this server")
				http.Error(w, "Supplied authorization token does not have access to any organizations allowed by this server", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func organizationListerForRequest(ctx context.Context, token string, logger *log.Logger) (organizationLister, error) {
	terraformAddress := utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
	clientIP, _ := ctx.Value(contextKey(ClientIPKey)).(string)
	tfeClient, err := NewTfeClientForToken(terraformAddress, parseTerraformSkipTLSVerify(ctx), token, clientIP, logger)
	if err != nil {
		return nil, err
	}
	return tfeClient.Organizations, nil
}

func tokenHasAllowedOrganization(ctx context.Context, lister organizationLister, allowedOrganizations map[string]struct{}) (bool, error) {
	pageNumber := 1
	for {
		orgs, err := lister.List(ctx, &tfe.OrganizationListOptions{
			ListOptions: tfe.ListOptions{
				PageNumber: pageNumber,
				PageSize:   100,
			},
		})
		if err != nil {
			return false, err
		}
		if orgs == nil {
			return false, nil
		}

		for _, org := range orgs.Items {
			if org == nil {
				continue
			}
			if _, ok := allowedOrganizations[strings.ToLower(org.Name)]; ok {
				return true, nil
			}
		}

		if orgs.Pagination == nil || orgs.NextPage == 0 {
			return false, nil
		}
		pageNumber = orgs.NextPage
	}
}

// getTokenFromAuthHeader extracts token from Authorization Bearer header
func getTokenFromAuthHeader(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	}
	return ""
}

// TerraformContextMiddleware adds Terraform-related header values to the request context
// This middleware extracts Terraform configuration from HTTP headers, query parameters,
// or environment variables and adds them to the request context for use by MCP tools
func TerraformContextMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	clientIPCfg := LoadClientIPConfigFromEnv()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// TFE_ADDRESS is never sourced from the client in streamable-http mode.
			// Allowing a client to set the address via header or query parameter would
			// let it redirect requests (and the Authorization token) to an arbitrary
			// server. We Reject those attempts and source the address via server-side only.
			if r.Header.Get(TerraformAddress) != "" || r.URL.Query().Get(TerraformAddress) != "" {
				logger.Warn("Rejecting request: Terraform address cannot be specified via header or query parameter")
				http.Error(w, "Cannot specify Terraform address via header or query parameter", http.StatusForbidden)
				return
			}
			terraformAddress := utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
			ctx = context.WithValue(ctx, contextKey(TerraformAddress), terraformAddress)
			if logger != nil {
				logger.Debug("Terraform address configured server-side")
			}

			// The remaining vals may still be sourced from the client.
			clientHeaders := []string{TerraformToken, TerraformSkipTLSVerify}
			for _, header := range clientHeaders {
				var headerValue string

				// The allowlist validates the Authorization bearer token, so it must
				// also be the token used for downstream Terraform API requests.
				if header == TerraformToken {
					headerValue = getTokenFromAuthHeader(r)
				}

				if headerValue == "" {
					headerValue = r.Header.Get(header)
				}

				if headerValue == "" {
					headerValue = r.URL.Query().Get(header)

					if header == TerraformToken && headerValue != "" {
						logger.Info(fmt.Sprintf("Terraform token was provided in query parameters by client %v, terminating request", r.RemoteAddr))
						http.Error(w, "Terraform token should not be provided in query parameters for security reasons, use the Authorization header", http.StatusBadRequest)
						return
					}
				}

				if headerValue == "" {
					headerValue = utils.GetEnv(header, "")
				}

				// Add to context using the header name as key
				ctx = context.WithValue(ctx, contextKey(header), headerValue)

				// Log the source of the configuration (without exposing sensitive values)
				if header == TerraformToken && headerValue != "" {
					logger.Debug("Terraform token provided via request context")
				}
			}

			// Capture client IP for X-Forwarded-For header forwarding
			if utils.GetEnv(ForwardClientIP, "") == "true" {
				clientIP := getClientIP(r, clientIPCfg)
				if clientIP != "" {
					ctx = context.WithValue(ctx, contextKey(ClientIPKey), clientIP)
					logger.Debugf("Client IP captured for forwarding (method=%s): %s", clientIPCfg.Method, clientIP)
				}
			}
			// Call the next handler with the enriched context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIPConfig controls how the client IP is sourced for forwarding.
type ClientIPConfig struct {
	// The Method is one of the RemoteIPMethod consts. It defaults to RemoteAddr.
	Method string
	// TrustedHops is the number of proxy hops to trust from the right of the
	// X-Forwarded-For chain. Only used when the selected method is X-Forwarded-For.
	TrustedHops int
}

// The LoadClientIPConfigFromEnv reads the client-IP sourcing config from the env.
// The default is RemoteAddr, which trusts only the direct connection.
func LoadClientIPConfigFromEnv() ClientIPConfig {
	method := strings.TrimSpace(os.Getenv(RemoteIPMethodEnv))
	switch method {
	case RemoteIPMethodXRealIP, RemoteIPMethodXFF, RemoteIPMethodRemoteAddr:
	default:
		method = RemoteIPMethodRemoteAddr
	}

	hops := 0
	if raw := strings.TrimSpace(os.Getenv(XFFTrustedHopsEnv)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			hops = n
		}
	}

	return ClientIPConfig{Method: method, TrustedHops: hops}
}

// The getClientIP returns the client IP from the request based on the cfg. It returns a
// validated IP str, or "" if none could be determined.
//
// RemoteAddr (the default) trusts only the direct TCP peer. X-Real-IP and
// X-Forwarded-For are opt-in, since those headers are client-supplied and can be
// spoofed unless a trusted proxy sets them.
func getClientIP(r *http.Request, cfg ClientIPConfig) string {
	switch cfg.Method {
	case RemoteIPMethodXRealIP:
		if ip := parseIP(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
		return remoteAddrIP(r)

	case RemoteIPMethodXFF:
		if ip := ipFromXFF(r.Header.Get("X-Forwarded-For"), cfg.TrustedHops); ip != "" {
			return ip
		}
		return remoteAddrIP(r)

	default: // RemoteIPMethodRemoteAddr
		return remoteAddrIP(r)
	}
}

// remoteAddrIP returns the validated IP from r.RemoteAddr, it handles >  IPv4, IPv6,
// and addresses with or without a port.
func remoteAddrIP(r *http.Request) string {
	addr := r.RemoteAddr
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return parseIP(host)
	}
	return parseIP(addr)
}

// ipFromXFF returns the trusted client IP from an X-Forwarded-For chain.
//
// hops is the number of trusted proxies between the server and the client,
// counted from the right (the rightmost entry is set by the nearest proxy).
// It skips those hops and returns the next entry to the left, which is the
// first client-supplied address beyond the trusted proxies.
// .  example flow on how it works, and what the logic represents >
//
//	hops=1, "200.1.2.3, 10.1.1.10"                         -> "200.1.2.3"
//	hops=2, "108.0.0.1, 200.1.2.3, 10.1.1.10, 192.168.0.1" -> "200.1.2.3"
//
// Returns "" if the chain is empty, hops is <= 0, the position is out of range,
// or the entry is not a valid IP.
func ipFromXFF(xff string, hops int) string {
	if xff == "" || hops <= 0 {
		return ""
	}
	parts := strings.Split(xff, ",")
	// Skip hops trusted proxies from the right, then take the next entry left.
	// hops=1 -> parts[len-2], hops=2 -> parts[len-3].
	idx := len(parts) - hops - 1
	if idx < 0 || idx >= len(parts) {
		return ""
	}
	return parseIP(parts[idx])
}

func parseIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return ""
	}
	return ip.String()
}
