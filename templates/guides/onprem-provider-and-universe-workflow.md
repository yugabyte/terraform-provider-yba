---
subcategory: ""
page_title: "YugabyteDB Anywhere On Premises Provider and Universes workflow in Terraform"
description: |-
  Managing onprem providers and universes via YugabyteDB Anywhere Terraform provider
---

# Use Terraform to create universes using on-premises provider

Using the YugabyteDB Anywhere Terraform provider, you can can configure [on-premises providers](https://docs.yugabyte.com/preview/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/on-premises/) and [universes](https://docs.yugabyte.com/preview/yugabyte-platform/create-deployments/) using a combination of resources and data sources.

~> **Note:** **Instance types** and **node instances** (marked as Optional parameters) can be added to the resource definition during provider configuration, or can be introduced after the provider has been created. These parameters are required for universe creation. For a complete list of Required and Optional parameters, see the *yba_onprem_provider* resource.

The following example workflow configures an on-premises provider and creates a universe with replication factor of 3, the YSQL API enabled, and TLS enabled using default certificates.

```terraform
provider "yba" {
  host      = "<host ip address>"
  api_token = "<customer-api-token>"
}

resource "yba_onprem_provider" "onprem" {

  name = "<onprem-provider-name>"

  details {
    passwordless_sudo_access = <boolean-for-sudo-password-requirement>
    skip_provisioning        = <boolean-for-manual-provisioning> # set true for manual provisioning
    ssh_user                 = "<ssh-user>"
  }

  access_keys {
    key_info {
      key_pair_name             = "<ssh-keypair-name>"
      ssh_private_key_file_path = "<file-path-to-private-key-on-terraform-system>"

    }
  }

  regions {
    name = "<region-name>"
    zones {
      name = "<zone1-name>"
    }
    zones {
      name = "<zone2-name>"
    }
    zones {
      name = "<zone3-name>"
    }
  }
  
  instance_types {
    instance_type_key {
      instance_type_code = "<name-of-instance-type>"
    }
    instance_type_details {
      volume_details_list {
        mount_path     = "<mount-path-separated-by-commas>"
        volume_size_gb = <volume-size-in-GB>
      }
    }
    mem_size_gb = <memory-size-in-GB>
    num_cores   = <number-of-cores-in-memory>
  }

  # Adding 3 nodes to be used in an RF-3 universe
  node_instances {
    instance_type = "<instance-type-defined-for-provider>"
    ip            = "<ip-of-node1>"
    region        = "<region-defined-for-provider>"
    zone          = "<zone-defined-for-region>"
  }
  node_instances {
    instance_type = "<instance-type-defined-for-provider>"
    ip            = "<ip-of-node2>"
    region        = "<region-defined-for-provider>"
    zone          = "<zone-defined-for-region>"
  }
  node_instances {
    instance_type = "<instance-type-defined-for-provider>"
    ip            = "<ip-of-node3>"
    region        = "<region-defined-for-provider>"
    zone          = "<zone-defined-for-region>"
  }
}

data "yba_provider_key" "onprem-key" {
  provider_id = yba_onprem_provider.onprem.id
}

locals {
  region_list  = yba_onprem_provider.onprem.regions[*].uuid
  provider_id  = yba_onprem_provider.onprem.id
  provider_key = data.yba_provider_key.onprem-key.id
}

data "yba_release_version" "release_version" {
  depends_on = [
    yba_onprem_provider.onprem
  ]
}

resource "yba_universe" "onprem_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "<universe-name>"
      provider_type      = "onprem"
      provider           = local.provider_id
      region_list        = local.region_list
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "<instance-type-defined-for-the-provider>"
      device_info {
        num_volumes   = <number-of-volumes-defined-in-the-instance-type>
        volume_size   = <volume-size-of-instances>
        storage_class = "standard"
        mount_points  = "<mount-points-for-nodes>"
      }
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = data.yba_release_version.release_version.id
      access_key_code               = local.provider_key
    }
  }
  communication_ports {}
}
```

## API errors encountered during universe creation

- 400 Bad Request: Couldn't find *number* nodes of type *instance_type*: Number of nodes of specified instance type is not available via the defined onprem provider.
- 500 Internal Server Error: No AZ found across regions: [*region uuid*]: Instance type may be unavailable for defined region and availability zones for the provider.
