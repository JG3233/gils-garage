#!/usr/bin/env bash
# ok/bad/meh always return 0, so `cond && ok .. || bad ..` is safe in this script.
# shellcheck disable=SC2015
# Verify the production deploy wiring end to end:
#   GitHub environment + variables  <->  Terraform outputs  <->  live AWS resources  <->  the public site.
#
# Read-only: it changes nothing in GitHub or AWS.
# Requires: gh (authenticated), aws (credentials for the site account), terraform (backend initialised
# in infra/terraform), jq, dig, curl.
#
# Usage: scripts/verify-deploy-setup.sh            (from the repo root)
#        REPO=owner/name ENV_NAME=production scripts/verify-deploy-setup.sh
set -uo pipefail

REPO="${REPO:-JG3233/gils-garage}"
ENV_NAME="${ENV_NAME:-production}"
TF_DIR="${TF_DIR:-infra/terraform}"

pass=0 fail=0 warn=0
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; pass=$((pass + 1)); }
bad()  { printf '  \033[31m✗\033[0m %s\n' "$1"; fail=$((fail + 1)); }
meh()  { printf '  \033[33m!\033[0m %s\n' "$1"; warn=$((warn + 1)); }
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

for bin in gh aws terraform jq dig curl; do
  command -v "$bin" >/dev/null || { echo "missing required tool: $bin" >&2; exit 2; }
done

# ---------------------------------------------------------------------------
section "Terraform outputs ($TF_DIR)"
tf() { terraform -chdir="$TF_DIR" output -raw "$1" 2>/dev/null; }
ROLE_ARN=$(tf deploy_role_arn); BUCKET=$(tf site_bucket)
DIST_ID=$(tf cloudfront_distribution_id); SITE_URL=$(tf site_url)
if [[ -z "$ROLE_ARN" || -z "$BUCKET" || -z "$DIST_ID" || -z "$SITE_URL" ]]; then
  bad "could not read outputs - run 'terraform init -backend-config=backend.hcl' in $TF_DIR (and check AWS_PROFILE)"
  exit 1
fi
ok "deploy_role_arn = $ROLE_ARN"
ok "site_bucket = $BUCKET"
ok "cloudfront_distribution_id = $DIST_ID"
ok "site_url = $SITE_URL"
DOMAIN="${SITE_URL#https://}"
REGION=$(aws s3api get-bucket-location --bucket "$BUCKET" --query LocationConstraint --output text 2>/dev/null)
[[ "$REGION" == "None" || -z "$REGION" ]] && REGION="us-east-1"

# ---------------------------------------------------------------------------
section "GitHub environment '$ENV_NAME' ($REPO)"
env_json=$(gh api "repos/$REPO/environments/$ENV_NAME" 2>/dev/null) || {
  bad "environment '$ENV_NAME' not found (name must match exactly - the OIDC trust policy depends on it)"
  env_json=""
}
if [[ -n "$env_json" ]]; then
  ok "environment exists"

  if [[ $(jq -r '.deployment_branch_policy.custom_branch_policies // false' <<<"$env_json") == "true" ]]; then
    branches=$(gh api "repos/$REPO/environments/$ENV_NAME/deployment-branch-policies" \
      --jq '[.branch_policies[] | "\(.type // "branch"):\(.name)"] | join(", ")')
    if [[ "$branches" == "branch:main" ]]; then
      ok "deployments restricted to branch main"
    else
      meh "deployment branch rules are '$branches' (expected only branch:main)"
    fi
  elif [[ $(jq -r '.deployment_branch_policy.protected_branches // false' <<<"$env_json") == "true" ]]; then
    meh "deployments allowed from any protected branch (consider restricting to main only)"
  else
    bad "no deployment branch restriction - any branch can deploy with the AWS role"
  fi

  reviewers=$(jq -r '[.protection_rules[]? | select(.type=="required_reviewers") | .reviewers[].reviewer | (.login // .slug)] | join(", ")' <<<"$env_json")
  if [[ -n "$reviewers" ]]; then
    ok "required reviewers: $reviewers"
  else
    meh "no required reviewers (needs a public repo or a paid plan for private repos)"
  fi
fi

check_var() {
  local name=$1 want=$2 got
  got=$(gh api "repos/$REPO/environments/$ENV_NAME/variables/$name" --jq .value 2>/dev/null)
  if [[ -z "$got" ]]; then
    if gh api "repos/$REPO/environments/$ENV_NAME/secrets/$name" >/dev/null 2>&1; then
      bad "$name is a *secret*, but the workflow reads vars.$name - recreate it as a variable"
    else
      bad "$name is not set"
    fi
  elif [[ "$got" == "$want" ]]; then
    ok "$name matches"
  else
    bad "$name = '$got' (expected '$want')"
  fi
}
if [[ -n "$env_json" ]]; then
  check_var AWS_DEPLOY_ROLE_ARN "$ROLE_ARN"
  check_var AWS_REGION          "$REGION"
  check_var SITE_BUCKET         "$BUCKET"
  check_var CF_DISTRIBUTION_ID  "$DIST_ID"
  check_var SITE_URL            "$SITE_URL"
fi

# ---------------------------------------------------------------------------
section "AWS"
role_name="${ROLE_ARN##*/}"
trust=$(aws iam get-role --role-name "$role_name" --query Role.AssumeRolePolicyDocument --output json 2>/dev/null)
if [[ -n "$trust" ]]; then
  sub=$(jq -r '[.Statement[].Condition.StringEquals["token.actions.githubusercontent.com:sub"]] | flatten | join(",")' <<<"$trust")
  want_sub="repo:$REPO:environment:$ENV_NAME"
  # GitHub's sub claim is case-insensitive on owner/repo; compare lowercased.
  if [[ "$(tr '[:upper:]' '[:lower:]' <<<"$sub")" == "$(tr '[:upper:]' '[:lower:]' <<<"$want_sub")" ]]; then
    ok "role trust policy only accepts $sub"
  else
    bad "role trust sub is '$sub' (expected '$want_sub')"
  fi
  provider=$(jq -r '.Statement[0].Principal.Federated' <<<"$trust")
  if aws iam get-open-id-connect-provider --open-id-connect-provider-arn "$provider" >/dev/null 2>&1; then
    ok "GitHub OIDC provider exists"
  else
    bad "OIDC provider $provider not found"
  fi
else
  bad "cannot read role $role_name"
fi

pab=$(aws s3api get-public-access-block --bucket "$BUCKET" --query PublicAccessBlockConfiguration --output json 2>/dev/null)
if [[ -n "$pab" && $(jq '[.[]] | all' <<<"$pab") == "true" ]]; then
  ok "bucket public access fully blocked"
else
  bad "bucket public access block is not fully enabled"
fi

dist=$(aws cloudfront get-distribution --id "$DIST_ID" --output json 2>/dev/null)
if [[ -n "$dist" ]]; then
  status=$(jq -r .Distribution.Status <<<"$dist")
  [[ "$status" == "Deployed" ]] && ok "CloudFront distribution Deployed" || meh "CloudFront status is $status (still propagating?)"
  aliases=$(jq -r '.Distribution.DistributionConfig.Aliases.Items | join(", ")' <<<"$dist")
  if jq -e --arg d "$DOMAIN" '.Distribution.DistributionConfig.Aliases.Items | index($d)' <<<"$dist" >/dev/null; then
    ok "aliases: $aliases"
  else
    bad "aliases '$aliases' missing $DOMAIN"
  fi
  cert_arn=$(jq -r .Distribution.DistributionConfig.ViewerCertificate.ACMCertificateArn <<<"$dist")
  cert_status=$(aws acm describe-certificate --region us-east-1 --certificate-arn "$cert_arn" \
    --query Certificate.Status --output text 2>/dev/null)
  [[ "$cert_status" == "ISSUED" ]] && ok "ACM certificate ISSUED" || bad "ACM certificate status: ${cert_status:-unknown}"
else
  bad "cannot read CloudFront distribution $DIST_ID"
fi

# ---------------------------------------------------------------------------
section "Public site ($SITE_URL)"
a_records=$(dig +short "$DOMAIN" A @1.1.1.1 | tr '\n' ' ')
[[ -n "$a_records" ]] && ok "DNS resolves: $a_records" || bad "$DOMAIN does not resolve yet"

headers=$(curl -sS -o /dev/null -D - --max-time 15 "$SITE_URL/" 2>&1 | tr -d '\r')
code=$(awk 'toupper($1) ~ /^HTTP/ {c=$2} END {print c}' <<<"$headers")
case "$code" in
  200)     ok "GET / -> 200" ;;
  403|404) meh "GET / -> $code (expected until the first deploy uploads the site)" ;;
  "")      bad "no HTTPS response: $(head -1 <<<"$headers")" ;;
  *)       bad "GET / -> $code" ;;
esac
if [[ -n "$code" ]]; then
  grep -qi '^strict-transport-security:.*preload' <<<"$headers" && ok "HSTS header present" || bad "HSTS header missing"
  grep -qi '^content-security-policy:' <<<"$headers" && ok "CSP header present" || bad "CSP header missing"
fi
www=$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 15 "https://www.$DOMAIN/" 2>/dev/null)
[[ "$www" == "301 $SITE_URL/" ]] && ok "www redirects to apex" || meh "www -> '$www' (expected '301 $SITE_URL/')"

# ---------------------------------------------------------------------------
printf '\n\033[1m%d passed, %d warnings, %d failed\033[0m\n' "$pass" "$warn" "$fail"
((fail == 0))
