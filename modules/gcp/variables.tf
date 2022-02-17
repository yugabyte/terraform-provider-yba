variable "cluster_name" {
  description = "The name for the cluster (platform instance) being created."
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
variable "node_count" {
  description = "The number of nodes for platform cluster"
  default     = 3
  type        = string
}
variable "vpc_network" {
  description = "VPC network to deploy platform instance"
  default     = "default"
  type        = string
}
variable "vpc_firewall" {
  description = "Firewall used by the YugaByte Node"
  default     = "default"
  type        = string
}
variable "ssh_private_key" {
  description = "Path to private key to use when connecting to the instances"
  type        = string
}
variable "ssh_public_key" {
  description = "Path to SSH public key to be use when creating the instances"
  type        = string
}
variable "ssh_user" {
  description = "User name to ssh into platform node to configure cluster"
  type        = string
}
variable "machine_type" {
  description = "Type of machine to be used for platform instance"
  default     = "n1-standard-4"
  type        = string
}
variable "region_name" {
  description = "Region name for GCP"
  default     = "us-west1"
  type        = string
}
variable "disk_size" {
  description = "disk size for platform instance"
  default     = "100"
  type        = string
}
variable "replicated_filepath" {
  description = "path to replicated config"
  type        = string
}
variable "license_filepath" {
  description = "path to Yugabyte platform license"
  type        = string
}
variable "tls_cert_filepath" {
  description = "path to tls certificate"
  type        = string
}
variable "tls_key_filepath" {
  description = "path to tls private key"
  type        = string
}
variable "application_settings_fielpath" {
  description = "path to platform application settings"
  type        = string
}