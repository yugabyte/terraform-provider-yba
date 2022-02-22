output "ami" {
  value = data.aws_ami.yb_ami.name
}