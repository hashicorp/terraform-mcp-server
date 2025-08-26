# Running tests

The HCP Terraform and TFE tools for the `terraform-mcp-server` relies on acceptance tests against their respective APIs. terraform-mcp-server is tested against HCP Terraform by our CI environment, and against Terraform Enterprise prior to release or otherwise as needed.

## 1. (Optional) Create repositories for policy sets and registry modules

If you are planning to run the full suite of tests or work on policy sets or registry modules, you'll need to set up repositories for them in GitHub.

Your policy set repository will need the following:
1. A policy set stored in a subdirectory `policy-sets/foo`
1. A branch other than `main` named `policies`

Alternatively, you can start with this [example repository for policy sets](https://github.com/hashicorp/test-policy-set) by forking the repository to your GitHub account, then setting `GITHUB_POLICY_SET_IDENTIFIER` to the forked repository identifier `your-github-handle/test-policy-set`.

Your registry module repository will need to be a [valid module](https://developer.hashicorp.com/terraform/cloud-docs/registry/publish-modules#preparing-a-module-repository).
It will need the following:
1. To be named `terraform-<PROVIDER>-<NAME>`
1. At least one valid SemVer tag in the format `x.y.z`
[terraform-random-module](https://github.com/caseylang/terraform-random-module) is a good example repo.

## 2. Set up environment variables (ENVVARS)

You'll need to have environment variables setup in your environment to run the tests. There are different options to facilitate setting up environment variables, using the tool [envchain](https://github.com/sorah/envchain) is one option:
   1. Install envchain - [refer to the envchain README for details](https://github.com/sorah/envchain#installation)
   2. Pick a namespace for storing your environment variables, such as: `terraform-mcp-server`. Then, for each environment variable you need to set, run the following command:
      ```sh
      envchain --set YOUR_NAMESPACE_HERE ENVIRONMENT_VARIABLE_HERE
      ```
      **OR**

      Set all of the environment variables at once with the following command:
      ```sh
      envchain --set YOUR_NAMESPACE_HERE TFE_ADDRESS TFE_TOKEN OAUTH_CLIENT_GITHUB_TOKEN GITHUB_POLICY_SET_IDENTIFIER
      ```

### Required ENVVARS

Tests are run against an actual backend so they require a valid backend address and token:

1. `TFE_ADDRESS` - URL of a HCP Terraform or Terraform Enterprise instance to be used for testing, including scheme. Example: `https://tfe.local`
1. `TFE_TOKEN` - A [user API token](https://developer.hashicorp.com/terraform/cloud-docs/users-teams-organizations/users#tokens) for the HCP Terraform or Terraform Enterprise instance being used for testing.

**Note:** Alternatively, you can set `TFE_HOSTNAME` which serves as a fallback for `TFE_ADDRESS`. It will only be used if `TFE_ADDRESS` is not set and will resolve the host to an `https` scheme. Example: `tfe.local` => resolves to `https://tfe.local`

## 3. Make sure run queue settings are correct

In order for the tests relating to queuing and capacity to pass, FRQ (fair run queuing) should be
enabled with a limit of 2 concurrent runs per organization on the HCP Terraform or Terraform Enterprise instance you are using for testing.

## 4. Run tests

For most situations, it's recommended to run specific tests because it takes about 20 minutes to run all of the tests.

### Running specific tests

Typically, you'll want to run specific tests. The commands below use notification configurations as an example.

#### With envchain:
```sh
$ envchain YOUR_NAMESPACE_HERE go test -run TestNotificationConfiguration -v ./...
```

#### Without envchain (Using TFE_ADDRESS):
```sh
$ TFE_TOKEN=xyz TFE_ADDRESS=https://tfe.local ENABLE_TFE=1 go test -run TestNotificationConfiguration -v ./...
```

#### Without envchain (Using TFE_HOSTNAME):
```sh
$ TFE_TOKEN=xyz TFE_HOSTNAME=tfe.local ENABLE_TFE=1 go test -run TestNotificationConfiguration -v ./...
```

#### Using Makefile target `test`
```sh
TFE_TOKEN=xyz TFE_ADDRESS=https://tfe.local TESTARGS="-run TestNotificationConfiguration" make test
```

### Running all tests
It takes about 20 minutes to run all of the tests, so specify a larger timeout when you run the tests (_the default timeout is 10 minutes_):

#### With envchain:
```sh
$ envchain YOUR_NAMESPACE_HERE go test ./... -timeout=30m
```

#### Without envchain  (Using TFE_ADDRESS):
```sh
$ TFE_TOKEN=xyz TFE_ADDRESS=https://tfe.local ENABLE_TFE=1 go test ./... -timeout=30m
```

#### Without envchain  (Using TFE_HOSTNAME):
```sh
$ TFE_TOKEN=xyz TFE_HOSTNAME=tfe.local ENABLE_TFE=1 go test ./... -timeout=30m
```


### Running tests for HCP Terraform features that require paid plans (HashiCorp Employees)

You can use the test helper `newSubscriptionUpdater()` to upgrade your test organization to a Business Plan, giving the organization access to all features in HCP Terraform. This method requires `TFE_TOKEN` to be a user token with administrator access in the target test environment. Furthermore, you **can not** have enterprise features enabled (`ENABLE_TFE=1`) in order to use this method since the API call fails against Terraform Enterprise test environments.
