# Writing posts

```sh
make new TITLE="Scoping OIDC trust policies"
# -> content/posts/2026-10-19-scoping-oidc-trust-policies.md
make serve   # preview with drafts at http://127.0.0.1:8080 (re-run to rebuild)
```

## Front matter

TOML between `+++` lines:

```toml
+++
title = "Scoping OIDC trust policies"
date = 2026-10-19
summary = "One line for the index page, meta description, and feed."
tags = ["aws", "github-actions"]
draft = true
# slug = "custom-slug"   # default: filename minus the date prefix
+++
```

| Field | Required | Notes |
|---|---|---|
| `title` | yes | |
| `date` | yes | `YYYY-MM-DD` or full RFC 3339 timestamp |
| `summary` | no | Shown on the index and in `<meta name="description">` |
| `tags` | no | Slugified: `"Cloud Security"` → `/tags/cloud-security/` |
| `draft` | no | Drafts are excluded from production builds |
| `slug` | no | URL becomes `/posts/<slug>/` |

## Body

GitHub-flavored Markdown: tables, task lists, strikethrough, autolinks, footnotes, fenced code. Raw HTML is escaped on purpose; if a post genuinely needs it, that's a deliberate engine change.

Images: put them in `static/img/` and reference as `/img/name.png`.

## Publishing

Set `draft = false`, open a PR, merge to `main`. Deploy runs automatically.
