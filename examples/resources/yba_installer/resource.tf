provider "yba" {
  alias = "unauthenticated"
  host  = "<host ip address>"
}

resource "yba_installer" "install" {
  provider                  = yba.unauthenticated
  ssh_host_ip               = "<ip-of-yba-node-for-ssh-commands>"
  ssh_user                  = "<ssh-user>"
  ssh_private_key           = "<ssh-private-key-filepath>"
  yba_license_file          = "<path-to-yba-license.lic-file>"
  application_settings_file = "<path-to-application_settings.conf-file>"
  yba_version               = "<YugabyteDB Anywhere-version-with-build-number>"
}