provider "yba" {
  alias = "unauthenticated"
  host  = "<host ip address>"
}

# Pass values that are already strings inside Terraform directly to the
# resource - no temporary files are required on the local filesystem.
resource "yba_installer" "install" {
  provider    = yba.unauthenticated
  ssh_host_ip = "<ip-of-yba-node-for-ssh-commands>"
  ssh_user    = "<ssh-user>"

  ssh_private_key      = var.ssh_private_key
  yba_license          = var.yba_license_content
  application_settings = local.yba_ctl_yaml
  tls_certificate      = tls_self_signed_cert.yba.cert_pem
  tls_key              = tls_private_key.yba.private_key_pem

  yba_version = "<YugabyteDB Anywhere-version-with-build-number>"
}

# Alternatively, point each attribute at a local file. The file-based
# attributes remain supported for backward compatibility.
resource "yba_installer" "install_from_files" {
  provider                  = yba.unauthenticated
  ssh_host_ip               = "<ip-of-yba-node-for-ssh-commands>"
  ssh_user                  = "<ssh-user>"
  ssh_private_key_file_path = "<ssh-private-key-filepath>"
  yba_license_file          = "<path-to-yba-license.lic-file>"
  application_settings_file = "<path-to-application_settings.conf-file>"
  yba_version               = "<YugabyteDB Anywhere-version-with-build-number>"
}
