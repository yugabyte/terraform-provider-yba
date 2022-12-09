output "public_ip" {
  value = var.service_type == "ClusterIP" ? data.kubernetes_service.yb_anywhere.spec.0.cluster_ip : data.kubernetes_service.yb_anywhere.status.0.load_balancer.0.ingress.0.hostname
}