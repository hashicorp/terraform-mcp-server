### CLI-driven runs

1. Ensure you are properly authenticated into HCP Terraform by running terraform login on the command line or by using a [credentials block](https://www.terraform.io/docs/commands/cli-config.html#credentials).

2. Add a code block to your Terraform configuration files to set up the cloud integration. You can add this configuration block to any .tf file in the directory where you run Terraform.

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


3. Run terraform init to initialize the workspace.
4. Run terraform apply to start the first run for this workspace.

For more details, see the [CLI workflow guide](https://developer.hashicorp.com/terraform/cloud-docs/run/cli).
