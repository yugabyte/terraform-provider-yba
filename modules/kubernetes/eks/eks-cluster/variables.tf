variable "cluster_name" {
  description = "name of the cluster to be created"
  type = string
  default = "yb-anywhere"
}
variable "vpc_id" {
  description = "ID of the VPC to use for creating this cluster"
  type = string
}