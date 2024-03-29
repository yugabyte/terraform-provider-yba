---
page_title: "yba_customer_resource Resource - YugabyteDB Anywhere"
description: |-
  Customer Resource.
---

# yba_customer_resource (Resource)

Customer Resource.

The following credential is required as environment variable before creation:

|Requirement|Environment Variable|
|-------|--------|
|[Customer Password](https://docs.yugabyte.com/preview/yugabyte-platform/configure-yugabyte-platform/create-admin-user/)|`YB_CUSTOMER_PASSWORD`|

## Example Usage

```terraform
provider "yba" {
  alias = "unauthenticated"
  host  = "<host-ip-address>"
}

resource "yba_customer_resource" "customer" {
  // use unauthenticcated provider to create customer
  provider = yba.unauthenticated
  code     = "<code>"
  email    = "<email-id>"
  name     = "<customer-name>"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `code` (String) Label for the user (i.e. admin).
- `email` (String) Email for the user, which is used for login on the YugabyteDB Anywhere portal.
- `name` (String) Name of the user.

### Optional

- `api_token` (String, Sensitive) API token for the customer.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))

### Read-Only

- `cuuid` (String) Customer UUID.
- `id` (String) The ID of this resource.

<a id="nestedblock--timeouts"></a>
### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)

## Import

Customer can be imported using `customer uuid`:

```sh
terraform import yba_customer_resource.customer <customer-uuid>
```
