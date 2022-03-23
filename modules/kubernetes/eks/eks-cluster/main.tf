resource "aws_iam_role" "yb-anywhere-cluster" {
  name = var.cluster_name

  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "eks.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
POLICY
}

resource "aws_iam_role_policy_attachment" "yb-anywhere-AmazonEKSClusterPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.yb-anywhere-cluster.name
}

# Optionally, enable Security Groups for Pods
# Reference: https://docs.aws.amazon.com/eks/latest/userguide/security-groups-for-pods.html
resource "aws_iam_role_policy_attachment" "yb-anywhere-AmazonEKSVPCResourceController" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSVPCResourceController"
  role       = aws_iam_role.yb-anywhere-cluster.name
}

resource "aws_iam_role" "yb-anywhere-node" {
  name = "${var.cluster_name}-node"

  assume_role_policy = jsonencode({
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
    Version = "2012-10-17"
  })
}

resource "aws_iam_role_policy_attachment" "yb-anywhere-AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.yb-anywhere-node.name
}

resource "aws_iam_role_policy_attachment" "yb-anywhere-AmazonEKS_CNI_Policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.yb-anywhere-node.name
}

resource "aws_iam_role_policy_attachment" "yb-anywhere-AmazonEC2ContainerRegistryReadOnly" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.yb-anywhere-node.name
}

data "aws_subnets" "subnets" {
  filter {
    name   = "vpc-id"
    values = [var.vpc_id]
  }
}

resource "aws_eks_cluster" "yb-anywhere" {
  name     = var.cluster_name
  role_arn = aws_iam_role.yb-anywhere-cluster.arn

  vpc_config {
    subnet_ids = data.aws_subnets.subnets.ids
  }

  # Ensure that IAM Role permissions are created before and deleted after EKS Cluster handling.
  # Otherwise, EKS will not be able to properly delete EKS managed EC2 infrastructure such as Security Groups.
  depends_on = [
    aws_iam_role_policy_attachment.yb-anywhere-AmazonEKSClusterPolicy,
    aws_iam_role_policy_attachment.yb-anywhere-AmazonEKSVPCResourceController,
  ]
}

resource "aws_eks_node_group" "yb-anywhere" {
  cluster_name    = aws_eks_cluster.yb-anywhere.name
  node_group_name = var.cluster_name
  node_role_arn   = aws_iam_role.yb-anywhere-node.arn
  subnet_ids      = data.aws_subnets.subnets.ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }

  # Ensure that IAM Role permissions are created before and deleted after EKS Node Group handling.
  # Otherwise, EKS will not be able to properly delete EC2 Instances and Elastic Network Interfaces.
  depends_on = [
    aws_iam_role_policy_attachment.yb-anywhere-AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.yb-anywhere-AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.yb-anywhere-AmazonEC2ContainerRegistryReadOnly,
  ]
}