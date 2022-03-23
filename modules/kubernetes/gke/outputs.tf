output "public_ip" {
  value = module.gke-cluster.endpoint
}