terraform {
  required_version = ">= 1.10" # S3 backend native locking (use_lockfile)

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # Partial config: values live in backend.hcl (gitignored). See docs/deployment.md.
  backend "s3" {}
}
