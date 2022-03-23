# Examples

This directory contains examples that are mostly used for documentation, but can also be run/tested manually via the Terraform CLI.

Each directory in `docker` contains a full example for creating a YugabyteDB Anywhere instance in that particular cloud, and then registering a customer, creating a cloud provider, and creating a universe.

The example configuration files can be found under `modules/resources`:
* `application_setting.conf` template for specifying YugabyteDB Anywhere settings
* `replicated.conf` template for specifying replicated settings to be used during the installation
  * additional configuration [options](https://help.replicated.com/docs/native/customer-installations/automating/)
* `replicated.conf` template for specifying replicated setting with HTTPS setup during the installation

Each directory in `kubernetes` contains a full example for creating a YugabyteDB Anywhere cluster in that particular cloud. The steps to then register a customer, create a universe etc. are the same as those in the docker examples.