data "aws_subnets" "subnets" {
  filter {
    name   = "vpc-id"
    values = [var.vpc_id]
  }
}

data "aws_iam_role" "role" {
  name = var.iam_role
}

resource "aws_eks_cluster" "yb-anywhere" {
  name     = var.cluster_name
  role_arn = data.aws_iam_role.role.arn

  vpc_config {
    subnet_ids = data.aws_subnets.subnets.ids
  }
}

resource "aws_eks_node_group" "yb-anywhere" {
  cluster_name    = aws_eks_cluster.yb-anywhere.name
  node_group_name = var.cluster_name
  node_role_arn   = data.aws_iam_role.role.arn
  subnet_ids      = data.aws_subnets.subnets.ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }
}