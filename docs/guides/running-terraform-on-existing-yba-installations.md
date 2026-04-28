---
subcategory: ""
page_title: "Running Terraform on existing YugabyteDB Anywhere installations"
description: |-
  Using existing YugabyteDB Anywhere installations in Terraform workflows
---

# Running Terraform on existing YugabyteDB Anywhere installations

You can configure the Terraform provider to run on an existing YugabyteDB Anywhere installation. To do so, define the provider with the YugabyteDB Anywhere host IP address and your [API token](https://api-docs.yugabyte.com/docs/yugabyte-platform/f10502c9c9623-yugabyte-db-anywhere-api-overview#api-tokens-and-uuids).

```terraform
provider "yba" {
  host      = "<yba-host-ip-address>"
  api_token = "<customer-api-token>"
}

```

After you define the preceding block, you can create resources via the Terraform provider on the YugabyteDB Anywhere installation running on the host machine.

If the installation does not yet have a customer, use an unauthenticated provider to create one, then switch to an authenticated provider to create and maintain the remaining resources.

```terraform
provider "yba" {
  alias = "unauthenticated"
  host = "<yba-host-ip-address>"
}

variable "customer_password" {
  type      = string
  sensitive = true
}

resource "yba_customer_resource" "customer" {
  provider = yba.unauthenticated
  code     = "admin"
  email    = "<email>"
  name     = "<customer-name>"
  password = var.customer_password
}

provider "yba" {
  host      = "<yba-host-ip-address>"
  api_token = yba_customer_resource.customer.api_token
}

```

You can import and manage other YugabyteDB Anywhere entities that the Terraform provider supports by following these steps:

1. Define an empty resource block in the configuration file for the resource you want to import. The following example shows an AWS cloud provider. Replace `yba_aws_provider` with `yba_gcp_provider`, `yba_azure_provider`, or `yba_onprem_provider` depending on the resource you are importing.

    ```terraform
    resource "yba_aws_provider" "aws_provider" {}

    ```

1. Import the resource using the *terraform import* command.

    ```sh
    $ terraform import yba_aws_provider.aws_provider <cloud-provider-uuid>
    yba_aws_provider.aws_provider: Importing from ID "00000000-0000-0000-0000-000000000000"...
    yba_aws_provider.aws_provider: Import prepared!
      Prepared yba_aws_provider for import
    yba_aws_provider.aws_provider: Refreshing state... [id=00000000-0000-0000-0000-000000000000]

    Import successful!

    The resources that were imported are shown above. These resources are now in your Terraform state and will henceforth be managed by Terraform.

    ```

1. To verify the import, run *terraform plan*. Using the plan output, update your configuration until the output shows **No changes**.

    ```sh
    $ terraform plan
    No changes. Your infrastructure matches the configuration.
    ```

    Once Terraform reports **No changes**, you can manage the resource safely for further operations.

## Sensitive fields are not imported

YugabyteDB Anywhere does not return secrets through its API, so any field marked `Sensitive` in the resource schema is absent from the state that `terraform import` produces. Running `terraform plan` after import will therefore show a diff on these fields. Reconcile the diff by either supplying the real value in your configuration or telling Terraform to ignore the field, depending on the resource:

- `yba_aws_provider`: `access_key_id`, `secret_access_key` -- supply in configuration.
- `yba_gcp_provider`: `credentials` -- supply in configuration.
- `yba_azure_provider`: `client_secret` -- supply in configuration.
- `yba_onprem_provider`: `ssh_private_key_content` -- supply in configuration.
- `yba_customer_resource`: `password` -- supply in configuration.
- `yba_universe`: `ysql_password`, `ycql_password` -- the API returns the literal string `REDACTED`. Use `lifecycle.ignore_changes` on these fields (passwords cannot be changed via Terraform after universe creation). See the [yba_universe Import section](../resources/universe#sensitive-fields-are-not-imported) for details.
- `yba_s3_storage_config`, `yba_gcs_storage_config`, `yba_azure_storage_config`: storage-config credential fields -- supply in configuration.

Refer to each resource's own Import section for resource-specific behavior.
