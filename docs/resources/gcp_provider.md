---
page_title: "yba_gcp_provider Resource - YugabyteDB Anywhere"
description: |-
  GCP Cloud Provider Resource. Use this resource to create and manage GCP cloud providers in YugabyteDB Anywhere.
---

# yba_gcp_provider (Resource)

GCP Cloud Provider Resource. Use this resource to create and manage GCP cloud providers in YugabyteDB Anywhere.

This resource provides a dedicated interface for GCP cloud providers with a simplified schema compared to the generic `yba_cloud_provider` resource.

## Credentials

GCP credentials can be provided in two ways:

1. **Service Account Credentials**: Set the `credentials` field with the JSON content of your service account key file
2. **Host Credentials**: Set `use_host_credentials = true` to use the credentials from the YugabyteDB Anywhere host (e.g., GCE metadata service)

-> **Note:** When using host credentials, ensure the YBA host has a service account with sufficient permissions for Compute Engine, VPC, and other required GCP services.

## VPC Configuration

GCP VPC can be configured in three ways:

1. **Existing VPC**: Set `network` to an existing VPC name (default behavior)
2. **Create New VPC**: Set `create_vpc = true` and `network` to the desired new VPC name
3. **Host VPC**: Set `use_host_vpc = true` to use the VPC from the YBA host

## Example Usage

### Basic GCP Provider with Credentials

```terraform
# Basic GCP Provider with credentials
resource "yba_gcp_provider" "example" {
  name        = "gcp-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"

  regions {
    code          = "us-west1"
    shared_subnet = "projects/my-project/regions/us-west1/subnetworks/my-subnet"
  }

  air_gap_install = false
}

# GCP Provider using host credentials (from the YBA host's GCP metadata)
resource "yba_gcp_provider" "host_credentials_example" {
  name                 = "gcp-host-creds-provider"
  use_host_credentials = true
  project_id           = "my-gcp-project"
  network              = "my-vpc-network"

  regions {
    code          = "us-central1"
    shared_subnet = "default"
  }

  air_gap_install = false
}

# GCP Provider with custom SSH key pair
resource "yba_gcp_provider" "ssh_example" {
  name        = "gcp-ssh-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"

  ssh_keypair_name        = "my-keypair"
  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  regions {
    code          = "us-west1"
    shared_subnet = "default"
  }

  air_gap_install = false
}

# GCP Provider with multiple regions
resource "yba_gcp_provider" "multi_region_example" {
  name        = "gcp-multi-region-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"

  regions {
    code          = "us-west1"
    shared_subnet = "projects/my-project/regions/us-west1/subnetworks/my-subnet"
  }

  regions {
    code          = "us-east1"
    shared_subnet = "projects/my-project/regions/us-east1/subnetworks/my-subnet"
  }

  regions {
    code          = "europe-west1"
    shared_subnet = "projects/my-project/regions/europe-west1/subnetworks/my-subnet"
  }

  air_gap_install = false
}

# GCP Provider with Shared VPC
resource "yba_gcp_provider" "shared_vpc_example" {
  name                  = "gcp-shared-vpc-provider"
  credentials           = file("~/.gcp/service-account.json")
  project_id            = "my-service-project"
  shared_vpc_project_id = "my-host-project"
  network               = "shared-vpc-network"

  regions {
    code          = "us-west1"
    shared_subnet = "projects/my-host-project/regions/us-west1/subnetworks/shared-subnet"
  }

  air_gap_install = false
}

# GCP Provider with firewall tags
resource "yba_gcp_provider" "firewall_example" {
  name             = "gcp-firewall-provider"
  credentials      = file("~/.gcp/service-account.json")
  project_id       = "my-gcp-project"
  network          = "my-vpc-network"
  yb_firewall_tags = "yb-db-node,allow-ssh"

  regions {
    code          = "us-west1"
    shared_subnet = "default"
  }

  air_gap_install = false
}

# GCP Provider with instance template
resource "yba_gcp_provider" "instance_template_example" {
  name        = "gcp-template-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"

  regions {
    code              = "us-west1"
    shared_subnet     = "default"
    instance_template = "projects/my-project/global/instanceTemplates/yb-node-template"
  }

  air_gap_install = false
}

# GCP Provider with custom image bundles
resource "yba_gcp_provider" "image_bundle_example" {
  name        = "gcp-image-bundle-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"

  regions {
    code          = "us-west1"
    shared_subnet = "default"
  }

  image_bundles {
    name           = "custom-x86-bundle"
    use_as_default = true
    details {
      arch     = "x86_64"
      ssh_user = "centos"
      ssh_port = 22
      region_overrides = {
        "us-west1" = "projects/my-project/global/images/my-custom-image"
      }
    }
  }

  air_gap_install = false
}

# GCP Provider with NTP configuration
resource "yba_gcp_provider" "ntp_example" {
  name        = "gcp-ntp-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"
  network     = "my-vpc-network"

  regions {
    code          = "us-west1"
    shared_subnet = "default"
  }

  # Use Google's NTP servers
  set_up_chrony = true

  air_gap_install = false
}

# GCP Provider creating a new VPC
resource "yba_gcp_provider" "create_vpc_example" {
  name        = "gcp-new-vpc-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"

  # Create a new VPC with the specified name
  create_vpc = true
  network    = "yba-created-vpc"

  regions {
    code          = "us-west1"
    shared_subnet = "default"
  }

  air_gap_install = false
}

# GCP Provider using host's VPC
resource "yba_gcp_provider" "host_vpc_example" {
  name        = "gcp-host-vpc-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "my-gcp-project"

  # Use the VPC from the YBA host
  use_host_vpc = true

  regions {
    code          = "us-west1"
    shared_subnet = "default"
  }

  air_gap_install = false
}
```

The details for configuration are available in the [YugabyteDB Anywhere Configure Cloud Provider Documentation](https://docs.yugabyte.com/preview/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/gcp/).

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Name of the provider.
- `regions` (Block List, Min: 1) GCP regions associated with the provider. (see [below for nested schema](#nestedblock--regions))

### Optional

- `air_gap_install` (Boolean) Flag indicating if YugabyteDB nodes are installed in an air-gapped environment, lacking access to the public internet for package downloads. Default is false.
- `create_vpc` (Boolean) Create a new VPC in GCP. If true, network must be specified as the new VPC name. Default is false.
- `credentials` (String, Sensitive) Google Service Account credentials JSON content. Stored in Terraform state - use an encrypted backend for security.
- `image_bundles` (Block List) Custom image bundles for the provider. At least one image_bundles or yba_managed_image_bundles block must be specified. (see [below for nested schema](#nestedblock--image_bundles))
- `network` (String) VPC network name in GCP.
- `ntp_servers` (List of String) List of NTP servers for time synchronization.
- `project_id` (String) GCP project ID that hosts universe nodes.
- `set_up_chrony` (Boolean) Set up NTP chrony service. When true with empty ntp_servers, uses cloud provider's NTP server (e.g., AWS Time Sync). When true with ntp_servers specified, uses custom NTP servers. When false, assumes NTP is pre-configured in the machine image. Default is false.
- `shared_vpc_project_id` (String) Shared VPC project ID. Use this to connect resources from multiple GCP projects to a common VPC.
- `ssh_keypair_name` (String) Custom SSH key pair name to access YugabyteDB nodes. Must be set together with ssh_private_key_content (self-managed mode). If both ssh_keypair_name and ssh_private_key_content are omitted, YugabyteDB Anywhere generates and manages the key pair (YBA-managed mode). YBA versions keys on every update: if a key with this name already exists it appends a timestamp (e.g. 'my-key-2026-03-18-10-01-29'). Use access_key_code to read the actual versioned name that was stored.
- `ssh_private_key_content` (String, Sensitive) SSH private key content to access YugabyteDB nodes. Must be set together with ssh_keypair_name (self-managed mode). If both fields are omitted, YugabyteDB Anywhere generates and manages the key pair (YBA-managed mode).
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `use_host_credentials` (Boolean) Use credentials from the YugabyteDB Anywhere host. Default is false.
- `use_host_vpc` (Boolean) Use VPC from the YugabyteDB Anywhere host. If false, network must be specified. Default is false.
- `yb_firewall_tags` (String) Tags for firewall rules in GCP.
- `yba_managed_image_bundles` (Block List, Max: 1) YBA managed image bundles for the provider. At least one image_bundles or yba_managed_image_bundles block must be specified. Only x86_64 architecture is supported. Omit this block to stop managing YBA default images via Terraform (any previously tracked bundles will be removed from the provider on the next apply). (see [below for nested schema](#nestedblock--yba_managed_image_bundles))

### Read-Only

- `access_key_code` (String) Access key code for this provider. Read-only, generated by YBA.
- `enable_node_agent` (Boolean) Flag indicating if node agent is enabled for this provider. Read-only.
- `host_vpc_id` (String) GCP Host VPC ID. Read-only, populated by YBA.
- `id` (String) The ID of this resource.
- `version` (Number) Provider version. Read-only, incremented on each update.
- `vpc_type` (String) VPC type: EXISTING or NEW. Read-only.

<a id="nestedblock--regions"></a>
### Nested Schema for `regions`

Required:

- `code` (String) GCP region code (e.g., us-west1, us-east1).

Optional:

- `instance_template` (String) Instance template for this region.
- `shared_subnet` (String) Shared subnet for all zones in this region. YBA will auto-discover zones and apply this subnet to each.

Read-Only:

- `name` (String) GCP region name. Read-only.
- `uuid` (String) Region UUID.
- `zones` (List of Object) Zones in this region. Auto-discovered by YBA based on the region. (see [below for nested schema](#nestedatt--regions--zones))

<a id="nestedatt--regions--zones"></a>
### Nested Schema for `regions.zones`

Read-Only:

- `code` (String)
- `name` (String)
- `subnet` (String)
- `uuid` (String)



<a id="nestedblock--image_bundles"></a>
### Nested Schema for `image_bundles`

Required:

- `details` (Block List, Min: 1, Max: 1) Image bundle details including SSH configuration. Architecture is always x86_64 for this cloud provider. (see [below for nested schema](#nestedblock--image_bundles--details))
- `name` (String) Name of the image bundle.

Optional:

- `use_as_default` (Boolean) Flag indicating if the image bundle should be used as default for this architecture. When no bundle for a given architecture has this set to true, YBA automatically promotes the first bundle as default. Terraform will suppress the resulting true->false drift in the plan.

Read-Only:

- `uuid` (String) Image bundle UUID.

<a id="nestedblock--image_bundles--details"></a>
### Nested Schema for `image_bundles.details`

Required:

- `ssh_user` (String) SSH user for the image.

Optional:

- `global_yb_image` (String) Global YB image for the bundle.
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

- `use_as_default` (Boolean) Flag indicating if the image bundle should be used as default. When no bundle for a given architecture has this set to true, YBA automatically promotes the first bundle as default. Terraform will suppress the resulting true->false drift in the plan.

Read-Only:

- `name` (String) Image bundle name assigned by YBA.
- `uuid` (String) Image bundle UUID.

## Import

GCP Providers can be imported using the provider UUID:

```sh
terraform import yba_gcp_provider.example <provider-uuid>
```

## Known Issues

### Zone ordering diff

YugabyteDB Anywhere may return availability zones in a different order from the one specified in your configuration. Because `zones` is a `TypeList` (order-sensitive), Terraform can produce a cosmetic plan diff that looks like zones are being swapped or removed even when no real change is intended. **This diff is cosmetic.** The underlying zone configuration is unchanged; only the positional order differs. Applying will not modify any infrastructure. To avoid seeing this diff repeatedly, define your `zones` blocks in the same order that YBA returns them — check the `terraform show` output after the first successful apply and reorder your config to match.

### Plan-time warnings for new image bundle attributes

When adding a new `image_bundles` block, Terraform may emit warnings of the form `was null, but now cty.StringVal(...)` for computed sub-fields such as `uuid`, `metadata_type`, and `ssh_port`. These are cosmetic warnings caused by a limitation in the legacy Terraform Plugin SDK's handling of nested computed attributes inside `TypeList` blocks. They do not affect apply behaviour and can be safely ignored.
