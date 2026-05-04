# Terraform module for deploying Ubuntu Auto-Update on AWS
# Creates: VPC, EKS cluster, RDS PostgreSQL, S3 bucket, and ECR registry.
#
# Usage:
#   cd infrastructure/terraform
#   terraform init
#   terraform plan -var 'region=us-east-1'
#   terraform apply

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # Remote state — uncomment and configure for team use.
  # backend "s3" {
  #   bucket  = "uau-terraform-state"
  #   key     = "production/terraform.tfstate"
  #   region  = "us-east-1"
  #   encrypt = true
  # }
}

variable "region" {
  description = "AWS region for deployment"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Deployment environment (dev, staging, production)"
  type        = string
  default     = "production"
}

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t3.medium"
}

variable "eks_node_instance_type" {
  description = "EKS node EC2 instance type"
  type        = string
  default     = "t3.medium"
}

variable "eks_desired_capacity" {
  description = "Desired number of EKS worker nodes"
  type        = number
  default     = 2
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project     = "ubuntu-auto-update"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

# ── VPC ──────────────────────────────────────────────────────────────────
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "uau-${var.environment}"
  cidr = "10.0.0.0/16"

  azs             = ["${var.region}a", "${var.region}b", "${var.region}c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway   = true
  single_nat_gateway   = var.environment != "production"
  enable_dns_hostnames = true
  enable_dns_support   = true

  public_subnet_tags = {
    "kubernetes.io/role/elb" = "1"
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = "1"
  }
}

# ── EKS Cluster ──────────────────────────────────────────────────────────
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.0"

  cluster_name    = "uau-${var.environment}"
  cluster_version = "1.29"

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  cluster_endpoint_public_access = true

  eks_managed_node_groups = {
    default = {
      instance_types = [var.eks_node_instance_type]
      min_size       = 1
      max_size       = 5
      desired_size   = var.eks_desired_capacity
    }
  }
}

# ── RDS PostgreSQL ───────────────────────────────────────────────────────
resource "aws_db_subnet_group" "uau" {
  name       = "uau-${var.environment}"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_security_group" "rds" {
  name_prefix = "uau-rds-"
  vpc_id      = module.vpc.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [module.eks.cluster_security_group_id]
  }
}

resource "aws_db_instance" "uau" {
  identifier     = "uau-${var.environment}"
  engine         = "postgres"
  engine_version = "15"
  instance_class = var.db_instance_class

  allocated_storage     = 20
  max_allocated_storage = 100
  storage_encrypted     = true

  db_name  = "uau_db"
  username = "uau_admin"
  # In production, use aws_secretsmanager_secret for the password.
  manage_master_user_password = true

  db_subnet_group_name   = aws_db_subnet_group.uau.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  multi_az            = var.environment == "production"
  skip_final_snapshot = var.environment != "production"

  backup_retention_period = var.environment == "production" ? 30 : 7
}

# ── S3 Bucket (for backups / object storage) ─────────────────────────────
resource "aws_s3_bucket" "uau" {
  bucket = "uau-${var.environment}-${data.aws_caller_identity.current.account_id}"
}

resource "aws_s3_bucket_versioning" "uau" {
  bucket = aws_s3_bucket.uau.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "uau" {
  bucket = aws_s3_bucket.uau.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "aws:kms"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "uau" {
  bucket                  = aws_s3_bucket.uau.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ── ECR Repository ───────────────────────────────────────────────────────
resource "aws_ecr_repository" "uau" {
  name                 = "ubuntu-auto-update"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "KMS"
  }
}

data "aws_caller_identity" "current" {}

# ── Outputs ──────────────────────────────────────────────────────────────
output "eks_cluster_endpoint" {
  value = module.eks.cluster_endpoint
}

output "rds_endpoint" {
  value = aws_db_instance.uau.endpoint
}

output "s3_bucket" {
  value = aws_s3_bucket.uau.id
}

output "ecr_repository_url" {
  value = aws_ecr_repository.uau.repository_url
}
