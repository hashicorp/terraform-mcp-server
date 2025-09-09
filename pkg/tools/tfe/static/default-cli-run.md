### CLI-driven runs

1. Add a code block to your Terraform configuration files to set up the cloud integration. You can add this configuration block to any .tf file in the directory where you run Terraform.

Example code

```hcl
terraform { 
  cloud { 
    
    organization = "<<your-terraform-org>>" 

    workspaces { 
      name = "<<your-terraform-workspace>>" 
    } 
  } 
}
```


2. Run terraform init to initialize the workspace.
3. Run terraform apply to start the first run for this workspace.

For more details, see the [CLI workflow guide](https://developer.hashicorp.com/terraform/cloud-docs/run/cli).
