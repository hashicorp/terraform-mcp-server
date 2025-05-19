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
}

binary {
  go_modules = true
  osv        = true
  go_stdlib  = true

  secrets {
    all = true
  }
}
