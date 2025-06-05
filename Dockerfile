# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# This Dockerfile contains multiple targets.
# Use 'docker build --target=<name> .' to build one.

# ===================================
#
#   Non-release images.
#
# ===================================

# devbuild compiles the binary
# -----------------------------------
FROM golang:1.24.2 AS devbuild
ARG VERSION="dev"
WORKDIR /build
RUN go env -w GOMODCACHE=/root/.cache/go-build
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build go mod download
COPY . ./
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -ldflags="-s -w -X terraform-mcp-server/version.GitCommit=$(shell git rev-parse HEAD) -X terraform-mcp-server/version.BuildDate=$(shell git show --no-show-signature -s --format=%cd --date=format:'%Y-%m-%dT%H:%M:%SZ' HEAD)" \
    -o terraform-mcp-server ./cmd/terraform-mcp-server

# dev runs the binary from devbuild using SCRATCH
# -----------------------------------
FROM scratch AS dev
ARG VERSION="dev"
WORKDIR /server
COPY --from=devbuild /build/terraform-mcp-server .
COPY --from=devbuild /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
CMD ["./terraform-mcp-server", "stdio"]

# ===================================
#
#   Release images that uses CI built binaries (CRT generated)
#
# ===================================

FROM scratch AS release-default
ARG BIN_NAME
# Export BIN_NAME for the CMD below, it can't see ARGs directly.
ENV BIN_NAME=$BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH
LABEL version=$PRODUCT_VERSION
LABEL revision=$PRODUCT_REVISION
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /bin/terraform-mcp-server
COPY --from=devbuild /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
CMD ["/bin/terraform-mcp-server", "stdio"]

# ===================================
#
#   Set default target to 'dev'.
#
# ===================================
FROM dev
