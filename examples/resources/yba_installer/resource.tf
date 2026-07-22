provider "yba" {
  alias = "unauthenticated"
  host  = "<host-ip-address>"
}

# Pass values that are already strings inside Terraform directly to the
# resource - no temporary files are required on the local filesystem.
resource "yba_installer" "install" {
  provider    = yba.unauthenticated
  ssh_host_ip = "<ip-of-yba-node-for-ssh-commands>"
  ssh_port    = 22
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

# ssh_host_ip and ssh_port are simply the address the installer dials - any
# reachable sshd works. Set ssh_port when that address uses a non-default
# port: an sshd listening on 2222, a NAT/firewall mapping, or the local end
# of a tunnel (e.g. `ssh -L 2222:<yba-node>:22 <jump-host>` with
# ssh_host_ip = "127.0.0.1").
resource "yba_installer" "install_non_default_port" {
  provider    = yba.unauthenticated
  ssh_host_ip = "<address-where-sshd-is-reachable>"
  ssh_port    = 2222
  ssh_user    = "<ssh-user>"

  ssh_private_key      = var.ssh_private_key
  yba_license          = var.yba_license_content
  application_settings = local.yba_ctl_yaml

  yba_version = "<YugabyteDB Anywhere-version-with-build-number>"
}
