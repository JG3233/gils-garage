output "site_url" {
  value = "https://${var.domain_name}"
}

output "site_bucket" {
  description = "Set as GitHub environment variable SITE_BUCKET."
  value       = aws_s3_bucket.site.bucket
}

output "cloudfront_distribution_id" {
  description = "Set as GitHub environment variable CF_DISTRIBUTION_ID."
  value       = aws_cloudfront_distribution.site.id
}

output "deploy_role_arn" {
  description = "Set as GitHub environment variable AWS_DEPLOY_ROLE_ARN."
  value       = aws_iam_role.deploy.arn
}
