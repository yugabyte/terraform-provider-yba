---
page_title: "yba_azure_provider Resource - YugabyteDB Anywhere"
description: |-
  Azure Cloud Provider Resource. Use this resource to create and manage Azure cloud providers in YugabyteDB Anywhere.
---

# yba_azure_provider (Resource)

Azure Cloud Provider Resource. Use this resource to create and manage Azure cloud providers in YugabyteDB Anywhere.

This resource provides a dedicated interface for Azure cloud providers with a simplified schema compared to the generic `yba_cloud_provider` resource.

## Credentials

Azure credentials are provided via a service principal:

- `client_id` — Application (client) ID of the service principal
- `client_secret` — Client secret value
- `tenant_id` — Directory (tenant) ID
- `subscription_id` — Azure subscription ID
- `resource_group` — Resource group for provider resources

-> **Note:** The service principal must have Contributor access to the subscription or resource group where YugabyteDB nodes will be deployed.

## Example Usage

### Basic Azure Provider

```terraform
# Basic Azure Provider
resource "yba_azure_provider" "example" {
  name            = "azure-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
    zones {
      code   = "eastus-2"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}

# Azure Provider with custom SSH key pair
resource "yba_azure_provider" "ssh_example" {
  name            = "azure-ssh-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  ssh_keypair_name        = "my-keypair"
  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  regions {
    code = "westus2"
    vnet = "<vnet-name>"

    zones {
      code   = "westus2-1"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}

# Azure Provider with multiple regions
resource "yba_azure_provider" "multi_region_example" {
  name            = "azure-multi-region-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code              = "eastus"
    vnet              = "<eastus-vnet>"
    security_group_id = "<eastus-nsg-id>"

    zones {
      code   = "eastus-1"
      subnet = "<eastus-subnet-1>"
    }
    zones {
      code   = "eastus-2"
      subnet = "<eastus-subnet-2>"
    }
  }

  regions {
    code              = "westus2"
    vnet              = "<westus2-vnet>"
    security_group_id = "<westus2-nsg-id>"

    zones {
      code   = "westus2-1"
      subnet = "<westus2-subnet-1>"
    }
  }

  air_gap_install = false
}

# Azure Provider with separate network subscription
resource "yba_azure_provider" "network_sub_example" {
  name            = "azure-network-sub-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  network_subscription_id = "<azure-network-subscription-id>"
  network_resource_group  = "<azure-network-resource-group>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}

# Azure Provider with custom image bundle
resource "yba_azure_provider" "image_bundle_example" {
  name            = "azure-image-bundle-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }

  image_bundles {
    name           = "custom-x86-bundle"
    use_as_default = true
    details {
      arch     = "x86_64"
      ssh_user = "azureuser"
      ssh_port = 22
      region_overrides = {
        "eastus" = "/subscriptions/<sub-id>/resourceGroups/<rg>/providers/Microsoft.Compute/images/<image-name>"
      }
    }
  }

  air_gap_install = false
}

# Azure Provider with Private DNS Zone
resource "yba_azure_provider" "dns_example" {
  name            = "azure-dns-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  hosted_zone_id = "<private-dns-zone-name>"

  regions {
    code = "eastus"
    vnet = "<vnet-name>"

    zones {
      code   = "eastus-1"
      subnet = "<subnet-name>"
    }
  }

  air_gap_install = false
}
```

The details for configuration are available in the [YugabyteDB Anywhere Configure Cloud Provider Documentation](https://docs.yugabyte.com/preview/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/azure/).

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Name of the provider.
- `regions` (Block List, Min: 1) Azure regions associated with the provider. (see [below for nested schema](#nestedblock--regions))

### Optional

- `air_gap_install` (Boolean) Flag indicating if YugabyteDB nodes are installed in an air-gapped environment, lacking access to the public internet for package downloads. Default is false.
- `client_id` (String) Azure Client ID for service principal authentication.
- `client_secret` (String, Sensitive) Azure Client Secret. Required with client_id. Stored in Terraform state - use an encrypted backend for security.
- `hosted_zone_id` (String) Private DNS Zone for Azure.
- `image_bundles` (Block List) Custom image bundles for the provider. At least one image_bundles or yba_managed_image_bundles block must be specified. (see [below for nested schema](#nestedblock--image_bundles))
- `network_resource_group` (String) Azure Network Resource Group. All network resources and NIC resources of VMs will be created in this group.
- `network_subscription_id` (String) Azure Network Subscription ID. All network resources and NIC resources of VMs will be created in this subscription.
- `ntp_servers` (List of String) List of NTP servers for time synchronization.
- `resource_group` (String) Azure Resource Group. Required with client_id.
- `set_up_chrony` (Boolean) Set up NTP chrony service. When true with empty ntp_servers, uses cloud provider's NTP server (e.g., AWS Time Sync). When true with ntp_servers specified, uses custom NTP servers. When false, assumes NTP is pre-configured in the machine image. Default is false.
- `ssh_keypair_name` (String) Custom SSH key pair name to access YugabyteDB nodes. Must be set together with ssh_private_key_content (self-managed mode). If both ssh_keypair_name and ssh_private_key_content are omitted, YugabyteDB Anywhere generates and manages the key pair (YBA-managed mode). YBA versions keys on every update: if a key with this name already exists it appends a timestamp (e.g. 'my-key-2026-03-18-10-01-29'). Use access_key_code to read the actual versioned name that was stored.
- `ssh_private_key_content` (String, Sensitive) SSH private key content to access YugabyteDB nodes. Must be set together with ssh_keypair_name (self-managed mode). If both fields are omitted, YugabyteDB Anywhere generates and manages the key pair (YBA-managed mode).
- `subscription_id` (String) Azure Subscription ID. Required with client_id.
- `tenant_id` (String) Azure Tenant ID. Required with client_id.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `yba_managed_image_bundles` (Block List, Max: 1) YBA managed image bundles for the provider. At least one image_bundles or yba_managed_image_bundles block must be specified. Only x86_64 architecture is supported. Omit this block to stop managing YBA default images via Terraform (any previously tracked bundles will be removed from the provider on the next apply). (see [below for nested schema](#nestedblock--yba_managed_image_bundles))

### Read-Only

- `access_key_code` (String) Access key code for this provider. Read-only, generated by YBA.
- `enable_node_agent` (Boolean) Flag indicating if node agent is enabled for this provider. Read-only.
- `id` (String) The ID of this resource.
- `version` (Number) Provider version. Read-only, incremented on each update.
- `vpc_type` (String) VPC type: EXISTING or NEW. Read-only.

<a id="nestedblock--regions"></a>
### Nested Schema for `regions`

Required:

- `code` (String) Azure region code (e.g., westus2).
- `zones` (Block List, Min: 1) Availability zones in this region. (see [below for nested schema](#nestedblock--regions--zones))

Optional:

- `network_resource_group` (String) Network resource group for this region.
- `resource_group` (String) Resource group for this region.
- `security_group_id` (String) Network security group ID for this region.
- `vnet` (String) Virtual network name for this region.

Read-Only:

- `name` (String) Azure region name.
- `uuid` (String) Region UUID.

<a id="nestedblock--regions--zones"></a>
### Nested Schema for `regions.zones`

Required:

- `code` (String) Azure availability zone code.
- `subnet` (String) Subnet for this zone.

Optional:

- `secondary_subnet` (String) Secondary subnet for this zone.

Read-Only:

- `name` (String) Azure availability zone name.
- `uuid` (String) Zone UUID.



<a id="nestedblock--image_bundles"></a>
### Nested Schema for `image_bundles`

Required:

- `details` (Block List, Min: 1, Max: 1) Image bundle details including architecture and SSH configuration. (see [below for nested schema](#nestedblock--image_bundles--details))
- `name` (String) Name of the image bundle.

Optional:

- `use_as_default` (Boolean) Flag indicating if the image bundle should be used as default for this architecture.

Read-Only:

- `uuid` (String) Image bundle UUID.

<a id="nestedblock--image_bundles--details"></a>
### Nested Schema for `image_bundles.details`

Required:

- `arch` (String) Image bundle architecture. Allowed values: x86_64, aarch64.
- `ssh_user` (String) SSH user for the image.

Optional:

- `global_yb_image` (String) Global YB image for the bundle.
- `region_overrides` (Map of String) Region overrides for the bundle. Provide region code as the key and override image as the value.
- `ssh_port` (Number) SSH port for the image. Default is 22.



<a id="nestedblock--timeouts"></a>
### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `update` (String)


<a id="nestedblock--yba_managed_image_bundles"></a>
### Nested Schema for `yba_managed_image_bundles`

Required:

- `arch` (String) Image bundle architecture. Only x86_64 is supported for this cloud provider.

Optional:

- `use_as_default` (Boolean) Flag indicating if the image bundle should be used as default.

Read-Only:

- `name` (String) Image bundle name assigned by YBA.
- `uuid` (String) Image bundle UUID.

## Import

Azure Providers can be imported using the provider UUID:

```sh
terraform import yba_azure_provider.example <provider-uuid>
```

## Known Issues

### Zone ordering diff

YugabyteDB Anywhere may return availability zones in a different order from the one specified in your configuration. Because `zones` is a `TypeList` (order-sensitive), Terraform can produce a cosmetic plan diff that looks like zones are being swapped or removed even when no real change is intended. **This diff is cosmetic.** The underlying zone configuration is unchanged; only the positional order differs. Applying will not modify any infrastructure. To avoid seeing this diff repeatedly, define your `zones` blocks in the same order that YBA returns them — check the `terraform show` output after the first successful apply and reorder your config to match.

### Plan-time warnings for new image bundle attributes

When adding a new `image_bundles` block, Terraform may emit warnings of the form `was null, but now cty.StringVal(...)` for computed sub-fields such as `uuid`, `metadata_type`, and `ssh_port`. These are cosmetic warnings caused by a limitation in the legacy Terraform Plugin SDK's handling of nested computed attributes inside `TypeList` blocks. They do not affect apply behaviour and can be safely ignored.
