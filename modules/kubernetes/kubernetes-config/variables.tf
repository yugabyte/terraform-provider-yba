variable "cluster_name" {
  description = "name of the cluster to be created"
  type        = string
  default     = "yb-anywhere"
}

variable "service_type" {
  description = "Whether the Yugaware Service type should be LoadBalancer or ClusterIP"
  type        = string
  default     = "ClusterIP"
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