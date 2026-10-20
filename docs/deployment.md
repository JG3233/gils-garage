# Deployment

One-time setup, then every push to `main` that touches site content or code deploys automatically.

## Prerequisites

- An AWS account and an admin-ish profile for the first `terraform apply`.
- Your domain's DNS hosted in a **Route 53 public hosted zone** in that account. If the domain is registered elsewhere, create the hosted zone and point the registrar's nameservers at it first.
- Terraform ≥ 1.10, AWS CLI v2.

## 1. Remote state bucket (once)

```sh
export AWS_PROFILE=<admin-profile> REGION=us-east-2
STATE_BUCKET="gils-garage-tfstate-$(aws sts get-caller-identity --query Account --output text)"

aws s3api create-bucket --bucket "$STATE_BUCKET" --region "$REGION" \
  --create-bucket-configuration LocationConstraint="$REGION"
aws s3api put-bucket-versioning --bucket "$STATE_BUCKET" --versioning-configuration Status=Enabled
aws s3api put-public-access-block --bucket "$STATE_BUCKET" \
  --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
```

State locking uses S3's native lockfile (`use_lockfile = true`), so no DynamoDB table.

## 2. Apply the infrastructure

```sh
cd infra/terraform
cp backend.hcl.example backend.hcl            # set bucket = $STATE_BUCKET
cp terraform.tfvars.example terraform.tfvars  # set domain_name
terraform init -backend-config=backend.hcl
terraform plan -out tfplan
terraform apply tfplan
terraform output
```

If the account already has a GitHub OIDC provider (`token.actions.githubusercontent.com`), set `create_oidc_provider = false`.

The first apply takes ~5–10 minutes (certificate validation + CloudFront propagation).

## 3. Configure the GitHub environment

Repo → **Settings → Environments → New environment → `production`**. Optional but recommended: *Deployment branches → Selected branches → `main`*, and yourself as a required reviewer.

Add **environment variables** (not secrets — none of these are sensitive):

| Variable | Value |
|---|---|
| `AWS_DEPLOY_ROLE_ARN` | `terraform output -raw deploy_role_arn` |
| `AWS_REGION` | e.g. `us-east-2` |
| `SITE_BUCKET` | `terraform output -raw site_bucket` |
| `CF_DISTRIBUTION_ID` | `terraform output -raw cloudfront_distribution_id` |
| `SITE_URL` | `terraform output -raw site_url` |

Also set `base_url` in `site.toml` to your real domain.

## 4. Ship

Push to `main` (or run **Actions → Deploy → Run workflow**). The job tests, builds, assumes the role via OIDC, syncs to S3, and invalidates CloudFront.

## Repo settings worth turning on

- Branch protection / ruleset on `main`: require PRs and the `CI` checks (`Go build & test`, `golangci-lint`, `Terraform fmt & validate`).
- Settings → Actions → General: *Workflow permissions: read repository contents*; require approval for fork PRs.
- Dependabot alerts + security updates.
- Secret scanning / push protection and CodeQL need GitHub Advanced Security on private repos (free if the repo goes public).

## Rollback

The site bucket is versioned (noncurrent versions kept 30 days). Fastest path: revert the commit on `main` and let Deploy run again.
