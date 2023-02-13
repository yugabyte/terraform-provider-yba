variable "cluster_name" {
  description = "The name for the cluster (YugabyteDB Anywhere instance) being created."
  type        = string
  default     = "yugaware"
}
variable "image_family" {
  description = "family for gcp compute image"
  type        = string
  default     = "ubuntu-1804-lts"
}
variable "image_project" {
  description = "project for gcp compute image"
  type        = string
  default     = "ubuntu-os-cloud"
}
variable "vpc_network" {
  description = "VPC network to deploy YugabyteDB Anywhere instance"
  default     = "default"
  type        = string
}
variable "vpc_subnetwork" {
  description = "VPC subnetwork to deploy YugabyteDB Anywhere instance"
  default     = null
  type        = string
}
variable "ssh_user" {
  description = "User name to ssh into YugabyteDB Anywhere node to configure cluster"
  type        = string
}
variable "machine_type" {
  description = "Type of machine to be used for YugabyteDB Anywhere instance"
  default     = "n1-standard-4"
  type        = string
}
variable "disk_size" {
  description = "disk size for YugabyteDB Anywhere instance"
  default     = "100"
  type        = string
}
variable "network_tags" {
  description = "network tags to apply to the YugabyteDB Anywhere instance"
  type = list(string)
}

variable "tags" {
  description = "Any tags that need to be added to the Virtual Machine"
  type        = map
}

// file-paths
variable "ssh_private_key" {
  description = "Path to private key to use when connecting to the instances"
  type        = string
}
variable "ssh_public_key" {
  description = "Path to SSH public key to be use when creating the instances"
  type        = string
}