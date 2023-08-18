---
subcategory: ""
page_title: "Running Terraform on existing YugabyteDB Anywhere installations"
description: |-
  Using existing YugabyteDB Anywhere installations in Terraform workflows
---

# Running Terraform on existing YugabyteDB Anywhere installations

You can configure the Terraform provider to run on an existing YugabyteDB Anywhere installation. To do so, the provider needs to be defined with the YugabyteDB Anywhere host IP address and your [API token](https://api-docs.yugabyte.com/docs/yugabyte-platform/f10502c9c9623-yugabyte-db-anywhere-api-overview#api-tokens-and-uuids).

```terraform
provider "yba" {
  host      = "<yba-host-ip-address>"
  api_token = "<customer-api-token>"
}

```

Once the preceding block is defined, resources can be created via the Terraform provider on the YugabyteDB Anywhere installed on the host machine.

In the event that a customer has not been created in the installation, an unauthenticated provider can be used to create a customer, and a fresh provider with authentication can then be used to create and maintain the resources.

```terraform
provider "yba" {
  alias = "unauthenticated"
  host = "<yba-host-ip-address>"
}

resource "yba_customer_resource" "customer" {
  provider   = yba.unauthenticated
  code       = "admin"
  email      = "<email>"
  name       = "<customer-name>"
}

provider "yba" {
  host      = "<yba-host-ip-address>"
  api_token = yba_customer_resource.customer.api_token
}

```

Other YugabyteDB Anywhere entities that are supported by the Terraform provider can also be imported and managed using Terraform by performing the following steps:

1. Define an empty resource block in the configuration file of the resource to be imported. The following example shows the cloud provider resource.

    ```terraform
    resource "yba_cloud_provider" "cloud_provider" {}

    ```

1. Import the resource using the *terraform import* command.

    ```sh
    $ terraform import yba_cloud_provider.cloud_provider <cloud-provider-uuid>
    yba_cloud_provider.cloud_provider: Importing from ID "7fc1c313-5590-4599-88f4-109a15fe7db9"...
    yba_cloud_provider.cloud_provider: Import prepared!
      Prepared yba_cloud_provider for import
    yba_cloud_provider.cloud_provider: Refreshing state... [id=7fc1c313-5590-4599-88f4-109a15fe7db9]

    Import successful!

    The resources that were imported are shown above. These resources are now in your Terraform state and will henceforth be managed by Terraform.

    ```

1. To verify the import, run *terraform plan*. Using the plan output, update your configuration until the output shows **No changes**.

    ```sh
    $ terraform plan
    No changes. Your infrastructure matches the configuration.
    ```

    Once the preceding message is displayed, the resource can be safely managed for further operations.

