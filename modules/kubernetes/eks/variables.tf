variable "cluster_name" {
  description = "name of the cluster to be created"
  type        = string
  default     = "yb-anywhere"
}
variable "vpc_id" {
  description = "ID of the VPC to use for creating this cluster"
  type        = string
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
variable "iam_role" {
  description = "name of the IAM role to use for the cluster"
  type        = string
}
variable "node_count" {
  description = "number of nodes to create for the cluster"
  type        = number
}
variable "subnet_ids" {
  description = "ids of subnets to use for the cluster"
  type        = list(string)
}