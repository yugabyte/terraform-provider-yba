---
subcategory: ""
page_title: "YugabyteDB Anywhere Cloud Providers and Universes workflow in Terraform"
description: |-
  Managing cloud providers and universes via YugabyteDB Anywhere Terraform provider
---

# Use Terraform to create universes on AWS, GCP, or Azure

Using the YugabyteDB Anywhere Terraform provider, you can can configure [cloud providers](https://docs.yugabyte.com/preview/yugabyte-platform/configure-yugabyte-platform/set-up-cloud-provider/aws/) and [universes](https://docs.yugabyte.com/preview/yugabyte-platform/create-deployments/) using a combination of resources and data sources.

The following example workflow configures an AWS cloud provider and a RF-3 universe with TLS enabled using default certificates.

```terraform
provider "yb" {
  host  = "<host ip address>:80"
  api_token = "<customer-api-token>"
}

resource "yb_cloud_provider" "aws" {
  code = "aws"
  name = "<aws-cloud-provider-name>"
  regions {
    code = "us-west-2"
    name = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code = "us-west-2a"
      name = "us-west-2a"
      subnet = "<subnet-id>"
    }
    zones {
      code = "us-west-2b"
      name = "us-west-2b"
      subnet = "<subnet-id>"
    }
    zones {
      code = "us-west-2c"
      name = "us-west-2c"
      subnet = "<subnet-id>"
    }
  }
  
  ssh_port        = 22
  air_gap_install = false
}

data "yb_provider_key" "aws-key" {
  provider_id = yb_cloud_provider.aws.id
}

data "yb_release_version" "release_version" {
  depends_on = [
    yb_cloud_provider.aws
  ]
}

resource "yb_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "<aws-universe-name>"
      provider_type      = "aws"
      provider           = yb_cloud_provider.aws.id
      region_list        = yb_cloud_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.large"
      device_info {
        num_volumes = 1
        volume_size = 250
        disk_iops = 3000
        throughput = 125
        storage_type = "GP3"
        storage_class = "standard"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = data.yb_release_version.release_version.id
      access_key_code               = data.yb_provider_key.aws-key.id
      instance_tags = {
        <labels-for-universe-nodes>
      }
      master_gflags = {
        <master-node-gflags>
      }
      tserver_gflags = {
        <tserver-node-gflags>
      }
    }
  }
  communication_ports {}
}
```

## Configuring Azure and GCP Cloud Providers

```terraform
resource "yb_cloud_provider" "gcp" {
  code = "gcp"
  dest_vpc_id = "<gcp-vpc-id>"
  name        = "<gcp-cloud-provider-name>"
  gcp_config_settings {
    network = "<gcp-network>"
    use_host_vpc = true
    project_id = "<gcp-project>"
  }
  regions {
    code = "us-west1"
    name = "us-west1"
    zones { 
      subnet = "<subnet-id>" 
    }
  }
  ssh_port        = 22
  air_gap_install = false
}

resource "yb_cloud_provider" "azure" {
  code = "azu"
  
  name        = "<azu-cloud-provider-name>"
  regions {
    code = "westus2"
    name = "westus2"
    vnet_name = "<azu-vnet-id>"
    zones {
      code = "westus2-1"
      name = "westus2-1"
      subnet = "<azu-subnet-id>"
    }
    zones {
      code = "westus2-2"
      name = "westus2-2"
      subnet = "<azu-subnet-id>"
    }
    zones {
      code = "westus2-3"
      name = "westus2-3"
      subnet = "<azu-subnet-id>"
    }
  }
}
```

## Configure universes with read replicas

The following universe definition can be used to create universes with read replicas.

```terraform
resource "yb_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "<universe-name>"
      provider_type      = "aws"
      provider           = yb_cloud_provider.aws.id
      region_list        = yb_cloud_provider.aws.regions[*].uuid
      num_nodes          = 1
      replication_factor = 1
      instance_type      = "c5.large"
      device_info {
        num_volumes = 1
        volume_size = 250
        disk_iops = 3000
        throughput = 125
        storage_type = "GP3"
        storage_class = "standard"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = data.yb_release_version.release_version.id
      access_key_code               = data.yb_provider_key.aws-key.id
      instance_tags = {
        <labels-for-universe-nodes>
      }
    }
  }
  clusters {
    cluster_type = "ASYNC"
    user_intent {
      universe_name      = "<universe-name-as-in-primary-cluster-definition>"
      provider_type      = "aws"
      provider           = yb_cloud_provider.aws.id
      region_list        = yb_cloud_provider.aws.regions[*].uuid
      num_nodes          = 3
      replication_factor = 3
      instance_type      = "c5.xlarge"
      device_info {
        num_volumes = 1
        volume_size = 250
        disk_iops = 3000
        throughput = 125
        storage_type = "GP3"
        storage_class = "standard"
      }
      assign_public_ip              = true
      use_time_sync                 = true
      enable_ysql                   = true
      enable_node_to_node_encrypt   = true
      enable_client_to_node_encrypt = true
      yb_software_version           = data.yb_release_version.release_version.id
      access_key_code               = data.yb_provider_key.aws-key.id
      instance_tags = {
        <labels-for-universe-nodes>
      }
    }
  }
  communication_ports {}
}
```

To delete read replicas, remove the read Replica cluster definition and run *terraform apply* to trigger the update universe workflow and remove the cluster.

~> **Note:** Adding read replicas after universe creation is not currently supported.