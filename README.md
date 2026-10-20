# Gil's Garage

A self-hosted blog about cloud security, CI/CD, and AI agents — and the static site generator that builds it, written in Go.

```
content/*.md ──► garage build ──► public/ ──► GitHub Actions (OIDC) ──► S3 ──► CloudFront ──► you
```

## Quick start

```sh
make serve                         # build with drafts, preview at http://127.0.0.1:8080
make new TITLE="My next post"      # scaffold content/posts/YYYY-MM-DD-my-next-post.md (draft)
make check                         # tests + lint + terraform validate (what CI runs)
```

Requires Go 1.24+. `make lint` needs [golangci-lint v2](https://golangci-lint.run); `make tf-validate` needs Terraform ≥ 1.10.

## Layout

| Path | What |
|---|---|
| `cmd/garage/` | CLI: `build`, `serve`, `new` |
| `internal/site/` | Engine: post parsing, Markdown, templates (embedded), Atom feed |
| `content/posts/` | Posts — Markdown with TOML front matter |
| `static/` | Copied verbatim to the site root (CSS, images, favicon) |
| `site.toml` | Site title, base URL, author |
| `infra/terraform/` | S3 + CloudFront + ACM + Route 53 + GitHub OIDC deploy role |
| `.github/workflows/` | `ci.yml` (every PR/push), `deploy.yml` (main → production) |
| `docs/` | [Architecture](docs/architecture.md) · [Deployment](docs/deployment.md) · [Writing](docs/writing.md) |

## Security posture

- **No AWS keys in GitHub.** Deploys assume an IAM role via OIDC; the trust policy accepts only `repo:JG3233/gils-garage:environment:production`.
- **Least-privilege deploy role:** `s3:ListBucket`/`PutObject`/`DeleteObject` on one bucket, `cloudfront:CreateInvalidation` on one distribution.
- **Private origin:** S3 public access fully blocked; CloudFront reads via Origin Access Control; TLS-only bucket policy.
- **Security headers** at the edge: HSTS (preload), strict CSP, `nosniff`, `frame-ancestors 'none'`, Permissions-Policy.
- **Supply chain:** every third-party action pinned to a commit SHA; Dependabot for Go, Actions and Terraform; `go mod verify` in CI; checkout with `persist-credentials: false`.
- **Safe rendering:** raw HTML in Markdown is escaped; static files copied through `os.Root` (no symlink escape).
