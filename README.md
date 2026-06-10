# Terraform Provider YugabyteDB Anywhere

This Terraform provider manages the following resources for YugabyteDB Anywhere:

* Cloud Providers - AWS, GCP, Azure
* On Prem Provider
* Universes
* Backup Storage Configurations
* Backup Schedules
* Restores
* Customers

In addition, there are modules included for installing and managing YugabyteDB Anywhere instances/clusters in the following clouds:

* AWS
* GCP
* Azure

## Prerequisites

This provider requires YugabyteDB Anywhere stable version `>=2024.2.0.0-b1` or preview version `>=2.23.1.0-b1`.
The acceptance tests run against a standing YugabyteDB Anywhere deployed with the YBA installer (`yba_installer`). See [`acctest/README.md`](acctest/README.md) for how to run them.

## Installation

Install the [Terraform CLI](https://www.terraform.io/downloads). Once the CLI is installed, there are a few steps to [manually install](https://www.terraform.io/cli/config/config-file#explicit-installation-method-configuration) and test the local provider:

* Run `make install` in the root directory of the project
* Add the following block to the configuration file to test the local provider:

```hcl
terraform {
  required_providers {
    yba = {
      version = "0.1.0-dev"
      source  = "yugabyte/yba"
    }
  }
}
```

## Resource files

Examples of configuration setting files can be found in the directory [resources](https://github.com/yugabyte/terraform-provider-yba/tree/main/acctest/resources)

* [YBA installer configuration file](https://github.com/yugabyte/terraform-provider-yba/tree/main/acctest/resources/yba-ctl.yml)

## Examples

There are example configurations located within the `examples` directory in [yba-terraform-workflow-example](https://github.com/yugabyte/yba-terraform-workflow-example.git) for using the provider and modules.
These bring up actual resources in the internal Yugabyte development environment.
More information can be found in the `README` located in the `examples` directory.
In the directory of the example you wish to run (i.e. `examples/docker/gcp`):

* `terraform init` installs the required providers
* `terraform apply` generates a plan, which, when approved, will create the desired resources
