// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package instructions

import _ "embed"

// Text is the server instructions shown to MCP clients. It is shared by both
// the mark3labs and go-sdk server implementations.
//
//go:embed instructions.md
var Text string
