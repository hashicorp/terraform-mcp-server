terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

variable "child_message" {
  description = "Message used to verify private registry submodule inputs"
  type        = string
  default     = "private-registry-submodule-test"
}

resource "random_pet" "child" {
  prefix = var.child_message
}

output "child_pet_name" {
  description = "Name returned by the private registry test submodule"
  value       = random_pet.child.id
}
