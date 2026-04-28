---
subcategory: ""
page_title: "YugabyteDB Anywhere Cloud Providers and Universes workflow in Terraform"
description: |-
  Managing cloud providers and universes via YugabyteDB Anywhere Terraform provider
---

# Use Terraform to create universes on AWS, GCP, or Azure

Using the YugabyteDB Anywhere Terraform provider, you can configure
[cloud providers](https://docs.yugabyte.com/stable/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/)
and [universes](https://docs.yugabyte.com/stable/yugabyte-platform/create-deployments/)
using dedicated provider resources and the `yba_universe` resource.

Use the dedicated provider resources for new deployments:

| Cloud | Resource |
|---|---|
| AWS | `yba_aws_provider` |
| GCP | `yba_gcp_provider` |
| Azure | `yba_azure_provider` |

The generic `yba_cloud_provider` resource is deprecated. `v2.0.0` will remove it, but it remains available throughout the v1.x line for existing configurations.

## AWS Provider and Universe

The following example configures an AWS cloud provider and creates a three-node universe with YSQL and TLS enabled.

```terraform
provider "yba" {
  host      = "<host-ip-address>"
  api_token = "<customer-api-token>"
}

resource "yba_aws_provider" "aws" {
  name              = "aws-provider"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"

  regions {
    code              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vpc_id            = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      subnet = "<subnet-id-a>"
    }
    zones {
      code   = "us-west-2b"
      subnet = "<subnet-id-b>"
    }
    zones {
      code   = "us-west-2c"
      subnet = "<subnet-id-c>"
    }
  }

  # Let YBA manage the default x86_64 image bundle
  yba_managed_image_bundles {
    arch           = "x86_64"
    use_as_default = true
  }

  air_gap_install = false
}

data "yba_provider_key" "aws_key" {
  provider_id = yba_aws_provider.aws.id
}

data "yba_release_version" "release" {
  track = "stable"
}

resource "yba_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "my-aws-universe"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.large"
      device_info {
        num_volumes  = 1
        volume_size  = 250
        disk_iops    = 3000
        throughput   = 125
        storage_type = "GP3"
      }
      access_key_code               = data.yba_provider_key.aws_key.id
      yb_software_version           = data.yba_release_version.release.id
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
    }
  }
  communication_ports {}
}
```

## GCP Provider and Universe

```terraform
resource "yba_gcp_provider" "gcp" {
  name        = "gcp-provider"
  credentials = file("~/.gcp/service-account.json")
  project_id  = "<gcp-project-id>"
  network     = "<gcp-vpc-network>"

  regions {
    code          = "us-west1"
    shared_subnet = "projects/<project>/regions/us-west1/subnetworks/<subnet>"
  }

  yba_managed_image_bundles {
    arch           = "x86_64"
    use_as_default = true
  }

  air_gap_install = false
}

data "yba_provider_key" "gcp_key" {
  provider_id = yba_gcp_provider.gcp.id
}

resource "yba_universe" "gcp_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "my-gcp-universe"
      provider           = yba_gcp_provider.gcp.id
      region_list        = yba_gcp_provider.gcp.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "n1-standard-4"
      device_info {
        num_volumes  = 1
        volume_size  = 375
        storage_type = "Persistent"
      }
      access_key_code               = data.yba_provider_key.gcp_key.id
      yb_software_version           = data.yba_release_version.release.id
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
    }
  }
  communication_ports {}
}
```

## Azure Provider and Universe

```terraform
resource "yba_azure_provider" "azure" {
  name            = "azure-provider"
  client_id       = "<azure-client-id>"
  client_secret   = "<azure-client-secret>"
  tenant_id       = "<azure-tenant-id>"
  subscription_id = "<azure-subscription-id>"
  resource_group  = "<azure-resource-group>"

  regions {
    code              = "westus2"
    vnet              = "<azure-vnet-name>"
    security_group_id = "<azure-nsg-id>"
    zones {
      code   = "westus2-1"
      subnet = "<subnet-name-1>"
    }
    zones {
      code   = "westus2-2"
      subnet = "<subnet-name-2>"
    }
    zones {
      code   = "westus2-3"
      subnet = "<subnet-name-3>"
    }
  }

  yba_managed_image_bundles {
    arch           = "x86_64"
    use_as_default = true
  }

  air_gap_install = false
}

data "yba_provider_key" "azure_key" {
  provider_id = yba_azure_provider.azure.id
}

resource "yba_universe" "azure_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "my-azure-universe"
      provider           = yba_azure_provider.azure.id
      region_list        = yba_azure_provider.azure.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "Standard_D4s_v3"
      device_info {
        num_volumes  = 1
        volume_size  = 250
        storage_type = "Premium_LRS"
      }
      access_key_code               = data.yba_provider_key.azure_key.id
      yb_software_version           = data.yba_release_version.release.id
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
    }
  }
  communication_ports {}
}
```

## Universe with Read Replicas

You can add a read replica (ASYNC) cluster to a universe. Define both clusters in the same
`yba_universe` resource.

```terraform
resource "yba_universe" "with_replica" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "my-universe"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.large"
      device_info {
        num_volumes  = 1
        volume_size  = 250
        storage_type = "GP3"
        disk_iops    = 3000
        throughput   = 125
      }
      access_key_code               = data.yba_provider_key.aws_key.id
      yb_software_version           = data.yba_release_version.release.id
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
    }
  }

  clusters {
    cluster_type = "ASYNC"
    user_intent {
      universe_name      = "my-universe"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.xlarge"
      device_info {
        num_volumes  = 1
        volume_size  = 250
        storage_type = "GP3"
        disk_iops    = 3000
        throughput   = 125
      }
      access_key_code               = data.yba_provider_key.aws_key.id
      yb_software_version           = data.yba_release_version.release.id
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
    }
  }

  communication_ports {}
}
```

To delete a read replica, remove the ASYNC cluster block and run `terraform apply`.

~> **Note:** Adding read replica clusters after universe creation is not currently supported.

## Universe with Custom Zone Placement

Use `cloud_list` to pin nodes to specific availability zones.

```terraform
resource "yba_universe" "pinned" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "pinned-universe"
      provider           = yba_aws_provider.aws.id
      region_list        = yba_aws_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.large"
      device_info {
        num_volumes  = 1
        volume_size  = 250
        storage_type = "GP3"
        disk_iops    = 3000
        throughput   = 125
      }
      access_key_code     = data.yba_provider_key.aws_key.id
      yb_software_version = data.yba_release_version.release.id
    }

    cloud_list {
      code = "aws"
      uuid = yba_aws_provider.aws.id
      region_list {
        code = "us-west-2"
        az_list {
          code      = "us-west-2a"
          num_nodes = 1
        }
        az_list {
          code      = "us-west-2b"
          num_nodes = 1
        }
        az_list {
          code      = "us-west-2c"
          num_nodes = 1
        }
      }
    }
  }
  communication_ports {}
}
```
