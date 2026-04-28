---
subcategory: ""
page_title: "YugabyteDB Anywhere On-Premises Provider and Universes workflow in Terraform"
description: |-
  Managing on-premises providers and universes via YugabyteDB Anywhere Terraform provider
---

# Use Terraform to create universes using on-premises provider

Using the YugabyteDB Anywhere Terraform provider, you can configure
[on-premises providers](https://docs.yugabyte.com/stable/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/on-premises/)
and [universes](https://docs.yugabyte.com/stable/yugabyte-platform/create-deployments/)
using the `yba_onprem_provider` resource.

-> **Note:** You can define instance types and node instances inline in the provider resource
(as shown below), or manage them separately using standalone `yba_onprem_node_instance` resources.
Do not use both approaches for the same provider, as this will cause conflicts.

-> **Note:** If the provider nodes have the YBA node agent installed, `access_key_code` is
not required in the universe configuration.

-> **Note:** To prepare nodes for on-premises providers, follow the instructions in
[Prepare nodes for use in a database cluster](https://docs.yugabyte.com/stable/yugabyte-platform/prepare/server-nodes-software/software-on-prem/#how-to-prepare-the-nodes-for-use-in-a-database-cluster)
to provision each node out of band, then declare the provider with `skip_provisioning = true`.

## Example: Pre-provisioned nodes (Automatic or Legacy Manual)

If you have provisioned your nodes out of band (either using the
[`node-agent-provision.sh` script](https://docs.yugabyte.com/stable/yugabyte-platform/prepare/server-nodes-software/software-on-prem/#how-to-prepare-the-nodes-for-use-in-a-database-cluster),
or by following the steps for legacy manual provisioning), create the on-premises provider
and universe as shown in the following example. Set `skip_provisioning = true`. Note that YBA
does not need to SSH into nodes when creating the provider.

```terraform
provider "yba" {
  host      = "<host-ip-address>"
  api_token = "<customer-api-token>"
}

resource "yba_onprem_provider" "onprem" {
  name             = "my-onprem-provider"
  ssh_user         = "yugabyte"
  ssh_keypair_name = "my-keypair"

  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  skip_provisioning        = true
  passwordless_sudo_access = true

  regions {
    code = "us-west"

    zones {
      code = "us-west-az1"
    }
    zones {
      code = "us-west-az2"
    }
    zones {
      code = "us-west-az3"
    }
  }

  instance_types {
    instance_type_code = "c5.large"
    num_cores          = 2
    mem_size_gb        = 4
    volume_size_gb     = 100
  }

  node_instances {
    ip            = "10.0.0.1"
    region_name   = "us-west"
    zone_name     = "us-west-az1"
    instance_type = "c5.large"
  }
  node_instances {
    ip            = "10.0.0.2"
    region_name   = "us-west"
    zone_name     = "us-west-az2"
    instance_type = "c5.large"
  }
  node_instances {
    ip            = "10.0.0.3"
    region_name   = "us-west"
    zone_name     = "us-west-az3"
    instance_type = "c5.large"
  }
}

data "yba_provider_key" "onprem_key" {
  provider_id = yba_onprem_provider.onprem.id
}

data "yba_release_version" "release" {
  track = "stable"
}

resource "yba_universe" "onprem_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "my-onprem-universe"
      provider           = yba_onprem_provider.onprem.id
      region_list        = yba_onprem_provider.onprem.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.large"
      device_info {
        num_volumes  = 1
        volume_size  = 100
        mount_points = "/mnt/d0"
      }
      access_key_code               = data.yba_provider_key.onprem_key.id
      yb_software_version           = data.yba_release_version.release.id
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
    }
  }
  communication_ports {}
}
```

## Common API Errors

- **400 Bad Request: Couldn't find N nodes of type `<instance_type>`** -- The number of
  node instances registered for the specified instance type is insufficient. Add more nodes
  to the provider via `node_instances` blocks or standalone `yba_onprem_node_instance` resources.

- **500 Internal Server Error: No AZ found across regions `[<region-uuid>]`** -- The instance
  type is not available in the configured region and availability zones. Verify that the
  `region_name` and `zone_name` in each `node_instances` block match the `code` values in
  your `regions` and `zones` blocks exactly.
