output "public_ip" {
  value = azurerm_public_ip.yb_public_ip.ip_address
}

output "private_ip" {
  value = azurerm_network_interface.yb_network_interface.private_ip_address
}