resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

locals {
  name_prefix = "mondoo-wif-${random_string.suffix.result}"
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
# VPC (shared by both clusters)
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
# Management Cluster (runs the operator, IRSA enabled)
################################################################################

module "management" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "${local.name_prefix}-mgmt"
  cluster_version = var.k8s_version

  cluster_endpoint_public_access = true
  enable_irsa                    = true

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
# Target Cluster (to be scanned)
################################################################################

module "target" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "${local.name_prefix}-target"
  cluster_version = var.k8s_version

  cluster_endpoint_public_access = true

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

  # Map the scanner IAM role so it can access this cluster
  access_entries = {
    mondoo-scanner = {
      principal_arn = aws_iam_role.scanner.arn
      policy_associations = {
        cluster_view = {
          policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSViewPolicy"
          access_scope = {
            type = "cluster"
          }
        }
      }
    }
  }

  tags = local.default_tags
}

################################################################################
# IAM Role for the scanner (IRSA)
################################################################################

resource "aws_iam_role" "scanner" {
  name = "${local.name_prefix}-scanner"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Federated = module.management.oidc_provider_arn
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "${module.management.oidc_provider}:aud" = "sts.amazonaws.com"
            "${module.management.oidc_provider}:sub" = "system:serviceaccount:${var.scanner_namespace}:${var.scanner_service_account}"
          }
        }
      },
    ]
  })

  tags = local.default_tags
}

resource "aws_iam_role_policy" "eks_access" {
  name = "eks-access"
  role = aws_iam_role.scanner.id

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
