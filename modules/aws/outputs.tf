output "public_ip" {
  value = aws_instance.yb_anywhere_node.public_ip
}

output "private_ip" {
  value = aws_instance.yb_anywhere_node.private_ip
}