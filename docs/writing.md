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

Standard Markdown, plus a few extras.

### What works

| Feature | Write | Notes |
|---|---|---|
| Headings, bold, italics, lists, links, quotes | the usual | Standard Markdown (CommonMark) |
| Code | ```` ```go ```` fenced blocks or `` `inline` `` | Shown as plain monospace, **no syntax colors** |
| Tables | `\| a \| b \|` with a `\|---\|---\|` row | |
| Strikethrough | `~~gone~~` | |
| Task lists | `- [ ] todo` / `- [x] done` | |
| Bare links | `https://example.com` | Becomes a link automatically |
| Footnotes | `text[^1]` … `[^1]: the note` | |
| Smart punctuation | `"quotes"`, `--`, `...` | Become curly quotes, –, and … |
| Heading links | `## Setup` | Gets `id="setup"`, so `/posts/x/#setup` links to it |
| Images | `![alt text](/img/name.png)` | Put the file in `static/img/` |

### What doesn't work (on purpose or not yet)

- **Raw HTML** is shown as text, not rendered. `<script>` comes out as `&lt;script&gt;`. This is a security choice.
- **No syntax highlighting, emoji shortcodes (`:rocket:`), Mermaid diagrams, or `> [!NOTE]` callouts.** These work on github.com but not here.
- **Images from other sites** work (any `https://` URL), but local files in `static/img/` are preferred.

### Under the hood

The engine is [goldmark](https://github.com/yuin/goldmark), a Go Markdown library, configured in [`internal/site/markdown.go`](../internal/site/markdown.go) with three extensions:

- **GFM**: tables, strikethrough, bare links, task lists (GitHub Flavored Markdown's additions)
- **Footnote**
- **Typographer**: smart punctuation

Plus automatic heading IDs.

To add a feature (syntax highlighting, callouts, lazy-loaded images…), add a goldmark extension or write a small custom one in that file. Nothing else imports goldmark. Two rules:

1. Never enable `html.WithUnsafe()`. Posts are trusted as safe HTML only because goldmark escapes raw HTML.
2. Anything that adds styling must use CSS classes in `static/style.css`. The site's CSP blocks inline `style=""` attributes.

## Publishing

Set `draft = false`, open a PR, merge to `main`. Deploy runs automatically.
