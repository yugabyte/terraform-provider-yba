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

## Prerequisites

This provider required some API changes that are only available in YugabyteDB versions `>=2.13.1`, `>=2.12.4`, or `>=2.8.5`. 
The automated tests in this repository are hardcoded to use `2.13.1.0-b69` (replicated release sequence `790`).

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

## Acceptance Testing

Self-hosted runners have been set up in AWS, Azure, and GCP.
There are also separate projects, each with their own service account credentials.
All of the credentials (for accessing the projects and runner instances) are in Keybase under `teams/yugabyte/terraform-acctest`.

* `acctest-gce.json` for accessing Google project `yugabyte-terraform-test` and the runner instance
* `aws_creds.csv` for accessing AWS account `yugabyte-terraform-test`
* `aws-acctest.pem` for accessing AWS runner instance
* `azure_creds.txt` for accessing Azure app deployment (resource group is `yugabyte-terraform-test`)
* `azure-acctest.pem` for accessing Azure runner instance
* `replicated.conf` is the replicated configuration used in the acceptance tests (the file is copied onto each runner instance)
* `acctest.rli` is the development replicated license used in acceptance tests (the file is copied onto each runner instance)
* `application_settings.conf` is the application settings used in acceptance tests (the file is copied onto each runner instance)