# Terraform Provider YugabyteDB Anywhere

This Terraform provider manages YugabyteDB Anywhere (YBA) and the universes it runs:

* YBA installation, the first customer, and users
* Cloud providers: AWS, GCP, Azure, and on-premises
* YugabyteDB releases
* Universes, their load balancer configuration, and custom hooks
* Encryption in transit (certificates) and encryption at rest (key management service configurations)
* Backup storage configurations, backups, backup schedules, and restores
* Export of universe logs and metrics to telemetry providers such as Datadog, Splunk, Dynatrace, AWS CloudWatch, GCP Cloud Monitoring, Amazon S3, and OTLP endpoints
* Perf Advisor registration
* Runtime configuration keys

The [provider documentation](https://registry.terraform.io/providers/yugabyte/yba/latest/docs) on the Terraform Registry describes each resource and data source, with guides for common workflows.

In addition, there are modules included for installing and managing YugabyteDB Anywhere instances/clusters in the following clouds:

* AWS
* GCP
* Azure

## Prerequisites

This provider requires YugabyteDB Anywhere 2024.2.0.0 or later.
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

## Examples

The [`examples`](examples) directory has a configuration for every resource and data source. The [guides](docs/guides) walk through complete workflows, such as creating a cloud provider and a universe, scheduling backups, and restoring them.
