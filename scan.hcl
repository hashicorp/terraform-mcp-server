# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Configuration for security scanner.
# Run on PRs and pushes to `main` and `release/**` branches.
# See .github/workflows/security-scan.yml for CI config.

# To run manually, install scanner and then run `scan repository .`

# Scan results are triaged via the GitHub Security tab for this repo.
# See `security-scanner` docs for more information on how to add `triage` config
# for specific results or to exclude paths.

# .release/security-scan.hcl controls scanner config for release artifacts, which
# unlike the scans configured here, will block releases in CRT.

repository {
  go_modules             = true
  osv                    = true
  go_stdlib_version_file = ".go-version"

  secrets {
    all = true
  }

  github_actions {
    pinned_hashes = true
    injection     = true
  }

  dependabot {
    required     = true
    check_config = true
  }

  dockerfile {
    pinned_hashes = true
    curl_bash     = true
  }

  github_branch_protections {
    branch "main" {
      include_administrators = true

      require_pr {
        required_approvals = 1
        dismiss_stale      = true
      }
    }
  }

  plugin "codeql" {
    languages = ["go"]
  }

  # Triage items that are _safe_ to ignore here. Note that this list should be
  # periodically cleaned up to remove items that are no longer found by the scanner.
  triage {
    suppress {
      vulnerabilities = []

      # Vulnerabilities that are false positives

      paths = []

      # Paths that are not relevant to the scan
    }
  }
}
