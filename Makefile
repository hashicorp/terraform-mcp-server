SHELL := /usr/bin/env bash -euo pipefail -c

BINARY_NAME ?= terraform-mcp-server
VERSION ?= $(if $(shell printenv VERSION),$(shell printenv VERSION),dev)

GO=go
DOCKER=docker

TARGET_DIR ?= $(CURDIR)/dist

# Build flags
LDFLAGS=-ldflags="-s -w -X terraform-mcp-server/version.GitCommit=$(shell git rev-parse HEAD) -X terraform-mcp-server/version.BuildDate=$(shell git show --no-show-signature -s --format=%cd --date=format:"%Y-%m-%dT%H:%M:%SZ" HEAD)"

.PHONY: all build crt-build test test-e2e test-security clean deps docker-build run-http run-http-secure docker-run-http test-http cleanup-test-containers help

# Default target
all: build

# Build the binary
# Get local ARCH; on Intel Mac, 'uname -m' returns x86_64 which we turn into amd64.
# Not using 'go env GOOS/GOARCH' here so 'make docker' will work without local Go install.
# Always use CGO_ENABLED=0 to ensure a statically linked binary is built
ARCH     = $(shell A=$$(uname -m); [ $$A = x86_64 ] && A=amd64; echo $$A)
OS       = $(shell uname | tr [[:upper:]] [[:lower:]])
build:
	CGO_ENABLED=0 GOARCH=$(ARCH) GOOS=$(OS) $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/terraform-mcp-server

crt-build:
	@mkdir -p $(TARGET_DIR)
	@$(CURDIR)/scripts/crt-build.sh build
	@cp $(CURDIR)/LICENSE $(TARGET_DIR)/LICENSE.txt

# Run tests
test:
	$(GO) test -v ./...

# Run e2e tests
test-e2e:
	@trap '$(MAKE) cleanup-test-containers' EXIT; $(GO) test -v --tags e2e ./e2e

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	$(GO) clean

# Download dependencies
deps:
	$(GO) mod download

# Build docker image
docker-build:
	$(DOCKER) build --build-arg VERSION=$(VERSION) -t $(BINARY_NAME):$(VERSION) .

# Run HTTP server locally
run-http:
	./$(BINARY_NAME) http --transport-port 8080 --transport-host 0.0.0.0

# Run HTTP server with security settings
run-http-secure:
	MCP_ALLOWED_ORIGINS="http://localhost:3000,https://example.com" MCP_CORS_MODE="development" ./$(BINARY_NAME) http --transport-port 8080 --transport-host 0.0.0.0

# Run HTTP server in Docker
docker-run-http:
	$(DOCKER) run -p 8080:8080 --rm $(BINARY_NAME):$(VERSION) http --transport-port 8080 --transport-host 0.0.0.0

# Test HTTP endpoint
test-http:
	@echo "Testing StreamableHTTP server health endpoint..."
	@curl -f http://localhost:8080/health || echo "Health check failed - make sure server is running with 'make run-http'"
	@echo "StreamableHTTP MCP endpoint available at: http://localhost:8080/mcp"

# Run security tests
test-security:
	$(GO) test ./cmd/terraform-mcp-server -v -run "TestIs|TestLoad|TestSecurity|TestOptions"

# Run docker container
# docker-run:
# 	$(DOCKER) run -it --rm $(BINARY_NAME):$(VERSION)

# Clean up test containers
cleanup-test-containers:
	@echo "Cleaning up test containers..."
	@$(DOCKER) ps -q --filter "ancestor=$(BINARY_NAME):test-e2e" | xargs -r $(DOCKER) stop
	@$(DOCKER) ps -aq --filter "ancestor=$(BINARY_NAME):test-e2e" | xargs -r $(DOCKER) rm
	@echo "Test container cleanup complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  all            - Build the binary (default)"
	@echo "  build          - Build the binary"
	@echo "  test           - Run all tests"
	@echo "  test-e2e       - Run end-to-end tests"
	@echo "  test-security  - Run security-related tests"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download dependencies"
	@echo "  docker-build   - Build docker image"
	@echo "  run-http       - Run StreamableHTTP server locally on port 8080"
	@echo "  run-http-secure - Run StreamableHTTP server with security settings"
	@echo "  docker-run-http - Run StreamableHTTP server in Docker on port 8080"
	@echo "  test-http      - Test StreamableHTTP health endpoint"
	@echo "  cleanup-test-containers - Stop and remove all test containers"
	@echo "  help           - Show this help message"

