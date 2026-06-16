# Copyright 2026 YugabyteDB, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# One VPC with four subnets (one for the YBA control-plane VM, three for the
# YBDB universe nodes YBA provisions, one per us-west-2 zone), an internet
# gateway + public route table so the YBA VM can reach the internet for the
# install, and a security group opening operator access plus intra-VPC traffic.
# One VPC means no peering needed.

# us-west-2a/b/c, in order. The YBA VM lands in the first; the three YBDB
# subnets map one-to-one onto these zones.
locals {
  azs = [for s in ["a", "b", "c"] : "${var.aws_region}${s}"]
}

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${var.prefix}-vpc" }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = { Name = "${var.prefix}-igw" }
}

# Subnet hosting the YBA control-plane VM. Public so the installer (and the
# UI/API) can reach it over the internet.
resource "aws_subnet" "yba" {
  vpc_id                  = aws_vpc.main.id
  cidr_block              = var.yba_subnet_cidr
  availability_zone       = local.azs[0]
  map_public_ip_on_launch = true

  tags = { Name = "${var.prefix}-yba" }
}

# Three subnets hosting YBDB universe nodes, one per us-west-2 zone. The AWS
# multi-zone acceptance test references the first two (AWS_ZONE_SUBNET_ID and
# AWS_ZONE_SUBNET_ID_2); the single-zone tests use the first (AWS_ZONE_SUBNET_ID).
resource "aws_subnet" "ybdb" {
  count                   = length(var.ybdb_subnet_cidrs)
  vpc_id                  = aws_vpc.main.id
  cidr_block              = var.ybdb_subnet_cidrs[count.index]
  availability_zone       = local.azs[count.index]
  map_public_ip_on_launch = true

  tags = { Name = "${var.prefix}-ybdb-${count.index + 1}" }
}

# Single public route table (default route to the IGW), associated with every
# subnet so YBA and the universe nodes have outbound internet.
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = { Name = "${var.prefix}-public" }
}

resource "aws_route_table_association" "yba" {
  subnet_id      = aws_subnet.yba.id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table_association" "ybdb" {
  count          = length(aws_subnet.ybdb)
  subnet_id      = aws_subnet.ybdb[count.index].id
  route_table_id = aws_route_table.public.id
}

# Security group, kept minimal: allow everything inside the VPC (and within the
# group itself, for the YBDB nodes YBA places here), plus operator access (SSH +
# YBA UI/API + Prometheus) from var.operator_cidr_ranges. Exposed as
# TF_VAR_AWS_SG_ID for the yba_aws_provider tests.
#
# Rules are declared inline (not as standalone aws_security_group_rule resources)
# so Terraform owns the full rule set and reconciles it declaratively; mixing the
# two models lets externally-present rules collide with standalone ones
# (InvalidPermission.Duplicate).
resource "aws_security_group" "main" {
  name        = "${var.prefix}-sg"
  description = "YBA acceptance-test: intra-VPC + operator access"
  vpc_id      = aws_vpc.main.id

  # All traffic between members of this group (the YBDB nodes YBA places here).
  ingress {
    description = "Intra-SG (self) all traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
  }

  # All traffic from inside the VPC (YBA <-> YBDB, YBDB <-> YBDB).
  ingress {
    description = "Intra-VPC all traffic"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }

  # Operator access: SSH for the installer, 443 for the YBA UI/API, 9090 for
  # Prometheus.
  ingress {
    description = "Operator SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.operator_cidr_ranges
  }

  ingress {
    description = "Operator YBA UI/API"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = var.operator_cidr_ranges
  }

  ingress {
    description = "Operator Prometheus"
    from_port   = 9090
    to_port     = 9090
    protocol    = "tcp"
    cidr_blocks = var.operator_cidr_ranges
  }

  egress {
    description = "Allow all egress"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${var.prefix}-sg" }
}
