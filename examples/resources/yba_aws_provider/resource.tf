# Basic AWS Provider with access keys
resource "yba_aws_provider" "example" {
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
  }

  air_gap_install = false
}

# AWS Provider using IAM Instance Profile
resource "yba_aws_provider" "iam_example" {
  name                     = "aws-iam-provider"
  use_iam_instance_profile = true

  regions {
    code              = "us-east-1"
    security_group_id = "<aws-sg-id>"
    vpc_id            = "<aws-vpc-id>"
    zones {
      code   = "us-east-1a"
      subnet = "<subnet-id>"
    }
  }

  air_gap_install = false
}

# AWS Provider with custom SSH key pair
resource "yba_aws_provider" "ssh_example" {
  name              = "aws-ssh-provider"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"

  ssh_keypair_name        = "my-keypair"
  ssh_private_key_content = file("~/.ssh/my-keypair.pem")

  regions {
    code              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vpc_id            = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      subnet = "<subnet-id>"
    }
  }

  air_gap_install = false
}

# AWS Provider with custom image bundles and region overrides
resource "yba_aws_provider" "image_bundle_example" {
  name              = "aws-image-bundle-provider"
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
  }

  image_bundles {
    name           = "custom-x86-bundle"
    use_as_default = true
    details {
      arch     = "x86_64"
      ssh_user = "ec2-user"
      ssh_port = 22
      region_overrides = {
        "us-west-2" = "<ami-id-for-us-west-2>"
      }
    }
  }

  image_bundles {
    name           = "custom-arm-bundle"
    use_as_default = false
    details {
      arch     = "aarch64"
      ssh_user = "ec2-user"
      ssh_port = 22
      region_overrides = {
        "us-west-2" = "<arm-ami-id-for-us-west-2>"
      }
    }
  }

  air_gap_install = false
}

# AWS Provider with NTP configuration
resource "yba_aws_provider" "ntp_example" {
  name              = "aws-ntp-provider"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"

  regions {
    code              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vpc_id            = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      subnet = "<subnet-id>"
    }
  }

  # Use AWS Time Sync service
  set_up_chrony = true

  air_gap_install = false
}

# AWS Provider with Route53 hosted zone
resource "yba_aws_provider" "route53_example" {
  name              = "aws-route53-provider"
  access_key_id     = "<aws-access-key-id>"
  secret_access_key = "<aws-secret-access-key>"

  hosted_zone_id = "<route53-hosted-zone-id>"

  regions {
    code              = "us-west-2"
    security_group_id = "<aws-sg-id>"
    vpc_id            = "<aws-vpc-id>"
    zones {
      code   = "us-west-2a"
      subnet = "<subnet-id>"
    }
  }

  air_gap_install = false
}
