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
