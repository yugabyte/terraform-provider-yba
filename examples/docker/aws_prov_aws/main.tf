terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
    yb = {
      version = "~> 0.1.0"
      source  = "terraform.yugabyte.com/platform/yugabyte-platform"
    }
  }
}

locals {
  home                  = "/home/deeptikumar"
  dir                   = "${local.home}/code/terraform-provider-yugabytedb-anywhere/modules/resources"
  cluster_name          = "terraform-aws-dkumar"
  google_creds          = "${local.home}/.yugabyte/yugabyte-gce.json"
  aws_access_key_id     = "<placeholder for aws access key id>"
  aws_secret_access_key = "<placeholder for aws secret access key>"

}

provider "aws" {
  region = "us-west-2"
}

module "aws_yb_anywhere" {
  source = "../../../modules/docker/aws"

  cluster_name        = local.cluster_name
  ssh_user            = "ubuntu"
  ssh_keypair         = "yb-dev-aws-2"
  security_group_name = "${local.cluster_name}_sg"
  vpc_id              = "vpc-0fe36f6b"
  subnet_id           = "subnet-f840ce9c"
  tags                = {
        "Owner" = "<placeholder for user>",
        "Task" = "<placeholder for task>"
        "Department" = "<placeholder for department>"
      }
  // files
  ssh_private_key = "${local.home}/.yugabyte/yb-dev-aws-2.pem"
}

provider "yb" {
  alias = "unauthenticated"
  // these can be set as environment variables
  host = "${module.aws_yb_anywhere.public_ip}:80"
}

module "installation" {
  source = "../../../modules/installation"

  public_ip = module.aws_yb_anywhere.public_ip
  private_ip = module.aws_yb_anywhere.private_ip
  ssh_host_ip = module.aws_yb_anywhere.public_ip
  ssh_user = "ubuntu"
  ssh_private_key_file = "${local.home}/.yugabyte/yb-dev-aws-2.pem"
  replicated_directory = local.dir
  replicated_license_file_path = "${local.home}/.yugabyte/yugabyte-dev.rli"
}

resource "yb_customer_resource" "customer" {
  provider   = yb.unauthenticated
  depends_on = [module.installation]
  code       = "admin"
  email      = "demo@yugabyte.com"
  name       = "demo"
  password   = "Password1@"
}

provider "yb" {
  host      = "${module.aws_yb_anywhere.public_ip}:80"
  api_token = yb_customer_resource.customer.api_token
}

resource "yb_cloud_provider" "aws" {

  lifecycle {
    ignore_changes = all
  }
  code = "aws"
  config = {
    "AWS_ACCESS_KEY_ID" = local.aws_access_key_id,
    "AWS_SECRET_ACCESS_KEY" = local.aws_secret_access_key
  }
  
  name        = "${local.cluster_name}-provider"
  regions {
    code = "us-west-2"
    name = "us-west-2"
    security_group_id = "sg-139dde6c"
    vnet_name = "vpc-0fe36f6b"
    zones {
      code = "us-west-2a"
      name = "us-west-2a"
      subnet = "subnet-6553f513"
    }
    zones {
      code = "us-west-2b"
      name = "us-west-2b"
      subnet = "subnet-f840ce9c"
    }
    zones {
      code = "us-west-2c"
      name = "us-west-2c"
      subnet = "subnet-01ac5b59"
    }
  }
  
  ssh_port        = 22
  air_gap_install = false
}

data "yb_provider_key" "aws-key" {
  provider_id = yb_cloud_provider.aws.id
}

locals {
  region_list  = yb_cloud_provider.aws.regions[*].uuid
  provider_id  = yb_cloud_provider.aws.id
  provider_key = data.yb_provider_key.aws-key.id
}

resource "yb_releases" "new_s3" {
  lifecycle {
    ignore_changes = [
      s3,
    ]
  }
  version = "2.17.2.0-b77"
  s3 {
    access_key_id = local.aws_access_key_id
    secret_access_key = local.aws_secret_access_key
    paths {
      x86_64 = "s3://releases.yugabyte.com/2.17.2.0-b77/yugabyte-2.17.2.0-b77-centos-x86_64.tar.gz"
    }
  }
} 

resource "yb_releases" "new_http" {
  version = "2.17.1.0-jlipgcat"
  http {
    paths {
      x86_64 =         "https://s3.us-west-2.amazonaws.com/uploads.dev.yugabyte.com/jli/yugabyte-2.17.1.0-e7a8bf45b04326a3a4f8a600c0ce545f46ecc9d8-release-clang15-centos-x86_64.tar.gz"
      x86_64_checksum = "sha1:e16f4ca6c2e7bde8c3fe32721bd1eb815dcbd9f6"
    }
  }
} 
data "yb_release_version" "release_version"{
  depends_on = [
    yb_customer_resource.customer
  ]
}

resource "yb_universe" "aws_universe" {
  clusters {
    cluster_type = "PRIMARY"
    user_intent {
      universe_name      = "terraform-aws-universe--01"
      provider_type      = "aws"
      provider           = local.provider_id
      region_list        = local.region_list
      num_nodes          = 1
      replication_factor = 1
      instance_type      = "c5.large"
      device_info {
        num_volumes  = 1
        volume_size  = 250
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
      access_key_code               = local.provider_key
      instance_tags = {
        "yb_owner" = "dkumar",
        "yb_task" = "dev"
        "yb_dept" = "dev"
      }
      master_gflags = {
        "cdc_wal_retention_time_secs": "1",
        "log_min_segments_to_retain": "1",
        "log_cache_size_limit_mb": "0",
        "global_log_cache_size_limit_mb": "0",
        "log_stop_retaining_min_disk_mb": "9223372036854775807"
      }
      tserver_gflags = {
        "log_min_segments_to_retain": "1",
        "log_cache_size_limit_mb": "0",
        "global_log_cache_size_limit_mb": "0",
        "log_stop_retaining_min_disk_mb": "9223372036854775807"
      }
    }
  }
  communication_ports {}
}

resource "yb_storage_config_resource" "aws_storage" {

  name = "S3"
  backup_location = "s3://dkumar-test"
  config_name = "dkumar-terraform"
  s3_access_key_id = local.aws_access_key_id
  s3_secret_access_key = local.aws_secret_access_key

}

data "yb_storage_configs" "configs"{
  depends_on = [yb_customer_resource.customer]
}

resource "yb_storage_config_resource" "gcs_storage" {

  name = "GCS"
  backup_location = "gs://dkumar-test"
  config_name = "dkumar-terraform-gcs"
  gcs_creds_json = jsondecode(file(local.google_creds))


}
data "yb_storage_configs" "configs_gcs" {
    config_name = yb_storage_config_resource.gcs_storage.config_name
}

/*
resource "yb_backups" "aws_universe_backup_redis" {

  uni_uuid = yb_universe.aws_universe.id
  keyspace = "system_redis"
  storage_config_uuid = data.yb_storage_configs.configs_gcs.id
  time_before_delete = 864000
  sse = false
  transactional_backup = false
  frequency = 864000
  parallelism = 8
  backup_type ="REDIS_TABLE_TYPE"
  table_uuid_list = ["6ce286b2-7098-4dc7-b3e2-f3d8f21e25e7"]
}

resource "yb_backups" "aws_universe_backup_ycql" {

  uni_uuid = yb_universe.aws_universe.id
  keyspace = "ybdemo_keyspace"
  storage_config_uuid = data.yb_storage_configs.configs.id
  time_before_delete = 864000
  sse = false
  transactional_backup = false
  frequency = 864000
  parallelism = 8
  backup_type ="YQL_TABLE_TYPE"
}
/*
resource "yb_backups" "aws_universe_backup_ysql" {

  uni_uuid = yb_universe.aws_universe.id
  keyspace = "postgres"
  storage_config_uuid = data.yb_storage_configs.configs.id
   time_before_delete = 864000
  sse = false
  transactional_backup = false
  frequency = 864000
  parallelism = 8
  backup_type ="PGSQL_TABLE_TYPE"
}
*/