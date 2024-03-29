variable "num_nodes" {
  description = "gke username"
  type        = number
}
variable "network" {
  description = "name of the network to create the cluster in"
  type        = string
}
variable "subnet" {
  description = "name of the subnet belonging to the network to created the cluster in"
  type        = string
}
variable "cluster_name" {
  description = "name of the cluster to be created"
  type        = string
  default     = "yb-anywhere"
}
variable "docker_config_json" {
  description = ".dockerconfigjson field from provided Yugabyte kubernetes secret"
  type        = string
}
variable "chart_version" {
  description = "version of the helm chart to install"
  type        = string
  default     = null
}
variable "memory_max" {
  description = "maximum amount of ram available to the cluster in GB"
  type        = string
}
variable "cpu_max" {
  description = "maximum amount of cpu available to the cluster in GB"
  type        = string
}