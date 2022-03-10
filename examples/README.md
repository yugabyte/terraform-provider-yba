# Examples

This directory contains examples that are mostly used for documentation, but can also be run/tested manually via the Terraform CLI.

Each directory contains a full example for creating a Yugabyte Anywhere instance in that particular cloud, and then registering a customer, creating a cloud provider, and creating a universe.

The example configuration files can be found under `modules/resources`:
* `application_setting.conf` template for specifying Yugabyte Anywhere settings
* `replicated.conf` template for specifying replicated settings to be used during the installation
* `replicated.conf` template for specifying replicated setting with HTTPS setup during the installation