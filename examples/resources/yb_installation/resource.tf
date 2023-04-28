provider "yb" {
  alias = "unauthenticated"
  host  = "<host ip address>:80"
}

resource "yb_installation" "installation" {
  provider                  = yb.unauthenticated
  public_ip                 = "<public-ip-of-yba-node>"
  private_ip                = "<private-ip-of-yba-node>"
  ssh_host_ip               = "<ip-of-yba-node-for-ssh>"
  ssh_user                  = "<ssh-user>"
  ssh_private_key           = file("<ssh-private-key-filepath>")
  replicated_config_file    = "<path-to-replicated.conf-file>"
  replicated_license_file   = "<path-to-replicated.rli-file>"
  application_settings_file = "<path-to-application_settings.conf-file>"
}