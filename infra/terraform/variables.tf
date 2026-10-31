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
  description = "owner/repo, used for resource tags."
  type        = string
  default     = "JG3233/gils-garage"
}

variable "github_sub_prefix" {
  description = <<-EOT
    Repo part of the GitHub OIDC `sub` claim allowed to assume the deploy role. This repo uses
    GitHub's immutable subject format (owner and repo IDs, so a renamed/re-registered account
    cannot match). Read it with:
      gh api repos/JG3233/gils-garage/actions/oidc/customization/sub --jq .sub_claim_prefix
    Matched case-sensitively by IAM.
  EOT
  type        = string
  default     = "repo:JG3233@47641679/gils-garage@1426422781"

  validation {
    condition     = can(regex("^repo:[^:]+$", var.github_sub_prefix))
    error_message = "Must look like repo:<owner>/<repo> or repo:<owner>@<id>/<repo>@<id>, with no trailing :context."
  }
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
