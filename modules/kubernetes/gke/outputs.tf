output "public_ip" {
  value = data.kubernetes_service.yb_anywhere.spec.external_ips
}