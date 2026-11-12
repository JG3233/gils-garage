+++
title = "How this site is built"
date = 2026-11-08
summary = ""  # TODO: one line for the index page, meta description, and feed
tags = ["meta", "go", "aws", "ci-cd"]
draft = true
+++

*TL;DR, two sentences for skimmers: a small Go program turns Markdown into plain files, and GitHub deploys them to AWS without holding a single key.*

```
content/*.md ──► garage build ──► public/ ──► GitHub Actions (OIDC) ──► S3 ──► CloudFront ──► you
```

*Explain the approach: four pieces (build it, check it, ship it, serve it), each ending with its security angle.*

## Building it: a tiny generator in Go

- *Why build my own instead of using Hugo/Jekyll: the job is just "Markdown in, folder of HTML out," so owning it is cheap. No themes, plugins, or surprise upgrades.*
- *Why Go is worth learning:*
  - *A single binary, with templates baked in via `go:embed`.*
  - *A strong standard library; only two dependencies (goldmark for Markdown, toml for front matter).*
  - *Fast builds, plus built-in testing and formatting.*
  - *It's the language of cloud tooling (Kubernetes, Terraform, most cloud CLIs).*
- *What it does, in one paragraph: read posts → render Markdown → fill templates → write `public/`. Size: about 540 lines.*
- *Guardrails: the build fails loudly on bad front matter or duplicate slugs, and checks everything before it wipes the old output.*

> 🔒 **Security angle:** *Raw HTML in posts gets escaped, so the site never runs code it didn't ship. That's what lets the CSP stay strict later.*

## Checking it: a hardened CI pipeline

- *Every PR runs three jobs: Go (module verify, vet, race tests, a smoke build that includes drafts), golangci-lint (with gosec), and Terraform fmt/validate.*
- *Workflow hygiene:*
  - *read-only `permissions` by default*
  - *every action pinned to a full commit SHA, not a tag*
  - *`persist-credentials: false` on checkout*
  - *Dependabot keeps the pins fresh*
- *Fork PRs: `pull_request`, not `pull_request_target`, plus requiring approval for outside contributors.*

> 🔒 **Security angle:** *The CI is a supply-chain boundary. A tag can be moved to point at new code, but a SHA can't. The job token can read the repo but never write to it.*

## Shipping it: deploying with no keys

```
GitHub Actions job (environment: production)
   └─ OIDC token  sub=repo:JG3233@47641679/gils-garage@1426422781:environment:production
        └─ sts:AssumeRoleWithWebIdentity → gils-garage-deploy (1h session)
             ├─ s3:ListBucket, PutObject, DeleteObject  (site bucket only)
             └─ cloudfront:CreateInvalidation           (this distribution only)
```

- *The usual way: store an AWS access key in GitHub secrets. Why that's risky (long-lived, can leak, needs rotating).*
- *How OIDC works, in plain terms: GitHub vouches for the job, AWS checks the claim, and you get hour-long credentials.*
- *The trust policy is the whole boundary. Its `sub`:*
  - *pins the `production` environment (so PRs and forks can't deploy)*
  - *pins the repo by immutable ID (so a renamed account can't be hijacked)*
- *Least privilege: worst case if the job is compromised, a defaced blog.*

### The first deploy failed, and that was the system working

- *An `AccessDenied` error, CloudTrail showing the subject format mismatch, the fix, and the lesson: an exact-match policy fails loudly on day one instead of quietly later.*

> 🔒 **Security angle:** *TODO: nothing long-lived to steal; the whole boundary is one readable trust policy.*

## Serving it: why CloudFront fits a static site

```
viewer ──HTTPS──► CloudFront ──OAC──► S3 (private)
                    ├─ function: www → apex, /foo/ → /foo/index.html
                    └─ security headers on every response
```

- *Static files need no servers, no runtime, nothing to patch. The attack surface is a bucket and a CDN.*
- *Private bucket: S3 is never public. CloudFront is the only way in (Origin Access Control).*
- *A tiny CloudFront Function handles pretty URLs and the www redirect.*
- *Security headers set at the edge (HSTS, a strict CSP). One sentence, plus how the escaped HTML lets the CSP stay strict.*
- *Caching: HTML always revalidates, assets cache for a day, and each deploy clears the cache.*

> 🔒 **Security angle:** *TODO: no servers to patch; the bucket can't be reached except through this one distribution.*

### What it costs

| Item | Monthly cost |
|---|---|
| S3 storage | *TODO* |
| CloudFront (free tier) | *TODO* |
| CloudFront Functions | *TODO* |
| Route 53 hosted zone | *TODO* |
| ACM certificate | *TODO* |
| Domain | *TODO (yearly)* |

*Check current AWS pricing and your own bill before filling this in.*

## Why it holds together

- *Escaped HTML → strict CSP*
- *No keys → nothing to leak → the trust policy is the whole story*
- *Static files + private bucket → almost nothing to attack, and almost nothing to pay*

*What I skipped, and future posts: how the build works inside, applying Terraform by hand, the `make verify` script.*

*Sign-off and a link to the source.*
