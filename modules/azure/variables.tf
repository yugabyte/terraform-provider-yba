variable "cluster_name" {
  description = "Name for platform cluster"
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
  description = "name of the subnet to use for the platform instance"
  type        = string
}
variable "vnet_name" {
  description = "name of the virtual network to use for the platform instance"
  type        = string
}
variable "vnet_resource_group" {
  description = "name of the resource group associated with the virtual network"
  type        = string
}

// files
variable "ssh_private_key" {
  description = "Path to private key to use when connecting to the instances"
  type        = string
}
variable "ssh_public_key" {
  description = "Path to SSH public key to be use when creating the instances"
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
variable "application_settings_filepath" {
  description = "path to platform application settings"
  type        = string
}
