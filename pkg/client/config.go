package client

import log "github.com/sirupsen/logrus"

type config interface {
	configDir(*log.Logger) (string, error)
}

// NewConfig is a factory function that returns the implementation
// based on the OS. The compiler will choose which implementation
// is available at build time.
func newConfig() config {
	return &platformConfig{}
}
