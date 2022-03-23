output "public_ip" {
  value = data.kubernetes_service.yb_anywhere.status[0].load_balancer[0].ingress[0].ip
}