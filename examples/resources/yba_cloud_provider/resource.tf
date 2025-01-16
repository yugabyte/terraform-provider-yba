resource "yba_cloud_provider" "cloud_provider" {
  code        = "<code>"
  dest_vpc_id = "<vpc-network>"
  name        = "<cloud-provider-name>"
  regions {
    code = "<region-code>"
    name = "<region-name>"
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "aws_cloud_provider" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    access_key_id     = "<s3-access-key-id>"
    secret_access_key = "<s3-secret-access-key>"
  }
  regions {
    code              = "us-west-2"
    name              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      name   = "us-west-2a"
      subnet = "<subnet-id>"
    }
    zones {
      code   = "us-west-2b"
      name   = "us-west-2b"
      subnet = "<subnet-id>"
    }
    zones {
      code   = "us-west-2c"
      name   = "us-west-2c"
      subnet = "<subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "aws_iam_cloud_provider" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    use_iam_instance_profile = true
  }
  regions {
    code              = "us-west-2"
    name              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      name   = "us-west-2a"
      subnet = "<subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "azure_cloud_provider" {
  code = "azu"
  name = "azure-provider"
  azure_config_settings {
    subscription_id = "<azure-subscription-id>"
    tenant_id       = "<azure-tenant-id>"
    client_id       = "<azure-client-id>"
    client_secret   = "<azure-client-secret>"
    resource_group  = "<azure-rg>"
  }
  regions {
    code      = "westus2"
    name      = "westus2"
    vnet_name = "<azure-vnet-id>"
    zones {
      code   = "westus2-1"
      name   = "westus2-1"
      subnet = "<azure-subnet-id>"
    }
    zones {
      code   = "westus2-2"
      name   = "westus2-2"
      subnet = "<azure-subnet-id>"
    }
    zones {
      code   = "westus2-3"
      name   = "westus2-3"
      subnet = "<azure-subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "gcp_cloud_provider" {
  code        = "gcp"
  dest_vpc_id = "<destination-vpc-id/network>"
  name        = "gcp-provider"
  gcp_config_settings {
    network      = "<gcp-network>"
    use_host_vpc = true
    project_id   = "<gcp-project-id>"
    application_credentials = {
      // GCP Service Account credentials JSON as map of strings
    }
  }
  regions {
    code = "us-west1"
    name = "us-west1"
    zones {
      subnet = "<gcp-shared-subnet-id>"
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "gcp_cloud_provider_with_image_bundles" {
  code = "gcp"
  name = "gcp-provider"
  gcp_config_settings {
    network      = "<gcp-network>"
    use_host_vpc = false
    project_id   = "<gcp-project-id>"
    application_credentials = {
      // GCP Service Account credentials JSON as map of strings
    }
  }
  regions {
    code = "us-west1"
    name = "us-west1"
    zones {
      subnet = "<gcp-shared-subnet-id>"
    }
  }
  image_bundles {
    name           = "<gcp-image-bundle-name-1>"
    use_as_default = false
    details {
      arch            = "x86_64"
      global_yb_image = "<ami-id>"
      ssh_user        = "centos"
      ssh_port        = 22
    }
  }
  image_bundles {
    name           = "<gcp-image-bundle-name-2>"
    use_as_default = true
    details {
      arch            = "x86_64"
      global_yb_image = "<ami-id>"
      ssh_user        = "centos"
      ssh_port        = 22
    }
  }
  air_gap_install = false
}

resource "yba_cloud_provider" "aws_cloud_provider_image_bundles" {
  code = "aws"
  name = "aws-provider"
  aws_config_settings {
    access_key_id     = "<s3-access-key-id>"
    secret_access_key = "<s3-secret-access-key>"
  }
  regions {
    code              = "us-west-2"
    name              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vnet_name         = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      name   = "us-west-2a"
      subnet = "<subnet-id>"
    }
    zones {
      code   = "us-west-2b"
      name   = "us-west-2b"
      subnet = "<subnet-id>"
    }
  }
  image_bundles {
    name           = "<image-bundle-name-1>"
    use_as_default = false
    details {
      arch = "x86_64"
      region_overrides = {
        "us-west-2" = "<ami-id>"
      }
      ssh_user = "ec2-user"
      ssh_port = 22
    }
  }
  image_bundles {
    name           = "<image-bundle-name-2>"
    use_as_default = true
    details {
      arch = "x86_64"
      region_overrides = {
        "us-west-2" = "<ami-id>"
      }
      ssh_user = "ec2-user"
      ssh_port = 22
    }
  }
  air_gap_install = false
}
