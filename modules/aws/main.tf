data "aws_ami" "yb_ami" {
  most_recent = true
  owners = ["aws-marketplace"]

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-xenial-16.04-amd64-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
  filter {
    name   = "architecture"
    values = ["x86_64"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }
}

resource "aws_instance" "yb_platform_node" {
  ami = data.aws_ami.yb_ami.id
  instance_type = var.instance_type
  associate_public_ip_address = true
  key_name = var.ssh_keypair

  root_block_device {
    volume_size = var.volume_size
  }

  tags = {
    Name = var.cluster_name
  }

  // replicated config
  provisioner "file" {
    source = var.replicated_filepath
    destination ="/tmp/replicated.conf"
    connection {
      host = self.public_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // tls certificate
  #  provisioner "file" {
  #    source = var.tls_cert_filepath
  #    destination ="/tmp/server.crt"
  #    connection {
  #      host = self.public_ip
  #      type = "ssh"
  #      user = var.ssh_user
  #      private_key = file(var.ssh_private_key)
  #    }
  #  }

  // tls key
  #  provisioner "file" {
  #    source = var.tls_key_filepath
  #    destination ="/tmp/server.key"
  #    connection {
  #      host = self.public_ip
  #      type = "ssh"
  #      user = var.ssh_user
  #      private_key = file(var.ssh_private_key)
  #    }
  #  }

  // license file
  provisioner "file" {
    source = var.license_filepath
    destination ="/tmp/license.rli"
    connection {
      host = self.public_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // application settings
  provisioner "file" {
    source = var.application_settings_filepath
    destination ="/tmp/settings.conf"
    connection {
      host = self.public_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // install replicated
  provisioner "remote-exec" {
    inline = [
      "sudo mv /tmp/replicated.conf /etc/replicated.conf",
      "curl -sSL https://get.replicated.com/docker | sudo bash",
    ]
    connection {
      host = self.public_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }
}