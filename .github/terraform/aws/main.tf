################################################################################
# Data & Locals
################################################################################

data "aws_availability_zones" "available" {}

data "aws_caller_identity" "current" {}

locals {
  name            = "${var.test_name}-integration-tests-${random_string.suffix.result}"
  cluster_version = var.kubernetes_version
  default_tags = {
    Name       = local.name
    GitHubRepo = "mondoo-operator"
    GitHubOrg  = "mondoohq"
    Terraform  = true
  }
}

resource "random_string" "suffix" {
  length  = 4
  special = false
  upper   = false
}

################################################################################
# VPC Configuration
################################################################################

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 3.14.0"

  name                 = "${local.name}-vpc"
  cidr                 = "10.0.0.0/16"
  azs                  = data.aws_availability_zones.available.names
  private_subnets      = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets       = ["10.0.4.0/24", "10.0.5.0/24", "10.0.6.0/24"]
  enable_dns_hostnames = true

  enable_flow_log                      = false

  tags = merge(
    local.default_tags, {
      "Name"                                = "${local.name}-vpc"
      "kubernetes.io/cluster/integration-tests" = var.test_name
    },
  )

  public_subnet_tags = {
    "kubernetes.io/cluster/integration-tests" = var.test_name
    "kubernetes.io/role/elb"              = "1"
  }

  private_subnet_tags = {
    "kubernetes.io/cluster/integration-tests" = var.test_name
    "kubernetes.io/role/internal-elb"     = "1"
  }
}


################################################################################
# EKS Module
################################################################################

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 18.28.0"

  cluster_name                    = "${local.name}-cluster"
  cluster_version                 = "${local.cluster_version}"
  cluster_endpoint_private_access = false
  cluster_endpoint_public_access  = true

  cluster_addons = {
    coredns = {
      resolve_conflicts = "OVERWRITE"
    }
    kube-proxy = {}
    vpc-cni = {
      resolve_conflicts = "OVERWRITE"
    }
  }

  cluster_encryption_config = [{
    provider_key_arn = aws_kms_key.eks.arn
    resources        = ["secrets"]
  }]

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  manage_aws_auth_configmap = true

  # Extend cluster security group rules
  cluster_security_group_additional_rules = {
    egress_nodes_ephemeral_ports_tcp = {
      description                = "To node 1025-65535"
      protocol                   = "tcp"
      from_port                  = 1025
      to_port                    = 65535
      type                       = "egress"
      source_node_security_group = true
    }
  }

  # Extend node-to-node security group rules
  node_security_group_additional_rules = {
    ingress_webhook = {
      description = "Allow ingress to Mondoo webhook"
      protocol    = "tcp"
      from_port   = 9443
      to_port     = 9443
      type        = "ingress"
      cidr_blocks = ["0.0.0.0/0"]
    }
    ingress_self_all = {
      description = "Node to node all ports/protocols"
      protocol    = "-1"
      from_port   = 0
      to_port     = 0
      type        = "ingress"
      self        = true
    }
    egress_all = {
      description      = "Node all egress"
      protocol         = "-1"
      from_port        = 0
      to_port          = 0
      type             = "egress"
      cidr_blocks      = ["0.0.0.0/0"]
      ipv6_cidr_blocks = ["::/0"]
    }
  }

  iam_role_use_name_prefix = false

  eks_managed_node_group_defaults = {
    ami_type       = "AL2_x86_64"
    disk_size      = 20
    instance_types = ["m5.large"]

  }
  eks_managed_node_groups = {
    complete = {
      name            = "eks-managed-nodes-${random_string.suffix.result}"
      use_name_prefix = true

      subnet_ids = module.vpc.public_subnets

      ami_type = "AL2_x86_64"

      min_size     = 1
      max_size     = 2
      desired_size = 2

      ami_id                     = data.aws_ami.eks_default.image_id
      enable_bootstrap_user_data = true
      bootstrap_extra_args       = "--kubelet-extra-args '--max-pods=20'"

      pre_bootstrap_user_data = <<-EOT
      export USE_MAX_PODS=false
      EOT

      post_bootstrap_user_data = <<-EOT
      echo "you are free little kubelet!"
      EOT

      capacity_type        = "SPOT"
      disk_size            = 20
      force_update_version = true
      instance_types       = ["m5.large", "m5n.large", "m5zn.large"]
      labels = {
        GithubRepo = "terraform-aws-eks"
        GithubOrg  = "terraform-aws-modules"
      }

      update_config = {
        max_unavailable_percentage = 50 # or set `max_unavailable`
      }

      description = "${var.test_name} EKS managed node group"

      ebs_optimized           = true
      disable_api_termination = false
      enable_monitoring       = false

      block_device_mappings = {
        xvda = {
          device_name = "/dev/xvda"
          ebs = {
            volume_size           = 20
            volume_type           = "gp3"
            iops                  = 3000
            throughput            = 150
            encrypted             = false
            delete_on_termination = true
          }
        }
      }

      create_iam_role          = true
      iam_role_name            = "${var.test_name}-iam-role"
      iam_role_use_name_prefix = false
      iam_role_description     = "${var.test_name} EKS managed node group role"
      iam_role_tags = {
        Purpose = "Protector of the kubelet"
      }
      iam_role_additional_policies = [
        "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
        "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
        "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
        "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
      ]

    }
  }

  tags = local.default_tags
}

################################################################################
# Supporting Resoures
################################################################################

resource "aws_kms_key" "eks" {
  description             = "EKS Secret Encryption Key"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  tags = local.default_tags
}

data "aws_ami" "eks_default" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["amazon-eks-node-${local.cluster_version}-v*"]
  }
}


################################################################################
# Create kubeconfig
################################################################################

resource "null_resource" "kubectl_config_update" {
  depends_on = [
    module.eks
  ]
  provisioner "local-exec" {
    command = "aws eks --region $(terraform output -raw region) update-kubeconfig --name $(terraform output -raw cluster_name) --kubeconfig ./eks-config"
  }
}

################################################################################
# Install additions
################################################################################

resource "null_resource" "kubectl_install_cert_manager" {
  depends_on = [
    module.eks,
    null_resource.kubectl_config_update
  ]
  provisioner "local-exec" {
    command = "kubectl --kubeconfig eks-config apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.8.0/cert-manager.yaml"
  }
}

resource "null_resource" "kubectl_install_psp" {
  depends_on = [
    module.eks,
    null_resource.kubectl_config_update
  ]
  provisioner "local-exec" {
    command = "kubectl --kubeconfig eks-config apply -f ./psp-unprivileged.yaml"
  }
}