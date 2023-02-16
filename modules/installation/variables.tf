
variable "public_ip" {
  description = "Public IP address of the VM instance to be accessed"
  type        = string
}
variable "private_ip" {
  description = "Private IP address of the VM instance to be accessed"
  type        = string
}
variable "ssh_host_ip" {
  description = "IP address of the VM instance to be accessed. If VM is within the same network, use private IP, else use public IP"
  type        = string
  default     = "" 
}
variable "ssh_user" {
  description = "User name to ssh into YugabyteDB Anywhere node to configure cluster"
  type        = string
  default     = "centos" 
}
variable "ssh_private_key_file" {
  description = "Private key file to ssh into YugabyteDB Anywhere node to configure cluster"
  type        = string
}
variable "replicated_directory" {
  description = "Directory containing files to configure the replicated environment"
  type        = string
}
variable "replicated_license_file_path" {
  description = "Path to the replicated license file"
  type = string
}