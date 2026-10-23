package mcpofficial

import (
	"os"
	"strconv"
	"strings"
)

// EnvEnabled selects the server implementation. The official go-sdk server is
// the default. Set it to a false value ("false", "0") to fall back to the
// legacy mark3labs server, for example to roll back after an upgrade.
const EnvEnabled = "TF_X_OFFICIAL_SDK_ENABLED"

// Enabled reports whether the official go-sdk server should be used. Unset,
// empty or unparseable values keep the default (enabled), so only an explicit
// false turns it off.
func Enabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvEnabled))
	if v == "" {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return enabled
}
