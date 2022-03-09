output "public_ip" {
  value = azurerm_public_ip.yb_public_ip.ip_address
}