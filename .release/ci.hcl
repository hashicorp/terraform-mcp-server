# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

schema = "2"

project "terraform-mcp-server" {
  team = "team-proj-mcp-servers"

  slack {
    notification_channel = "C08QFU5CWG1"
  }

  github {
    organization     = "hashicorp"
    repository       = "terraform-mcp-server"
    release_branches = ["main", "release/**"]
  }
}

event "merge" {
}

event "build" {
  action "build" {
    organization = "hashicorp"
    repository   = "terraform-mcp-server"
    workflow     = "build"
    depends      = null
    config       = ""
  }

  depends = ["merge"]
}

event "prepare" {
  action "prepare" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
    config       = ""
  }

  depends = ["build"]

  notification {
    on = "fail"
  }
}

event "trigger-staging" {
}

event "promote-staging" {
  action "promote-staging" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-staging"
    depends      = null
    config       = "oss-release-metadata.hcl"
  }

  depends = ["trigger-staging"]

  notification {
    on = "always"
  }

  promotion-events {

    pre-promotion {
      organization = "hashicorp"
      repository   = "terraform-mcp-server"
      workflow     = "enos-run"
    }
  }
}

event "trigger-production" {
}

event "promote-production" {
  action "promote-production" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-production"
    depends      = null
    config       = ""
  }

  depends = ["trigger-production"]

  notification {
    on = "always"
  }

  promotion-events {
    bump-version-patch = true
    update-ironbank    = true
  }
}
