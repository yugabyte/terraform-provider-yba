---
subcategory: ""
page_title: "Managing a private YBA over an SSH tunnel"
description: |-
  Use the provider with a YugabyteDB Anywhere host whose API port you cannot reach directly, through an SSH local port forward
---

# Managing a private YBA over an SSH tunnel

Many YugabyteDB Anywhere (YBA) installations run in a private VPC or a closed
network. The machine that runs Terraform cannot reach the YBA API port there.
To call the YBA API, the provider needs only an HTTPS (or HTTP) address that it
can connect to. It does not open tunnels itself. If you can SSH to the YBA VM,
an SSH local port forward gives the provider that address: the forward opens a
port on the machine that runs Terraform, and the provider connects to that port
as if it were YBA.

You manage the tunnel yourself, outside Terraform. Start it before you run
`terraform plan`, `apply` or `destroy`, and keep it open until the run ends. The
provider configuration has no tunnel fields. Set `host` to the local address of
the tunnel.

This page uses two names for the two machines:

- **Terraform runner**: the machine that runs `terraform` (your workstation or
  a CI runner). You run the tunnel command here, and the forwarded ports open
  here.
- **YBA VM**: the private host that runs YBA, or will run it after
  `yba_installer` installs it. It is the SSH destination. Its `sshd` must allow
  TCP forwarding, which is the OpenSSH default.

## How the YBA API becomes reachable

The examples on this page forward two ports:

| Listens on (Terraform runner) | Forwards to (YBA VM) | Used by |
| --- | --- | --- |
| `127.0.0.1:9443` | `443`, the YBA API | The provider (`host`) |
| `127.0.0.1:2222` | `22`, the `sshd` of the VM | `yba_installer` only |

While the tunnel is open, the Terraform runner reaches the YBA API, which is
`https://<yba-vm>:443` in the private network, at `https://127.0.0.1:9443`.
You need the second forward only when your configuration has a `yba_installer`
resource, which installs and manages YBA on the VM over SSH. Leave it out when
you manage a YBA that Terraform did not install.

The local ports `9443` and `2222` are examples. You can use any free local
ports.

## Start the tunnel

On the **Terraform runner**, run:

```sh
ssh -N -L 9443:localhost:443 -L 2222:localhost:22 user@yba-vm
```

| Part | Side | Meaning |
| --- | --- | --- |
| `user@yba-vm` | YBA VM | The SSH login on the YBA VM. |
| `-L 9443:localhost:443` | both | Open port `9443` on the Terraform runner, and forward connections to port `443` on the YBA VM. |
| `-L 2222:localhost:22` | both | Open port `2222` on the Terraform runner, and forward connections to the `sshd` of the YBA VM on port `22`. Only `yba_installer` needs it. |
| `-N` | - | Do not run a remote command. Open the tunnel only. |

In each `-L local:host:port` forward, the first port opens on the Terraform
runner. The YBA VM resolves `host:port`, so `localhost:443` means port 443 on
the YBA VM, not on your machine.

When YBA is already installed, make sure that the API is reachable before you
run Terraform:

```sh
curl -k https://127.0.0.1:9443
```

Keep the `ssh` process running while Terraform works with YBA. If the tunnel
closes during an apply, the API calls in progress fail, as with any other
network failure.

~> **Note:** If the Terraform runner cannot SSH to the YBA VM directly, go
through a host that it can reach. Add `-J user@jump-host`, or SSH to that host
and replace `localhost` in the forwards with the private address of the YBA VM.
You still run the command on the Terraform runner, and the provider
configuration below does not change. Port forwarding tools from cloud vendors
(`gcloud compute ssh -- -L ...`, AWS Systems Manager port forwarding,
`az network bastion tunnel`) also work: the provider only needs a local TCP
port on the Terraform runner.

## Point the provider at the tunnel

Set the `host` argument of the provider to an IP address or domain name with a
port, and no scheme:

```terraform
provider "yba" {
  host      = "127.0.0.1:9443"
  api_token = var.yba_api_token
}
```

The tunnel listens on the Terraform runner, so `host` is always `127.0.0.1` (or
`localhost`) and the local port that you chose. The location of the YBA VM does
not change it.

## TLS over the tunnel

~> **Note:** The provider does not verify the TLS certificate of the YBA
server, with or without a tunnel. It checks neither the certificate chain nor
the host name. So a connection to `127.0.0.1:9443` succeeds although YBA
presents a certificate for a different host name. You do not need to change
anything for the tunnel. This also means that over a tunnel, SSH authenticates
the YBA VM through its host key. TLS does not.

## Install YBA through the same tunnel

`yba_installer` installs YBA over SSH. Set its SSH fields to the forwarded
`sshd` port on the Terraform runner, not to the real address of the VM:

```terraform
resource "yba_installer" "install" {
  provider    = yba.unauthenticated
  ssh_host_ip = "127.0.0.1"
  ssh_port    = 2222
  ssh_user    = "<ssh-user>"

  ssh_private_key = var.ssh_private_key
  yba_license     = var.yba_license
  yba_version     = "<yba-version-with-build-number>"
}
```

`ssh_port` is optional, and its default is `22`. Set it when, as here, `sshd`
is reachable through a forwarded port and not directly on port `22`.

~> **Note:** The YBA VM downloads the YBA Installer package itself. The install
commands run over the SSH session and download the package from
`downloads.yugabyte.com`, not from your workstation. The tunnel carries only the
SSH session, so the YBA VM needs its own outbound access to
`downloads.yugabyte.com`.

~> **Warning:** Keep the tunnel open when you destroy `yba_installer`. If
nothing answers SSH at `127.0.0.1:2222` for about 30 seconds, destroy treats the
host as gone. It removes `yba_installer` from state and leaves YBA installed on
the VM.

## Complete example

Start the tunnel on the Terraform runner. Then apply a configuration that
installs YBA over the forwarded `sshd`, creates the first customer through an
unauthenticated provider, and has an authenticated provider for other
resources.

```sh
ssh -N -L 9443:localhost:443 -L 2222:localhost:22 user@yba-vm
```

```terraform
terraform {
  required_providers {
    yba = {
      source  = "yugabyte/yba"
      version = "~> 1.1"
    }
  }
}

variable "ssh_private_key" {
  type      = string
  sensitive = true
}

variable "yba_license" {
  type      = string
  sensitive = true
}

variable "customer_password" {
  type      = string
  sensitive = true
}

provider "yba" {
  alias = "unauthenticated"
  host  = "127.0.0.1:9443"
}

resource "yba_installer" "install" {
  provider    = yba.unauthenticated
  ssh_host_ip = "127.0.0.1"
  ssh_port    = 2222
  ssh_user    = "<ssh-user>"

  ssh_private_key = var.ssh_private_key
  yba_license     = var.yba_license
  yba_version     = "<yba-version-with-build-number>"
}

resource "yba_customer_resource" "customer" {
  provider   = yba.unauthenticated
  depends_on = [yba_installer.install]
  code       = "dev"
  email      = "<email>"
  name       = "<customer-name>"
  password   = var.customer_password
}

provider "yba" {
  host      = "127.0.0.1:9443"
  api_token = yba_customer_resource.customer.api_token
}
```

After Terraform creates the customer, use the authenticated `yba` provider for
the other resources (cloud providers, universes, storage configurations and so
on), as for any other YBA installation. For more about this change of provider,
see
[Running Terraform on existing YugabyteDB Anywhere installations](running-terraform-on-existing-yba-installations).
