# Terraform Provider YugabyteDB Anywhere

This Terraform provider manages the following resources for YugabyteDB Anywhere:
* Cloud Providers
* Universes
* Backup Storage Configurations
* Backup Schedules
* Users
* Customers

In addition, there are modules included for installing and managing YugabyteDB Anywhere instances/clusters in the following clouds:
* AWS
* GCP
* Azure

## Installation

Install the [Terraform CLI](https://www.terraform.io/downloads). Once the CLI is installed, there are a few steps for [manually installing](https://www.terraform.io/cli/config/config-file#explicit-installation-method-configuration) this provider since it is not in the Terraform registry:
* Create the required folders in the [implied local mirror directories](https://www.terraform.io/cli/config/config-file#implied-local-mirror-directories)
  * `mkdir -p <implied_mirror_directory>/terraform.yugabyte.com/platform/yugabyte-platform/<provider_version>/<system_architecture>` 
  * `implied_mirror_directory` can be found from the link above
  * `provider_version` is the version of the provider you wish to use (only `0.1.0` for now)
  * `system_architecture` can be found by running `terraform -version`
  * An example for `x64 Linux` is `mkdir -p /usr/local/share/terraform/plugins/terraform.yugabyte.com/platform/yugabyte-platform/0.1.0/linux_amd64`
* Build the binary into the previously created folder
  * In the root directory, `go build -o <implied_mirror_directory>/terraform.yugabyte.com/platform/yugabyte-platform/<provider_version>/<system_architecture>`

## Examples

There are example configurations located within the `examples` directory for using the provider and modules. 
These bring up actual resources in the internal Yugabyte development environment.
More information can be found in the `README` located in the `examples` directory.
In the directory of the example you wish to run (i.e. `examples/docker/gcp`):
* `terraform init` installs the required providers
* `terraform apply` generates a plan, which, when approved, will create the desired resources
