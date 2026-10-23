# API-key authentication. An explicit false is valid next to the API-key fields.
# The private key is read from a file at apply time.
resource "yba_oci_provider" "example" {
  name           = "oci-provider"
  compartment_id = "ocid1.compartment.oc1..aaaa"
  default_region = "us-ashburn-1"

  use_instance_principal = false
  tenancy_id             = "ocid1.tenancy.oc1..aaaa"
  user_id                = "ocid1.user.oc1..aaaa"
  fingerprint            = "12:34:56:78:90:ab:cd:ef"
  private_key_file_path  = "~/.oci/oci_api_key.pem"

  hosted_zone_id = "ocid1.dns-zone.oc1..aaaa"

  air_gap_install = false
  set_up_chrony   = true
  ntp_servers     = ["169.254.169.123"]

  ssh_keypair_name        = "my-key"
  ssh_private_key_content = file("~/.ssh/my-key.pem")

  regions {
    code                      = "us-ashburn-1"
    vcn_id                    = "ocid1.vcn.oc1.iad.aaaa"
    instance_configuration_id = "ocid1.instanceconfiguration.oc1.iad.aaaa"
    zones {
      code   = "us-ashburn-1-AD-1"
      subnet = "ocid1.subnet.oc1.iad.aaaa"
    }
  }

  yba_managed_image_bundles {
    arch           = "x86_64"
    use_as_default = true
  }

  image_bundles {
    name           = "custom-alma9"
    use_as_default = false
    details {
      arch     = "x86_64"
      ssh_user = "opc"
      ssh_port = 22
      region_overrides = {
        "us-ashburn-1" = "ocid1.image.oc1.iad.aaaa"
      }
    }
  }
}

# Instance principal of the YugabyteDB Anywhere host.
# The API-key fields are omitted because use_instance_principal is true.
resource "yba_oci_provider" "instance_principal" {
  name                   = "oci-instance-principal"
  compartment_id         = "ocid1.compartment.oc1..aaaa"
  default_region         = "us-ashburn-1"
  use_instance_principal = true

  regions {
    code   = "us-ashburn-1"
    vcn_id = "ocid1.vcn.oc1.iad.aaaa"
    zones {
      code   = "us-ashburn-1-AD-1"
      subnet = "ocid1.subnet.oc1.iad.aaaa"
    }
  }

  yba_managed_image_bundles {
    arch = "x86_64"
  }
}

# API-key authentication with the PEM in configuration.
resource "yba_oci_provider" "private_key" {
  name           = "oci-provider-key"
  compartment_id = "ocid1.compartment.oc1..aaaa"
  default_region = "us-ashburn-1"

  use_instance_principal = false
  tenancy_id             = "ocid1.tenancy.oc1..aaaa"
  user_id                = "ocid1.user.oc1..aaaa"
  fingerprint            = "12:34:56:78:90:ab:cd:ef"
  private_key_content    = file("~/.oci/oci_api_key.pem")

  regions {
    code   = "us-ashburn-1"
    vcn_id = "ocid1.vcn.oc1.iad.aaaa"
    zones {
      code   = "us-ashburn-1-AD-1"
      subnet = "ocid1.subnet.oc1.iad.aaaa"
    }
  }

  yba_managed_image_bundles {
    arch           = "aarch64"
    use_as_default = false
  }
}
