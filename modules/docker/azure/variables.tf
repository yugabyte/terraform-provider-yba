variable "cluster_name" {
  description = "Name for YugabyteDB Anywhere cluster"
  type        = string
  default     = "yugaware"
}
variable "region_name" {
  description = "region to use for resources"
  type        = string
}
variable "vm_size" {
  description = "vm specs"
  type        = string
  default     = "Standard_D4s_v3"
}
variable "disk_size" {
  description = "disk size"
  type        = string
  default     = "100"
}
variable "ssh_user" {
  description = "name of the ssh user"
  type        = string
}
variable "subnet_name" {
  description = "name of the subnet to use for the YugabyteDB Anywhere instance"
  type        = string
}
variable "vnet_name" {
  description = "name of the virtual network to use for the YugabyteDB Anywhere instance"
  type        = string
}
variable "resource_group" {
  description = "name of the resource group that all existing and created resources should belong"
  type        = string
}

variable "tags" {
  description = "Any tags that need to be added to the Virtual Machine"
  type        = map(string)
  default     = {}
}

// files
variable "ssh_private_key" {
  description = "Path to private key to use when connecting to the instances"
  type        = string
  sensitive   = true
}
variable "ssh_public_key" {
  description = "Path to SSH public key to be use when creating the instances"
  type        = string
  sensitive   = true
}

variable "security_group" {
  description = "Security group for the VM"
  type        = string
}


variable "runner_ip" {
  description = "IP of the runners to be ablee to connect to the instances"
  type = string
}