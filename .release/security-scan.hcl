# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

container {
  dependencies    = true
  osv             = true
  alpine_security = true
  go_modules      = true
  local_daemon    = true

  secrets {
    all = true
  }

  triage {
    suppress {
      # GO-2026-6911 is fixed in mcp-go commit 68b6198264163e70269f65eea14518ef1a9c94d2,
      # but the Go vulnerability database still lists all versions as affected.
      # Remove once the advisory includes a fixed version.
      vulnerabilities = ["GO-2026-6911"]
    }
  }
}

binary {
  go_modules = true
  osv        = true
  go_stdlib  = true

  secrets {
    all = true
  }

  triage {
    suppress {
      # Match the container exception while the advisory lacks a fixed version.
      vulnerabilities = ["GO-2026-6911"]
    }
  }
}
