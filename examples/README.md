# Examples

This directory contains examples that are mostly used for documentation, but can also be run/tested manually via the Terraform CLI.

Each directory in `docker` contains a full example for creating a YugabyteDB Anywhere instance in that particular cloud, and then registering a customer, creating a cloud provider, and creating a universe.

The example configuration files can be found under `modules/resources`:
* `application_setting.conf` template for specifying YugabyteDB Anywhere settings
* `replicated.conf` template for specifying replicated settings to be used during the installation
  * additional configuration [options](https://help.replicated.com/docs/native/customer-installations/automating/)
* `replicated.conf` template for specifying replicated setting with HTTPS setup during the installation

The examples will install the YugabyteDB Anywhere version that is associated with the default release channel in the replicated license. For the internal development license, this is the `alpha` release channel.
There are additional settings (found in the link above) that can be specified in `replicated.conf` that set the [release channel](https://community.replicated.com/t/multi-channel-licenses/64) and the [release sequence](https://community.replicated.com/t/pinning-the-release-sequence-of-an-application/66#from-the-node-2).

Each directory in `kubernetes` contains a full example for creating a YugabyteDB Anywhere cluster in that particular cloud. The steps to then register a customer, create a universe etc. are the same as those in the docker examples.

If the kubernetes cluster already exists, an example for installing the helm chart can be found in `modules/kubernetes/<desired_cloud>/main.tf`.

Additional examples for creating cloud providers/universes in AWS/Azure can be found in the respective test files.