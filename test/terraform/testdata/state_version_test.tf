terraform {
  required_version = ">= 1.4.0"
}

resource "terraform_data" "resource_one" {
  input = "terraform-mcp-server state version integration test - resource one"
}

resource "terraform_data" "resource_two" {
  input = "terraform-mcp-server state version integration test - resource two"
}

output "resource_one_id" {
  value = terraform_data.resource_one.id
}

output "resource_two_id" {
  value = terraform_data.resource_two.id
}