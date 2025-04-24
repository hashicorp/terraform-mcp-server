package tfregistry

import (
	"net/http"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterResources(s *server.MCPServer, registryClient *http.Client, t translations.TranslationHelperFunc) {}
