terraform {
  required_version = ">= 1.4.0"
}

# This state-only resource has no external provider dependencies.
resource "terraform_data" "run_test" {
  input = "terraform-mcp-server integration test for run tools"
}

output "run_test_id" {
  value = terraform_data.run_test.id
}
