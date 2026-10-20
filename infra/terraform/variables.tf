variable "domain_name" {
  description = "Apex domain for the blog (must have a Route 53 public hosted zone in this account)."
  type        = string
}

variable "region" {
  description = "Region for the S3 origin bucket."
  type        = string
  default     = "us-east-2"
}

variable "github_repo" {
  description = "owner/repo allowed to assume the deploy role."
  type        = string
  default     = "JG3233/gils-garage"
}

variable "github_environment" {
  description = "GitHub Actions environment the deploy job runs in; pinned in the OIDC trust policy."
  type        = string
  default     = "production"
}

variable "create_oidc_provider" {
  description = "Create the GitHub OIDC provider. Set false if the account already has one."
  type        = bool
  default     = true
}

variable "include_www" {
  description = "Also serve www.<domain_name>."
  type        = bool
  default     = true
}
