---
page_title: "yba_installer Resource - YugabyteDB Anywhere"
description: |-
  Manages the installation of YugabyteDB Anywhere on an existing virtual machine using YBA Installer.
---

# yba_installer (Resource)

Manages the installation of YugabyteDB Anywhere on an existing virtual machine using YBA Installer.

~> **Note:** Destroy runs `yba-ctl clean` on the host. This removes the YugabyteDB Anywhere software and keeps the data directory, `/opt/yugabyte/data`. To delete the data, remove that directory on the host yourself.

~> **Note:** When `/opt/yugabyte/data` already holds YugabyteDB Anywhere data (for example, on a persistent disk that outlives the VM), create installs the software without data and starts YBA on the existing data. So when you destroy and recreate this resource on the same host, YBA keeps its data. For a fresh installation, delete `/opt/yugabyte/data` on the host before the next apply.

~> **Warning:** If nothing accepts an SSH connection at `ssh_host_ip` and `ssh_port` for about 30 seconds, destroy treats the host as gone. It removes the resource from state and does not clean up the host. Make sure that the host (and any SSH tunnel to it) is reachable before you destroy this resource.

~> **Security Note:** The values of `ssh_private_key`, `yba_license`, `application_settings`, `tls_certificate` and `tls_key` are stored in the Terraform state file (marked as sensitive). Use an encrypted backend and restrict access to your state files.

-> **Note:** The YugabyteDB Anywhere host needs **curl** and outbound HTTPS access to `downloads.yugabyte.com`. During *terraform apply*, the host downloads the YBA Installer package from there.

## YugabyteDB Anywhere version

Set `yba_version` to the full YugabyteDB Anywhere version with its build number, as it appears in the name of the YBA Installer package. For example, for the package `yba_installer_full-2025.2.7.0-b107-linux-x86_64.tar.gz`, set `yba_version = "2025.2.7.0-b107"`. The [YugabyteDB Anywhere release notes](https://docs.yugabyte.com/stable/releases/yba-releases/) give the build number of each release.

## Example Usage

```terraform
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
```

### Inline content or file paths

You can give each input as a string (inline content) or as the path to a local file. For each input, set one form, not both. The inline fields are marked sensitive.

| Inline content         | File path                   |
| ---------------------- | --------------------------- |
| `ssh_private_key`      | `ssh_private_key_file_path` |
| `application_settings` | `application_settings_file` |
| `yba_license`          | `yba_license_file`          |
| `tls_certificate`      | `tls_certificate_file`      |
| `tls_key`              | `tls_key_file`              |

Use the inline form when the value is already in Terraform, for example the output of a `tls_private_key` resource or a variable. You then do not need a local file for it. Terraform stores the inline values in the state file.

## Upgrade YugabyteDB Anywhere

To upgrade YugabyteDB Anywhere, set `yba_version` to the new version and run *terraform apply*. If the upgrade fails, the provider keeps the earlier `yba_version` in state, so the next *terraform apply* runs the upgrade again.

## Change the settings, certificates or license

A change to `application_settings`, `tls_certificate` or `tls_key` runs `yba-ctl reconfigure` on the next apply. A change to the path in `application_settings_file`, `tls_certificate_file` or `tls_key_file` also runs it. The provider does not track the content of these files. After you edit a file in place, change its path, or change `reconfigure` from `false` to `true`. A reconfiguration requires `application_settings` or `application_settings_file`.

A change to `yba_license`, or to the path in `yba_license_file`, adds the new license on the next apply.

-> **Note:** The provider copies the TLS certificate and key to **/tmp/server.crt** and **/tmp/server.key** on the YugabyteDB Anywhere host. Before installation, set `server_cert_path` and `server_key_path` to these paths in the application settings.

For host requirements and settings, refer to [Install YBA software using YBA Installer](https://docs.yugabyte.com/stable/yugabyte-platform/install-yugabyte-platform/install-software/installer/).

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `ssh_host_ip` (String) IP address of the host for SSH and SCP connections. With a local SSH tunnel to the host, use `127.0.0.1`.
- `ssh_user` (String) User with sudo access to use for ssh commands.
- `yba_version` (String) Version of YugabyteDB Anywhere to install, with its build number, for example `2025.2.7.0-b107`. A change to this field on an existing installation runs `yba-ctl upgrade` to the new version. YBA Installer does not support downgrades, so the plan fails when the new version is lower.

### Optional

- `application_settings` (String, Sensitive) Contents of the YBA Installer settings file (`yba-ctl.yml`) that configures YugabyteDB Anywhere. If you set neither this field nor `application_settings_file`, YBA Installer uses its default settings. Conflicts with `application_settings_file`.
- `application_settings_file` (String) Path to a local YBA Installer settings file (`yba-ctl.yml`) that configures YugabyteDB Anywhere. If you set neither this field nor `application_settings`, YBA Installer uses its default settings. Conflicts with `application_settings`.
- `host_architecture` (String) Architecture of the host Virtual Machine. Default is x86_64.
- `host_os` (String) Operating System of the host Virtual Machine. Default is linux.
- `reconfigure` (Boolean) Change this field to `true` to run `yba-ctl reconfigure` on the next apply when no other input changed. While it stays `true`, every update of this resource also runs a reconfiguration. Requires `application_settings` or `application_settings_file`. A change to `application_settings`, `tls_certificate` or `tls_key`, or to the path in their `_file` fields, starts a reconfiguration without this field.
- `skip_preflight_checks` (List of String) Check names to be skipped during preflight check.
- `ssh_port` (Number) TCP port for SSH and SCP connections to the host. Default is 22. Set this field when sshd listens on a different port at `ssh_host_ip`: for example, a custom sshd port, a NAT or firewall port mapping, or the local end of an SSH tunnel.
- `ssh_private_key` (String, Sensitive) Contents of the private key for SSH connections. Use this field instead of `ssh_private_key_file_path` to pass the key without a local file. Set exactly one of `ssh_private_key_file_path` or `ssh_private_key`.
- `ssh_private_key_file_path` (String) Path to a local file that contains the private key for SSH connections. Set exactly one of `ssh_private_key_file_path` or `ssh_private_key`.
- `timeouts` (Block, Optional) (see [below for nested schema](#nestedblock--timeouts))
- `tls_certificate` (String, Sensitive) Contents of the TLS certificate for HTTPS. The provider copies it to `/tmp/server.crt` on the host, so set `server_cert_path` to that path in the application settings. Conflicts with `tls_certificate_file`.
- `tls_certificate_file` (String) Path to a local TLS certificate file for HTTPS. The provider copies it to `/tmp/server.crt` on the host, so set `server_cert_path` to that path in the application settings. Conflicts with `tls_certificate`.
- `tls_key` (String, Sensitive) Contents of the TLS key for HTTPS. The provider copies it to `/tmp/server.key` on the host, so set `server_key_path` to that path in the application settings. Conflicts with `tls_key_file`.
- `tls_key_file` (String) Path to a local TLS key file for HTTPS. The provider copies it to `/tmp/server.key` on the host, so set `server_key_path` to that path in the application settings. Conflicts with `tls_key`.
- `yba_license` (String, Sensitive) Contents of the YugabyteDB Anywhere license. Use this field instead of `yba_license_file` to pass the license without a local file. Set exactly one of `yba_license_file` or `yba_license`.
- `yba_license_file` (String) Path to a local YugabyteDB Anywhere license file. Set exactly one of `yba_license_file` or `yba_license`.

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedblock--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String)
- `delete` (String)
- `update` (String)
