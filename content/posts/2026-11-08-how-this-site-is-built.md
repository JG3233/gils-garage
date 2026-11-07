+++
title = "How this site is built"
date = 2026-11-08
summary = "Following one post from a Markdown file to your browser: a small Go generator, a private S3 bucket behind CloudFront, and a deploy that holds no AWS keys."
tags = ["meta", "go", "aws", "ci-cd"]
draft = true
+++

The best way I know to explain a system is to push one request through it and watch what touches it. So instead of a component list, here's the life of this post: from an empty file on my laptop to the bytes your browser just rendered.

```
content/*.md ──► garage build ──► public/ ──► GitHub Actions (OIDC) ──► S3 ──► CloudFront ──► you
```

Every arrow in that line is a decision. Most of them are the boring, strict option — and the interesting part is how the strict options end up holding each other up.

## 1. A file

It starts with:

```sh
make new TITLE="How this site is built"
# content/posts/2026-11-08-how-this-site-is-built.md
```

That writes a skeleton with TOML front matter between `+++` lines and `draft = true`:

```toml
+++
title = "How this site is built"
date = 2026-11-08
summary = ""
tags = []
draft = true
+++
```

The filename carries the date so posts sort chronologically on disk. The URL doesn't: the slug defaults to the filename minus its `YYYY-MM-DD-` prefix, so this post lives at `/posts/how-this-site-is-built/`. Tags and slugs both pass through the same `Slugify` — lowercase, runs of anything non-alphanumeric collapsed to one hyphen — so `"Cloud Security"` and `"cloud-security"` are the same tag by construction, not by discipline.

`make serve` builds with drafts included and serves `public/` on localhost. There's no file watcher and no live reload; I re-run it. A build takes single-digit milliseconds, so the watcher would be more code than the time it saves, and this way the preview is exactly the artifact that ships.

## 2. The generator

`garage` is a static site generator I wrote for this site: about 540 lines of Go plus 60 lines of templates, with two dependencies — `BurntSushi/toml` and `yuin/goldmark`.

I didn't write it because existing generators are bad. I wrote it because the contract of a static site generator is tiny — *produce a directory of files* — and when the contract is that small, owning the implementation is cheap. No theme system, no plugin API, no upgrade that silently changes my URL structure. Templates are compiled into the binary with `go:embed`, so `go run ./cmd/garage build` is the whole toolchain.

The code splits along one line: `cmd/garage` parses flags and prints; `internal/site` makes every decision. That's what lets the tests drive the real engine instead of a mock of it.

### Parsing: fail loudly, fail early

`splitFrontMatter` normalizes line endings, insists the file *starts* with `+++`, and finds the closing delimiter. If there isn't one, that's an error — not a post that quietly renders its own metadata as the first paragraph.

`ParsePost` then requires a `title` and a `date`. Missing either fails the whole build with the filename in the message.

`LoadPosts` reads every post, drops drafts unless asked, and **rejects duplicate slugs**, naming both files. This one earns its place: two posts resolving to the same URL means one silently overwrites the other and the build still exits zero. That's data loss with a green checkmark. Making it impossible costs a map and four lines.

### Markdown: the setting that isn't there

```go
goldmark.New(
    goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.Typographer),
    goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)
```

GFM for tables, task lists, strikethrough and autolinks; footnotes and smart quotes because I write prose; heading IDs so sections are linkable.

The important line is the one I didn't write: `html.WithUnsafe()`. Without it, goldmark **escapes raw HTML** in Markdown instead of passing it through. That guarantee is what makes the next step safe — the rendered body gets wrapped in `template.HTML` so `html/template` won't double-escape it. Wrapping untrusted strings in `template.HTML` is normally exactly how XSS happens; here it's sound only because goldmark already removed the possibility. Both spots carry a `#nosec` comment that says *why*, so the linter suppression documents the invariant instead of hiding it.

The cost: no inline `<iframe>`, no hand-written `<figure>`. If a post ever really needs raw HTML, that's an engine change with a test, not a flag.

### Rendering: everything that can fail, fails first

`Build` is one pass with no incremental state, and its order is the design:

1. Load and validate `site.toml`.
2. Parse every post.
3. Parse every template.
4. **Then** wipe the output directory.
5. Render the index, each post, each tag page, and the 404.
6. Write the Atom feed.
7. Copy `static/` over the top.

Bad front matter, a duplicate slug, a template syntax error — all of it surfaces before step 4. A failed build leaves the last good `public/` untouched.

Every page receives the same `pageData` struct — site config, title, post list, optional single post, tag, tag counts, year — and each template uses what it needs. `base.html` owns the document shell and a `{{block "content" .}}`; the four page templates fill it. Tags are built by inverting posts into a map and then *sorting* the tag list, because Go randomizes map iteration and a static site shouldn't produce a differently ordered page on every build.

The feed is hand-rolled Atom via `encoding/xml` struct tags: newest 20 posts, full content. Its `updated` comes from the newest post, falling back to the Unix epoch on an empty site — never `time.Now()`. A feed that claims to change on every build teaches readers' aggregators to ignore it.

### Two guards around the filesystem

The build `os.RemoveAll`s its output directory, so `safeOutDir` refuses `/`, `$HOME`, and the current directory. `-out .` versus `-out ./public` is a one-keystroke typo, and that typo should not be able to delete my home directory.

Static assets are copied like this:

```go
root, err := os.OpenRoot(src)
return os.CopyFS(dst, root.FS())
```

`os.OpenRoot` (Go 1.24) confines traversal to `static/`, so a symlink in there can't drag `~/.aws/credentials` into a public bucket. And `os.CopyFS` refuses to overwrite, so a static file that collides with a generated page fails the build instead of winning silently. Two hazards closed by choosing the right stdlib call, zero lines of custom checking.

## 3. A pull request

I flip `draft = false`, push a branch, open a PR. CI runs three jobs in parallel:

- **Go**: `go mod verify`, `go vet`, `go test -race -cover`, then `garage build -drafts` as a smoke test. The tests cover slugs, front matter, the duplicate-slug rule, the output-dir guard, and a full build asserting on the generated tree. The smoke build catches template breakage, and `-drafts` means unpublished posts have to compile too.
- **golangci-lint**, pinned to an exact version.
- **Terraform**: `fmt -check`, then `init -backend=false` and `validate`. No credentials, so it's safe on any PR.

The workflow hygiene is the same everywhere:

- Top-level `permissions: contents: read`. Nothing gets write access by default.
- Every third-party action pinned to a **full commit SHA**, version in a comment. A tag like `v7.0.1` is a pointer its owner can move; a SHA is the code.
- `persist-credentials: false` on checkout, so the job token isn't left in `.git/config` for later steps to find.
- Dependabot for Go modules, Actions and Terraform, grouped weekly. That's what makes SHA pinning livable instead of a reason never to update.
- CI cancels superseded runs on the same ref.

## 4. The deploy, and the identity it borrows

Merging to `main` triggers `deploy.yml` — but only if the merge touched content, code, config or the workflow itself. A README edit doesn't wake up a CDN.

The deploy job rebuilds without drafts, then needs to write to AWS. **There are no AWS keys in this repository.** Not in secrets, not anywhere. Instead:

```
GitHub Actions job (environment: production)
   └─ OIDC token  sub=repo:JG3233@47641679/gils-garage@1426422781:environment:production
        └─ sts:AssumeRoleWithWebIdentity → gils-garage-deploy (1h session)
             ├─ s3:ListBucket, PutObject, DeleteObject  (site bucket only)
             └─ cloudfront:CreateInvalidation           (this distribution only)
```

The job asks GitHub for a signed OIDC token (`id-token: write`, granted on this one job only), hands it to STS, and gets back credentials that expire in an hour. Nothing long-lived exists to rotate or leak.

All of the security lives in the role's trust policy, which asks for three things: the token came from GitHub's OIDC provider, its audience is `sts.amazonaws.com`, and its subject is **exactly** that `sub` string — `StringEquals`, no wildcards.

That subject does two jobs.

**It pins the environment.** `:environment:production` only appears in tokens from jobs that declare `environment: production`. A pull request can't produce it. Neither can a fork, a feature branch, or a new workflow someone sneaks in. That makes the `environment:` line in `deploy.yml` load-bearing, and the workflow has a comment saying so — remove it and deploys fail closed. The environment is also where GitHub hangs branch restrictions and required reviewers, so "who can deploy" is a repo setting, not an IAM change.

**It pins the repo by ID, not name.** `JG3233@47641679/gils-garage@1426422781` is GitHub's immutable subject format: owner and repo identified by numeric IDs. Names can be given up and claimed by someone else; IDs can't. With a name-based subject, renaming my account would open a window in which whoever registers the old name could mint tokens my role accepts.

### The first deploy failed, and that was the system working

I didn't start with that format. The first trust policy matched `repo:JG3233/gils-garage:environment:production` — the shape every tutorial shows. The first real deploy got `AccessDenied` on `sts:AssumeRoleWithWebIdentity`.

CloudTrail showed why: this repo's tokens arrive in the immutable format, so the subject GitHub sent and the subject IAM expected never matched. IAM compares the string exactly and case-sensitively. Close doesn't count.

That's the behavior you want from a trust policy — a mismatch is a hard no, not a fallback — but it's an unpleasant one to debug if you don't know where to look. The fix was to stop hardcoding the shape: the Terraform variable is now `github_sub_prefix`, defaulting to the immutable prefix, with validation that rejects anything that isn't `repo:` followed by a colon-free prefix. And the description tells future-me where the truth lives:

```sh
gh api repos/JG3233/gils-garage/actions/oidc/customization/sub --jq .sub_claim_prefix
```

### What the role can actually do

Three statements: list the site bucket, put and delete objects in it, create invalidations on the distribution. If the deploy job were fully compromised, the worst outcome is a defaced blog. It can't read other buckets, touch IAM, or spin up compute.

The upload itself is two `aws s3 sync --delete` passes, split by how long things should be cached (more on that below), followed by a single `/*` invalidation. The deploy concurrency group does **not** cancel in-progress runs — two quick merges queue rather than racing each other into a half-synced bucket.

### Checking the wiring, not trusting the docs

`make verify` runs a read-only script that walks the whole chain and asserts it agrees with itself:

- Terraform outputs exist and can be read.
- The `production` environment exists, is restricted to `main`, and has reviewers.
- Each of the five environment variables matches its Terraform output — and if one was accidentally created as a *secret*, it says exactly that, since the workflow reads `vars.*` and would see nothing.
- The role's trust `sub` matches what GitHub will actually send, fetched live from the OIDC customization API rather than assumed. That check exists because of the failure above.
- The OIDC provider exists, the bucket's public access block is fully on, the distribution is `Deployed` with the right alias, and the certificate is `ISSUED`.
- DNS resolves, `GET /` is 200, HSTS and CSP headers are present, and `www` 301s to the apex.

Setup instructions drift. A script that checks them doesn't.

## 5. The edge

Now you request the page.

```
viewer ──HTTPS──► CloudFront ──OAC (SigV4)──► S3 (private, versioned, SSE)
                    │
                    ├─ viewer-request function: www → apex 301, /foo/ → /foo/index.html
                    ├─ response headers policy: HSTS, CSP, nosniff, DENY framing, referrer, permissions
                    └─ 403/404 from origin → /404.html (404)
```

DNS is Route 53 A and AAAA alias records for both the apex and `www`, pointing at CloudFront. TLS uses an ACM certificate in `us-east-1` — CloudFront won't accept one from any other region — DNS-validated by records in the same Terraform. Minimum TLS 1.2, HTTP/2 and HTTP/3, IPv6, `PriceClass_100` (North America and Europe edges, which is where my readers are).

### A rewrite function

A CloudFront Function runs on every viewer request:

- If the host is `www.gilsgarage.dev`, return a 301 to the apex with the path preserved. One canonical hostname, enforced before anything is fetched.
- If the URI ends in `/`, or its last segment has no file extension, append `index.html`. That's how `/posts/how-this-site-is-built/` finds `posts/how-this-site-is-built/index.html` — S3 behind CloudFront has no notion of directory indexes.

CloudFront Functions instead of Lambda@Edge because this is precisely their niche: sub-millisecond, no cold starts, no network or filesystem access. A URL rewriter shouldn't have either.

### A private origin

The S3 bucket isn't a website bucket and isn't public in any sense. All four public-access blocks are on. ACLs are disabled (`BucketOwnerEnforced`). Objects are encrypted, the bucket is versioned, and old versions expire after 30 days.

CloudFront reaches it through **Origin Access Control**, signing each request with SigV4. The bucket policy allows `s3:GetObject` from the CloudFront service principal *only* when `AWS:SourceArn` is this specific distribution — someone else's distribution aimed at my bucket gets nothing. A second statement denies every S3 action over plain HTTP.

One consequence worth knowing: because CloudFront has `GetObject` but not `ListBucket`, S3 answers **403**, not 404, for a missing key — it won't reveal whether the key exists. So the distribution maps *both* 403 and 404 to `/404.html` with a 404 status. Without the 403 mapping, every typo'd URL would show an XML "Access Denied."

### Headers on every response

A response headers policy, not `<meta>` tags, so it applies to every object — HTML, CSS, the feed, the 404:

- `Strict-Transport-Security`: two years, `includeSubDomains`, `preload`
- `Content-Security-Policy`: `default-src 'self'`, `script-src 'self'`, `style-src 'self'`, `object-src 'none'`, `base-uri 'self'`, `frame-ancestors 'none'`, `form-action 'self'` — no `unsafe-inline` anywhere
- `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy` turning off camera, microphone, geolocation and FLoC

That CSP has no exceptions, and it can afford none because of step 2: posts can't contain raw HTML, and styling is one static stylesheet. The engine's choice is what makes the edge's choice cheap.

### Caching

Nothing on this site has a content hash in its filename, so freshness is set at upload time:

| Object | `Cache-Control` |
|---|---|
| HTML and `feed.xml` | `public, max-age=0, must-revalidate` |
| Everything else | `public, max-age=86400` |

HTML always revalidates, so fixes appear immediately. CSS and images get a day. Each deploy then invalidates `/*`, which counts as one path against CloudFront's 1,000 free invalidations a month — cheaper and less fragile than computing exactly which paths changed.

## What I left manual

Terraform isn't applied by CI. I run `plan` and `apply` from my machine; state lives in a versioned, private S3 bucket using S3's native lock file (`use_lockfile = true`, Terraform ≥ 1.10), so there's no DynamoDB table to pay for.

Automating `apply` would mean GitHub can assume a role that manages IAM, S3, CloudFront and Route 53 — a far juicier target than a role that can write one bucket. Content changes weekly; infrastructure changes a few times a year. When I do automate it, it'll be the same pattern as the deploy, twice: a read-only `plan` role for PRs, and an `apply` role locked to a second protected environment.

## Why it holds together

Pulling the thread back out, each layer's strictness is what lets the next one stay strict:

- The generator escapes raw HTML → the CSP needs no inline exceptions.
- The bucket is private → missing keys come back as 403 → the 404 mapping covers both.
- The deploy holds no keys → there's nothing to rotate → the trust policy is the entire security boundary, small enough to read in one sitting.
- The trust policy compares exactly → a misconfiguration is a hard failure on day one, not a quiet hole later.
- `os.Root` and `os.CopyFS` confine the copy → a stray symlink isn't something I have to think about.

None of that is clever. It's mostly picking the stricter default and then arranging things so I never need an exception to it. Between deploys, this whole site is a few kilobytes in a bucket and a CDN that knows how to add a slash.

About 540 lines of Go, 460 of Terraform, 140 of workflow YAML. The source is at [github.com/JG3233/gils-garage](https://github.com/JG3233/gils-garage).
