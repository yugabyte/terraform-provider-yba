variable "instance_type" {
  description = "The instance type for the YugabyteDB Anywhere node"
  type        = string
  default     = "c5.xlarge"
}
variable "volume_size" {
  description = "Volume size for YugabyteDB Anywhere node"
  type        = string
  default     = "100"
}
variable "cluster_name" {
  description = "Name for YugabyteDB Anywhere cluster"
  type        = string
  default     = "yugaware"
}
variable "ssh_keypair" {
  description = "Name of existing AWS key pair"
  type        = string
}
variable "ssh_user" {
  description = "User name to ssh into the YugabyteDB Anywhere node to configure cluster"
  type        = string
}
variable "security_group_name" {
  description = "Name for the created security group"
  type        = string
}
variable "allowed_sources" {
  description = "source ips to restrict traffic"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}
variable "vpc_id" {
  description = "VPC ID"
  type        = string
}
variable "subnet_id" {
  description = "ID of subnet in VPC"
  type        = string
}


// file-paths
variable "ssh_private_key" {
  description = "Path to private key to use when connecting to the instances"
  type        = string
}