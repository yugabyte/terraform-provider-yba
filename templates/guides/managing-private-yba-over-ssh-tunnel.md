---
subcategory: ""
page_title: "Managing a private YBA over an SSH tunnel"
description: |-
  Using the Terraform provider against a YugabyteDB Anywhere host with no reachable API endpoint, via a local port-forward
---

# Managing a private YBA over an SSH tunnel

Many YugabyteDB Anywhere (YBA) installations run inside a private VPC or a
locked-down network where the API port is not reachable from the machine
that runs Terraform. The provider itself only needs an HTTPS (or HTTP)
endpoint it can connect to - it has no notion of SSH or tunnels. If you can
SSH to the YBA VM, an SSH local port-forward is all it takes: the forward
opens a listener on the Terraform runner, and the provider talks to that
listener as if it were YBA.

The tunnel is entirely out-of-band. You start it yourself, outside of
Terraform, before running `terraform plan` / `apply` / `destroy`, and keep it
open for the duration of the run. The provider configuration has no
tunnel-specific fields - it just points `host` at the local address the
tunnel is listening on.

Two machines are involved throughout this page:

- **Terraform runner** - the machine that executes `terraform` (your
  workstation or a CI runner). The tunnel command runs *here*, and the
  forwarded ports open *here*.
- **YBA VM** - the private host running (or about to run) YBA. It is the
  SSH destination; nothing on it needs to change.

## How the YBA API becomes reachable

The examples on this page forward two ports:

| Listens on (Terraform runner) | Forwards to (YBA VM) | Used by |
| --- | --- | --- |
| `127.0.0.1:9443` | `443` - the YBA API | The provider (`host`) |
| `127.0.0.1:2222` | `22` - the VM's `sshd` | `yba_installer` only |

While the tunnel is up, the YBA API - normally `https://<yba-vm>:443` inside
the private network - is reachable from the Terraform runner as
`https://127.0.0.1:9443`. The second forward is only needed if you use
`yba_installer` to install or manage YBA on the node over SSH; skip it when
managing an already-installed YBA.

The local ports `9443` and `2222` are arbitrary conventions used throughout
this page - any free local ports work.

## Start the tunnel

On the **Terraform runner**, run:

```sh
ssh -N -L 9443:localhost:443 -L 2222:localhost:22 user@yba-vm
```

Breaking that down:

| Part | Side | Meaning |
| --- | --- | --- |
| `user@yba-vm` | YBA VM | The SSH login on the YBA VM. |
| `-L 9443:localhost:443` | both | Listen on port `9443` on the Terraform runner; forward connections to port `443` on the YBA VM. |
| `-L 2222:localhost:22` | both | Listen on port `2222` on the Terraform runner; forward to the YBA VM's own `sshd` on `22`. Only needed for `yba_installer`. |
| `-N` | - | Do not run a remote command - tunnel only. |

In each `-L local:host:port` forward, the *first* port is opened on the
Terraform runner, and `host:port` is resolved *from the YBA VM's
perspective* - `localhost:443` means port 443 on the YBA VM itself, not on
your machine.

Verify the API is reachable before running Terraform:

```sh
curl -k https://127.0.0.1:9443
```

Keep the `ssh` process running for as long as you need Terraform to talk to
YBA - closing the tunnel mid-`apply` fails the in-flight API calls the same
way any other network interruption would.

~> **Note:** If the Terraform runner cannot SSH to the YBA VM directly, hop
through a reachable host with `-J user@jump-host`, or SSH to that host and
replace `localhost` in the forwards with the YBA VM's private address as
seen from there. The command still runs on the Terraform runner, and the
provider configuration below is unchanged. The same applies to cloud-native
port-forwarding tools (`gcloud compute ssh -- -L ...`, AWS SSM port
forwarding, `az network bastion tunnel`): anything that produces a local
TCP listener on the Terraform runner works identically.

## Point the provider at the tunnel

The provider's `host` argument takes an IP address or domain name with port,
no scheme:

```terraform
provider "yba" {
  host      = "127.0.0.1:9443"
  api_token = var.yba_api_token
}
```

Since the tunnel's listener is on the Terraform runner, `host` is always
`127.0.0.1` (or `localhost`) plus the local port you chose - `9443` in these
examples - regardless of where the YBA VM actually lives.

## TLS behavior over the tunnel

~> **Note:** The provider does not validate the YBA server's TLS
certificate (neither the chain nor the hostname) under any circumstances,
tunneled or not. A connection to `127.0.0.1:9443` therefore succeeds even though the
certificate YBA presents was issued for a different hostname; there is
nothing to enable or work around for the tunnel scenario specifically.
Practically, this means the SSH tunnel itself is what authenticates and
encrypts the hop to the VM - treat the tunnel, not the HTTPS layer on top of
it, as the real security boundary.

## Installing YBA through the same tunnel

`yba_installer` drives the install over SSH, so point its SSH fields at the
forwarded `sshd` port on the Terraform runner instead of the VM's real
address:

```terraform
resource "yba_installer" "install" {
  provider    = yba.unauthenticated
  ssh_host_ip = "127.0.0.1"
  ssh_port    = 2222
  ssh_user    = "<ssh-user>"

  ssh_private_key = var.ssh_private_key
  yba_version     = "<yba-version-with-build-number>"
}
```

`ssh_port` is optional and defaults to `22`; set it only when, as here,
`sshd` is reachable through a forwarded port rather than directly on `22`.

~> **Note:** `yba_installer` downloads the YBA install bundle *on the node
itself* - the install commands run over the SSH session and include a
`curl -O` against `downloads.yugabyte.com` (or `releases.yugabyte.com` for
pre-release/CI builds), not against your workstation. The tunnel only needs
to carry the SSH session; the YBA VM needs its own outbound network path to
`downloads.yugabyte.com` / `releases.yugabyte.com` to complete the install.

## Complete example

Putting it together: start the tunnel on the Terraform runner, then apply a
configuration that installs YBA over the forwarded `sshd`, creates the
initial customer through an unauthenticated provider, and is ready for
further resources through an authenticated one.

```sh
ssh -N -L 9443:localhost:443 -L 2222:localhost:22 user@yba-vm
```

```terraform
terraform {
  required_providers {
    yba = {
      source  = "yugabyte/yba"
      version = "~> 1.0"
    }
  }
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
  yba_version     = "<yba-version-with-build-number>"
}

variable "customer_password" {
  type      = string
  sensitive = true
}

resource "yba_customer_resource" "customer" {
  provider   = yba.unauthenticated
  depends_on = [yba_installer.install]
  code       = "admin"
  email      = "<email>"
  name       = "<customer-name>"
  password   = var.customer_password
}

provider "yba" {
  host      = "127.0.0.1:9443"
  api_token = yba_customer_resource.customer.api_token
}
```

Once the customer resource is created, switch subsequent resources
(providers, universes, storage configs, and so on) to the authenticated
`yba` provider, same as any other YBA installation. See
[Running Terraform on existing YugabyteDB Anywhere installations](running-terraform-on-existing-yba-installations)
for that hand-off in more detail.
