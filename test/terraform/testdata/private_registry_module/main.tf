variable "integration_test_message" {
  description = "Message used to verify private registry module inputs"
  type        = string
  default     = "private-registry-test"
}

output "returned_test_message" {
  description = "Message returned by the private registry test module"
  value       = var.integration_test_message
}
