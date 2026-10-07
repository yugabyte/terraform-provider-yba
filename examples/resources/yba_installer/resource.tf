provider "yba" {
  alias = "unauthenticated"
  host  = "<host-ip-address>"
}

# Pass values that are already strings in Terraform directly to the
# resource. No local files are necessary.
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

# Alternatively, point each input at a local file.
resource "yba_installer" "install_from_files" {
  provider                  = yba.unauthenticated
  ssh_host_ip               = "<ip-of-yba-node-for-ssh-commands>"
  ssh_user                  = "<ssh-user>"
  ssh_private_key_file_path = "<ssh-private-key-filepath>"
  yba_license_file          = "<path-to-yba-license.lic-file>"
  application_settings_file = "<path-to-application_settings.conf-file>"
  yba_version               = "<YugabyteDB Anywhere-version-with-build-number>"
}

# ssh_host_ip and ssh_port give the address where the provider opens SSH
# connections. Set ssh_port when sshd listens on a port other than 22: a
# custom sshd port, a NAT or firewall port mapping, or the local end of an
# SSH tunnel. For example, with `ssh -L 2222:<yba-node>:22 <jump-host>`, set
# ssh_host_ip = "127.0.0.1" and ssh_port = 2222.
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
