variable "instance_type" {
  description = "The instance type for the platform node"
  type        = string
  default     = "c5.xlarge"
}
variable "volume_size" {
  description = "Volume size for platform node"
  type        = string
  default     = "100"
}
variable "cluster_name" {
  description = "Name for platform cluster"
  type        = string
  default     = "yugaware"
}
variable "ssh_keypair" {
  description = "Name of existing AWS key pair"
  type        = string
}
variable "ssh_user" {
  description = "User name to ssh into platform node to configure cluster"
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
  type = string
}


// file-paths
variable "ssh_private_key" {
  description = "Path to private key to use when connecting to the instances"
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