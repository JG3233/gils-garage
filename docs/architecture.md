# Architecture

## Build

`garage build` is a single pass:

1. Load `site.toml` (title, `base_url`, author).
2. Parse every `content/posts/*.md`: TOML front matter between `+++` lines, Markdown body rendered by [goldmark](https://github.com/yuin/goldmark) (GFM, footnotes, typographer, heading IDs). Raw HTML is **not** passed through.
3. Drop drafts (unless `-drafts`), reject duplicate slugs, sort newest first.
4. Render with `html/template` (templates embedded in the binary from `internal/site/templates/`):
   - `/index.html`, `/posts/<slug>/index.html`, `/tags/<tag>/index.html`, `/404.html`
   - `/feed.xml` — Atom, latest 20 posts, full content
5. Copy `static/` into the output root. A static file that collides with a generated page is an error.

The output directory is wiped first; the CLI refuses to wipe `/`, `$HOME`, or the working directory.

## Serving

```
viewer ──HTTPS──► CloudFront ──OAC (SigV4)──► S3 (private, versioned, SSE)
                    │
                    ├─ viewer-request function: www → apex 301, /foo/ → /foo/index.html
                    ├─ response headers policy: HSTS, CSP, nosniff, DENY framing, referrer, permissions
                    └─ 403/404 from origin → /404.html (404)
```

- ACM certificate in `us-east-1` (CloudFront requirement), DNS-validated through Route 53.
- `PriceClass_100` (NA + EU edges), HTTP/2 + HTTP/3, IPv6.
- Cache: `Managed-CachingOptimized`. HTML/feed are uploaded with `max-age=0, must-revalidate`; everything else `max-age=86400`. Each deploy invalidates `/*` (free tier covers 1,000 paths/month; `/*` counts as one).

## Deploy identity

```
GitHub Actions job (environment: production)
   └─ OIDC token  sub=repo:JG3233@47641679/gils-garage@1426422781:environment:production  aud=sts.amazonaws.com
        └─ sts:AssumeRoleWithWebIdentity → gils-garage-deploy (1h session)
             ├─ s3:ListBucket, PutObject, DeleteObject  (site bucket only)
             └─ cloudfront:CreateInvalidation           (this distribution only)
```

Pinning `sub` to the environment means a PR, a fork, or a workflow that doesn't declare `environment: production` can't get credentials. Add required reviewers or a branch rule on the environment to gate deploys further.

Terraform is applied by hand from a workstation for now (see [deployment.md](deployment.md)). A natural next step is a separate read-only `plan` role for PRs and an `apply` role bound to a second protected environment.
