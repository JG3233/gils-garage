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

Then run `make verify` from the repo root. It is read-only and checks that the environment, its branch rule and reviewers, and every variable match the Terraform outputs, and that the role trust policy, certificate, CloudFront distribution, DNS and security headers are live.

## 4. Ship

Push to `main` (or run **Actions → Deploy → Run workflow**). The job tests, builds, assumes the role via OIDC, syncs to S3, and invalidates CloudFront. The next section explains each step.

## How CI/CD works

Two GitHub Actions workflows do everything. **CI** checks code. **Deploy** publishes the site.

```
open/update PR ──► CI (3 checks) ──► merge to main ──► CI again
                                                  └──► Deploy ──► OIDC ──► S3 ──► CloudFront
```

### GitHub Actions in 60 seconds

| Term | Meaning | In this repo |
|---|---|---|
| **Workflow** | A YAML file in `.github/workflows/` | `ci.yml`, `deploy.yml` |
| **Trigger** (`on:`) | The event that starts a workflow | PR, push to `main`, manual button |
| **Job** | A group of steps on a fresh VM. Jobs run in parallel unless told otherwise | CI has 3; Deploy has 1 |
| **Step** | A shell command (`run:`) or a reusable action (`uses:`) | `go test`, `actions/checkout` |
| **Environment** | A named deploy target with its own variables, branch rules and reviewers | `production` |
| **`vars.*` / `secrets.*`** | Environment or repo settings injected into a workflow | Only `vars.*`; nothing here is secret |
| **`GITHUB_TOKEN`** | A short-lived token every run gets automatically | Read-only (`permissions: contents: read`) |

### CI: `.github/workflows/ci.yml`

**Runs on:** every pull request, and every push to `main`.

Three jobs run in parallel. Their names are the required checks for branch protection:

| Job | Steps | Catches |
|---|---|---|
| `Go build & test` | `go mod verify` → `go vet` → `go test -race -cover` → `garage build -drafts` | Tampered deps, bugs, failing tests, a post (draft or not) that breaks the build |
| `golangci-lint` | golangci-lint `v2.15.0` (gosec, errorlint, revive, gofmt) | Security smells, bad error handling, formatting |
| `Terraform fmt & validate` | `fmt -check` → `init -backend=false` → `validate` | Unformatted or invalid Terraform. Needs no AWS credentials |

A new push to the same branch cancels the older run (`cancel-in-progress: true`).

### Deploy: `.github/workflows/deploy.yml`

**Runs on:** a push to `main` that touches `content/`, `static/`, `cmd/`, `internal/`, `site.toml`, `go.mod`, `go.sum` or the workflow itself. Manual runs work too (**Actions → Deploy → Run workflow**).

**Doesn't run on:** changes only to `docs/`, `infra/`, the Makefile or `ci.yml`. Infrastructure changes are applied by hand with `terraform apply` (step 2 above).

| # | Step | What happens |
|---|---|---|
| 1 | Checkout, set up Go | Same pinned actions as CI |
| 2 | `go test ./...` | Deploy doesn't wait for CI, so it re-tests. A broken engine never ships |
| 3 | `garage build -out public` | Production build, **no drafts** |
| 4 | Configure AWS credentials | The OIDC handshake below. Ends with 1-hour AWS credentials in the job |
| 5 | Sync to S3 (two passes) | Assets: `max-age=86400` (1 day). HTML and `feed.xml`: `max-age=0, must-revalidate`. `--delete` removes stale files; `--no-follow-symlinks` never uploads a link's target |
| 6 | Invalidate CloudFront | `/*` clears the edge cache so readers see the new version right away |

Deploys **queue** instead of cancelling (`cancel-in-progress: false`), because stopping halfway through a sync would leave the bucket half old, half new. If the environment has a required reviewer, the run waits for your approval before step 1.

### The OIDC handshake (no AWS keys stored anywhere)

1. The job declares `environment: production` and `permissions: id-token: write`.
2. `configure-aws-credentials` asks GitHub for a signed OIDC token. Its `sub` claim is `repo:JG3233@47641679/gils-garage@1426422781:environment:production`.
3. The action calls `sts:AssumeRoleWithWebIdentity` with that token and `vars.AWS_DEPLOY_ROLE_ARN`.
4. AWS checks the role's **trust policy** (`infra/terraform/github_oidc.tf`): the issuer is GitHub, `aud` is `sts.amazonaws.com`, and `sub` matches exactly.
5. AWS returns temporary credentials. The role can only list, put and delete objects in the site bucket and invalidate the distribution.

A PR, a fork, or any workflow without `environment: production` gets a different `sub`, so AWS refuses it.

### Supply-chain hardening

| Setting | Why |
|---|---|
| Every `uses:` pinned to a 40-character commit SHA | Tags can be moved to point at malicious code; SHAs can't |
| Dependabot (weekly, grouped) for Go, Actions and Terraform | Keeps the pins current without manual SHA hunting |
| `permissions: contents: read` at the top | The automatic token can't push, comment or release |
| `persist-credentials: false` on checkout | The token isn't left in `.git/config` for later steps to read |
| `pull_request`, never `pull_request_target` | Fork PRs never run with this repo's secrets |
| `timeout-minutes` on every job | A hung job can't burn minutes for hours |
| Exact tool versions (golangci-lint, Terraform) | A new release can't break CI overnight |

### When a deploy fails

| Symptom | Likely cause |
|---|---|
| `Not authorized to perform sts:AssumeRoleWithWebIdentity` | The `sub` doesn't match the trust policy: `environment:` removed from the job, environment renamed, or repo moved. Run `make verify` |
| `Credentials could not be loaded` / no OIDC token | `id-token: write` missing from the job's permissions |
| `AccessDenied` on `s3 sync` or invalidation | `SITE_BUCKET` or `CF_DISTRIBUTION_ID` doesn't match the Terraform outputs. `make verify` checks this |
| Run sits at "Waiting" | The environment's required reviewer hasn't approved it |
| Merged, but no Deploy run | The change only touched paths outside the trigger list. Use **Run workflow** to force one |
| Deployed, but you still see the old page | Browser cache. HTML revalidates, but CSS and images are cached for a day; hard-refresh |

## Repo settings worth turning on

- Branch protection / ruleset on `main`: require PRs and the `CI` checks (`Go build & test`, `golangci-lint`, `Terraform fmt & validate`).
- Settings → Actions → General: *Workflow permissions: read repository contents*; require approval for fork PRs.
- Dependabot alerts + security updates.
- Secret scanning / push protection and CodeQL need GitHub Advanced Security on private repos (free if the repo goes public).

## Rollback

The site bucket is versioned (noncurrent versions kept 30 days). Fastest path: revert the commit on `main` and let Deploy run again.
