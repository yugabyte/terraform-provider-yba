variable "cluster_name" {
  description = "name of the cluster to be created"
  type        = string
  default     = "yb-anywhere"
}
variable "region_name" {
  description = "region to use for resources"
  type        = string
}
variable "num_nodes" {
  description = "number of nodes to create for the cluster"
  type        = number
}