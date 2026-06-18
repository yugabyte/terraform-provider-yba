# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# The acceptance-test YBA: a control-plane VM, the YBA install (over SSH), and
# the initial customer. Applied once as part of the base; its endpoint
# (TF_VAR_AWS_YBA_HOST/TF_VAR_AWS_YBA_API_KEY) is exposed through the `test_env`
# output so `make acctest` just consumes it. Tear down with the base.
# Mirrors acctest/gcp and acctest/azure.

locals {
  yba_ssh_host = aws_eip.yba.public_ip
}

# Dedicated SSH keypair for the standing YBA VM, generated once and kept in the
# shared remote state (alongside random_password.customer). The public half goes
# on the VM and the private half is fed to the installer inline via
# ssh_private_key, so applies don't depend on (or drift against) any developer's
# ~/.ssh keypair. Passing it inline (not as a file path) sidesteps the
# installer's plan-time file-existence check, which a key generated in the same
# apply can't satisfy.
resource "tls_private_key" "yba" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Resolve base_image (an owner + name pattern) to a concrete AMI. Exposed as
# TF_VAR_AWS_AMI_ID for the provider tests' custom image bundles, and used as
# the YBA VM's boot image below.
data "aws_ami" "almalinux" {
  most_recent = true
  owners      = [var.base_image.owner]

  filter {
    name   = "name"
    values = [var.base_image.name_pattern]
  }
  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# EC2 key pair the installer's SSH key maps to.
resource "aws_key_pair" "yba" {
  key_name   = "${var.prefix}-yba"
  public_key = tls_private_key.yba.public_key_openssh
}

# Stable public IP for the YBA VM: a fixed address for the installer (SSH) and
# the UI/API. Exposed as TF_VAR_AWS_YBA_HOST.
resource "aws_eip" "yba" {
  domain = "vpc"

  tags = { Name = "${var.prefix}-yba" }
}

# Persistent state for YBA, mounted at /opt/yugabyte/data by the user-data
# script. Kept on a separate volume as in byoc-setup. Must be in the VM's AZ.
resource "aws_ebs_volume" "data" {
  availability_zone = local.azs[0]
  size              = 250
  type              = "gp3"

  tags = { Name = "${var.prefix}-yba-data" }
}

# Single YBA control-plane VM (no HA).
resource "aws_instance" "yba" {
  ami                    = data.aws_ami.almalinux.id
  instance_type          = var.yba_instance_type
  subnet_id              = aws_subnet.yba.id
  vpc_security_group_ids = [aws_security_group.main.id]
  key_name               = aws_key_pair.yba.key_name
  iam_instance_profile   = aws_iam_instance_profile.yba.name

  # user_data runs the mount/preflight script on first boot (cloud-init runs an
  # executable user_data payload directly).
  user_data = file("${path.module}/../resources/aws-mount-data-disk.sh")

  root_block_device {
    volume_size = 100
    volume_type = "gp3"
  }

  tags = { Name = "${var.prefix}-yba" }
}

# Attach the data volume; the user-data script resolves it as the non-root
# block device.
resource "aws_volume_attachment" "data" {
  device_name = "/dev/sdf"
  volume_id   = aws_ebs_volume.data.id
  instance_id = aws_instance.yba.id
}

resource "aws_eip_association" "yba" {
  instance_id   = aws_instance.yba.id
  allocation_id = aws_eip.yba.id
}

# Randomly generated password for the initial YBA superuser.
resource "random_password" "customer" {
  length           = 16
  min_upper        = 1
  min_lower        = 1
  min_numeric      = 1
  min_special      = 1
  override_special = "!#$%*-_"
}

# Install YugabyteDB Anywhere on the VM over SSH. The SSH key is the generated
# keypair (passed inline); the license file is at the repo root and yba-ctl.yml
# is in acctest/resources (both validated to exist at plan time). The installer
# connects as the VM admin user.
resource "yba_installer" "install" {
  provider = yba.bootstrap

  ssh_host_ip               = local.yba_ssh_host
  ssh_user                  = var.yba_admin_user
  ssh_private_key           = tls_private_key.yba.private_key_openssh
  yba_license_file          = "${path.module}/../../yugabyte_anywhere.lic"
  application_settings_file = "${path.module}/../resources/yba-ctl.yml"
  yba_version               = var.yba_version
  host_os                   = "linux"
  host_architecture         = "x86_64"

  # The instance is an implicit dependency (via ssh_host_ip → EIP), but the data
  # volume, the EIP association that makes that address routable, the SG that
  # opens SSH/443, and the route to the internet are not — without these the
  # installer can start before the disk is mounted or the host is reachable and
  # fail.
  depends_on = [
    aws_volume_attachment.data,
    aws_eip_association.yba,
    aws_security_group.main,
    aws_route_table_association.yba,
  ]
}

# Register the initial superuser; exposes the API token (published as
# TF_VAR_AWS_YBA_API_KEY).
resource "yba_customer_resource" "customer" {
  provider = yba.bootstrap

  code     = "admin"
  email    = var.yba_username
  name     = "admin"
  password = random_password.customer.result

  lifecycle {
    ignore_changes = [password]
  }

  depends_on = [yba_installer.install]
}
