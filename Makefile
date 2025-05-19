SHELL := /usr/bin/env bash -euo pipefail -c

BINARY_NAME ?= terraform-mcp-server
VERSION ?= $(if $(shell printenv VERSION),$(shell printenv VERSION),dev)

GO=go
DOCKER=docker

TARGET_DIR ?= $(CURDIR)/dist

# Build flags
LDFLAGS=-ldflags="-s -w -X terraform-mcp-server/version.GitCommit=$(shell git rev-parse HEAD) -X terraform-mcp-server/version.BuildDate=$(shell git show --no-show-signature -s --format=%cd --date=format:"%Y-%m-%dT%H:%M:%SZ" HEAD)"

.PHONY: all build crt-build test test-e2e clean deps docker-build help

# Default target
all: build

# Build the binary
# Get local ARCH; on Intel Mac, 'uname -m' returns x86_64 which we turn into amd64.
# Not using 'go env GOOS/GOARCH' here so 'make docker' will work without local Go install.
# Always use CGO_ENABLED=0 to ensure a statically linked binary is built
ARCH     = $(shell A=$$(uname -m); [ $$A = x86_64 ] && A=amd64; echo $$A)
OS       = $(shell uname | tr [[:upper:]] [[:lower:]])
build:
	CGO_ENABLED=0 GOARCH=$(ARCH) GOOS=$(OS) $(GO) build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/terraform-mcp-server

crt-build:
	@mkdir -p $(TARGET_DIR)
	@$(CURDIR)/scripts/crt-build.sh build
	@cp $(CURDIR)/LICENSE $(TARGET_DIR)/LICENSE.txt

# Run tests
test:
	$(GO) test -v ./...

# Run e2e tests
test-e2e:
	$(GO) test -v --tags e2e ./e2e

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

# Run docker container
# docker-run:
# 	$(DOCKER) run -it --rm $(BINARY_NAME):$(VERSION)

# Show help
help:
	@echo "Available targets:"
	@echo "  all          - Build the binary (default)"
	@echo "  build        - Build the binary"
	@echo "  test         - Run all tests"
	@echo "  test-e2e     - Run end-to-end tests"
	@echo "  clean        - Remove build artifacts"
	@echo "  deps         - Download dependencies"
	@echo "  docker-build - Build docker image"
	@echo "  help         - Show this help message"
