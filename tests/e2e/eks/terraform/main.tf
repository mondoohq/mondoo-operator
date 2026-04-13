# Copyright Mondoo, Inc. 2026
# SPDX-License-Identifier: BUSL-1.1

resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-e2e-${random_string.suffix.result}"
  default_tags = {
    Name       = local.name_prefix
    GitHubRepo = "mondoo-operator"
    GitHubOrg  = "mondoohq"
    Terraform  = "true"
  }
}

data "aws_availability_zones" "available" {}
data "aws_caller_identity" "current" {}

################################################################################
# VPC
################################################################################

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "${local.name_prefix}-vpc"
  cidr = "10.0.0.0/16"

  azs             = slice(data.aws_availability_zones.available.names, 0, 2)
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24"]
  public_subnets  = ["10.0.3.0/24", "10.0.4.0/24"]

  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true

  tags = local.default_tags
}

################################################################################
# EKS Cluster (scanner)
################################################################################

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "${local.name_prefix}-cluster"
  cluster_version = var.k8s_version

  cluster_endpoint_public_access           = true
  enable_cluster_creator_admin_permissions = true

  # IRSA is required for WIF external cluster scanning
  enable_irsa = var.enable_wif_test

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  eks_managed_node_groups = {
    default = {
      instance_types = ["m5.large"]
      capacity_type  = "SPOT"
      min_size       = 1
      max_size       = 2
      desired_size   = 1
    }
  }

  tags = local.default_tags
}

################################################################################
# ECR Repository
################################################################################

resource "aws_ecr_repository" "e2e" {
  name                 = "${local.name_prefix}-repo"
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  tags = local.default_tags
}

# Separate ECR repo for private WIF test image (ECR repos are flat, no nesting)
resource "aws_ecr_repository" "private_test" {
  count                = var.enable_wif_test ? 1 : 0
  name                 = "${local.name_prefix}-private-test"
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  tags = local.default_tags
}

################################################################################
# Target Cluster (optional, for external cluster scanning tests)
################################################################################

module "eks_target" {
  count   = var.enable_target_cluster ? 1 : 0
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "${local.name_prefix}-target"
  cluster_version = var.k8s_version

  cluster_endpoint_public_access           = true
  enable_cluster_creator_admin_permissions = true

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  eks_managed_node_groups = {
    default = {
      instance_types = ["m5.large"]
      capacity_type  = "SPOT"
      min_size       = 1
      max_size       = 2
      desired_size   = 1
    }
  }

  tags = local.default_tags
}

# Allow scanner cluster nodes to reach target cluster API server (port 443).
# Both clusters share the same VPC, so DNS resolves to the private endpoint.
# Without this rule, the scanner pods get "i/o timeout" connecting to the target.
resource "aws_security_group_rule" "scanner_to_target_api" {
  count                    = var.enable_target_cluster ? 1 : 0
  type                     = "ingress"
  from_port                = 443
  to_port                  = 443
  protocol                 = "tcp"
  security_group_id        = module.eks_target[0].cluster_primary_security_group_id
  source_security_group_id = module.eks.node_security_group_id
  description              = "Scanner cluster nodes to target cluster API"
}

# EKS Access Entry for scanner role on target cluster (separate resource for conditional creation)
resource "aws_eks_access_entry" "scanner" {
  count         = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  cluster_name  = module.eks_target[0].cluster_name
  principal_arn = aws_iam_role.scanner[0].arn
}

resource "aws_eks_access_policy_association" "scanner" {
  count         = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  cluster_name  = module.eks_target[0].cluster_name
  principal_arn = aws_iam_role.scanner[0].arn
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSViewPolicy"

  access_scope {
    type = "cluster"
  }

  depends_on = [aws_eks_access_entry.scanner]
}

################################################################################
# Kubeconfig
################################################################################

resource "local_file" "kubeconfig" {
  content = templatefile("${path.module}/kubeconfig.tpl", {
    cluster_name     = module.eks.cluster_name
    cluster_endpoint = module.eks.cluster_endpoint
    cluster_ca       = module.eks.cluster_certificate_authority_data
    region           = var.region
    profile          = var.profile != null ? var.profile : ""
  })
  filename = "${path.module}/kubeconfig"
}

resource "local_file" "kubeconfig_target" {
  count = var.enable_target_cluster ? 1 : 0
  content = templatefile("${path.module}/kubeconfig.tpl", {
    cluster_name     = module.eks_target[0].cluster_name
    cluster_endpoint = module.eks_target[0].cluster_endpoint
    cluster_ca       = module.eks_target[0].cluster_certificate_authority_data
    region           = var.region
    profile          = var.profile != null ? var.profile : ""
  })
  filename = "${path.module}/kubeconfig-target"
}

################################################################################
# WIF: IAM Role for scanner (IRSA)
################################################################################

resource "aws_iam_role" "scanner" {
  count = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  name  = "${local.name_prefix}-wif-scanner"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Federated = module.eks.oidc_provider_arn
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "${module.eks.oidc_provider}:aud" = "sts.amazonaws.com"
          }
          StringLike = {
            # Allow both external cluster (mondoo-client-wif-target-cluster) and
            # container registry WIF (mondoo-client-cr-wif) KSAs
            "${module.eks.oidc_provider}:sub" = "system:serviceaccount:mondoo-operator:mondoo-client-*"
          }
        }
      },
    ]
  })

  tags = local.default_tags
}

resource "aws_iam_role_policy" "eks_access" {
  count = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  name  = "eks-access"
  role  = aws_iam_role.scanner[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "eks:DescribeCluster",
          "eks:ListClusters",
        ]
        Resource = "*"
      },
    ]
  })
}

# Grant the scanner role read access to ECR (for container registry WIF scanning)
resource "aws_iam_role_policy" "ecr_read" {
  count = var.enable_wif_test && var.enable_target_cluster ? 1 : 0
  name  = "ecr-read"
  role  = aws_iam_role.scanner[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecr:GetAuthorizationToken",
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "ecr:BatchCheckLayerAvailability",
          "ecr:GetDownloadUrlForLayer",
          "ecr:BatchGetImage",
        ]
        Resource = [
          aws_ecr_repository.e2e.arn,
          var.enable_wif_test ? aws_ecr_repository.private_test[0].arn : aws_ecr_repository.e2e.arn,
        ]
      },
    ]
  })
}
