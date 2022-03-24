data "aws_iam_role" "role" {
  name = var.iam_role
}

resource "aws_eks_cluster" "yb-anywhere" {
  name     = var.cluster_name
  role_arn = data.aws_iam_role.role.arn

  vpc_config {
    subnet_ids = var.subnet_ids
  }
}

resource "aws_eks_node_group" "yb-anywhere" {
  cluster_name    = aws_eks_cluster.yb-anywhere.name
  node_group_name = var.cluster_name
  node_role_arn   = data.aws_iam_role.role.arn
  subnet_ids      = var.subnet_ids

  scaling_config {
    desired_size = var.node_count
    max_size     = var.node_count
    min_size     = var.node_count
  }
}