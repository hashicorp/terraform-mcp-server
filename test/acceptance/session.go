package acceptance

import "github.com/mark3labs/mcp-go/mcp"

type TestSession struct {
	id           string
	notifChannel chan mcp.JSONRPCNotification
	initialized  bool
}

func (s *TestSession) SessionID() string { return s.id }
func (s *TestSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return s.notifChannel
}
func (s *TestSession) Initialize()       { s.initialized = true }
func (s *TestSession) Initialized() bool { return s.initialized }
