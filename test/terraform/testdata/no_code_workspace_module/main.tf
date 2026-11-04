# Copyright IBM Corp. 2025, 2026
# SPDX-License-Identifier: MPL-2.0

variable "name" {
  description = "Required name used by the no-code workspace integration test."
  type        = string
}

variable "description" {
  description = "Optional description used to verify that module defaults are honored."
  type        = string
  default     = ""
}

output "name" {
  description = "The supplied name."
  value       = var.name
}

output "description" {
  description = "The optional description."
  value       = var.description
}
