resource "yba_release" "ybdb_release" {
  version            = "2024.2.3.0-b1"
  release_type       = "LTS"
  release_tag        = "example"
  release_notes      = "Example YBDB release managed by Terraform."
  release_date_msecs = 1740000000000
  state              = "ACTIVE"

  # x86_64 tarball uploaded to YugabyteDB Anywhere from the machine
  # running Terraform.
  artifact {
    platform     = "LINUX"
    architecture = "x86_64"
    local_file   = "/opt/releases/yugabyte-2024.2.3.0-b1-linux-x86_64.tar.gz"
  }

  # aarch64 tarball uploaded to YugabyteDB Anywhere from the machine
  # running Terraform.
  artifact {
    platform     = "LINUX"
    architecture = "aarch64"
    local_file   = "/opt/releases/yugabyte-2024.2.3.0-b1-el8-aarch64.tar.gz"
  }

  # Kubernetes helm chart downloaded by YugabyteDB Anywhere from a URL.
  # KUBERNETES artifacts do not set an architecture.
  artifact {
    platform    = "KUBERNETES"
    package_url = "https://downloads.yugabyte.com/releases/2024.2.3.0/yugabyte-2024.2.3.0-b1-helm.tar.gz"
  }
}
