output "public_ip" {
  value = aws_instance.yb_platform_node.public_ip
}

output "private_ip" {
  value = aws_instance.yb_platform_node.private_ip
}