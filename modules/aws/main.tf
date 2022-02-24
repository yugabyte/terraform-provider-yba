data "aws_ami" "yb_ami" {
  most_recent = true
  owners      = ["aws-marketplace"]

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

resource "aws_security_group" "yb_security_group" {
  name   = var.security_group_name
  vpc_id = var.vpc_id
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 8800
    to_port     = 8800
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 7000
    to_port     = 7000
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 7100
    to_port     = 7100
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 9000
    to_port     = 9000
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 9100
    to_port     = 9100
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 11000
    to_port     = 11000
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 9300
    to_port     = 9300
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 9042
    to_port     = 9042
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 5433
    to_port     = 5433
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  ingress {
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    self        = true
    cidr_blocks = var.allowed_sources
  }
  egress {
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}

resource "aws_instance" "yb_platform_node" {
  ami                         = data.aws_ami.yb_ami.id
  instance_type               = var.instance_type
  associate_public_ip_address = true
  key_name                    = var.ssh_keypair
  vpc_security_group_ids      = [aws_security_group.yb_security_group.id]
  subnet_id                   = var.subnet_id

  root_block_device {
    volume_size = var.volume_size
  }

  tags = {
    Name = var.cluster_name
  }

  // replicated config
  provisioner "file" {
    source      = var.replicated_filepath
    destination = "/tmp/replicated.conf"
    connection {
      host        = self.public_ip
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // tls certificate
  provisioner "file" {
    content = try(file(var.tls_cert_filepath), "")
    destination = "/tmp/server.crt"
    connection {
      host = self.public_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // tls key
  provisioner "file" {
    content = try(file(var.tls_key_filepath), "")
    destination = "/tmp/server.key"
    connection {
      host = self.public_ip
      type = "ssh"
      user = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // license file
  provisioner "file" {
    source      = var.license_filepath
    destination = "/tmp/license.rli"
    connection {
      host        = self.public_ip
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }

  // application settings
  provisioner "file" {
    source      = var.application_settings_filepath
    destination = "/tmp/settings.conf"
    connection {
      host        = self.public_ip
      type        = "ssh"
      user        = var.ssh_user
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
      host        = self.public_ip
      type        = "ssh"
      user        = var.ssh_user
      private_key = file(var.ssh_private_key)
    }
  }
}