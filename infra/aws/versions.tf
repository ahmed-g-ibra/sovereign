terraform {
  required_version = ">= 1.10"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.40"
    }
  }

  # Partial backend config: bucket and key come from -backend-config at init
  # time so that this file carries no account-specific values.
  #
  # use_lockfile puts the state lock in S3 itself (Terraform 1.10+), which
  # retires the DynamoDB lock table every older guide still tells you to create.
  backend "s3" {
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = "sovereign"
      ManagedBy = "terraform"
      Stack     = "aws"
    }
  }
}

data "aws_caller_identity" "current" {}
