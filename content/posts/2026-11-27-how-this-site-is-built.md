+++
title = "How this site is built (securely!)"
date = 2026-11-27
summary = "Designing this blog with security as a first-class citizen"
tags = ["meta", "go", "aws", "ci-cd", "github-actions", "github", "security"]
draft = false
+++

Welcome back to Gil's Garage! A blog site I'll be continuing to use to share my thoughts and sharpen my skills on security engineering.

The Garage is nothing transformational in terms of architecting a blog, but I have made it unique in my implementation. That means that while the site is just HTML generated from Markdown, sitting in an S3 bucket served by CloudFront, I took every opportunity to do 2 things:

- Add security hardening just about anywhere it was possible.
- Learn more about the `Go` programming language by writing my own static site generator.

Practically, this is best explored in 2 workstreams.
```
1. Dev POV: from push to deploy  
2. User POV: from request to page load <-- this is you!
```
Let's dig into the details on the architecture, with a few security chatterbox pitstops.

## Architecture

Gil's Garage is a common use case for a blog: I want to write a thing and host it on my site. A very simple way to do that is with a static site generator that takes my written markdown files and generates HTML files for display. The site doesn't need to do much else, just know which file the user is looking at, fetch it, and provide properly generated HTML. There are some easily resolvable questions about what domain to use and who is doing the fetching, but the mechanics are relatively standard.

From that point, there are a few ways to accomplish this goal. The simplest from a blogger perspective is to use 2 services like Hugo and Netlify. Hugo does the static site generation from the source markdown, and Netlify does the hosting of said files. Done! Much cool!

> Disclaimer: I didn't want to do this, so I didn't! Naturally, that means I may have some blind spots on how Hugo and Netlify work on the details.

The Garage does the same thing, but more so on my own. Instead of Hugo generating HTML, I wrote a Go program to do this for me. I will gladly opine about the benefits of Go at length later, but for now it's sufficient to say I wanted to learn about the mechanics of Go and continue building my knowledge of the language. This is for a similar reason to why my personal RSS feed - [Spindle](https://github.com/JG3233/spindle) - used Go and WASM. 

> My opinion on how to use AI tools to learn programming has changed drastically (as many opinions have) since building Spindle, though I'll save that thought for a later post. Suffice to say that laborious hand-coding is going to be on my calendar much more often. *Actual* growth and personal satisfaction will flourish as a result.

So now when I need to create the HTML file folder, I simply run `garage build` instead of using Hugo. This invokes the Go binary to compile the static site from my content directory. So how do I make this public?

For hosting, I am using AWS. No, not a bare metal server that I built from the electrons with my own two hands (as any *real* engineer would). I do some homelabbing, but I'd rather not expose my servers to the internet. And to boot, I know Oracle Cloud Infrastructure (OCI) quite well, but AWS is a really important place to know how to build. As a competing cloud, it sharpens my knowledge to have multiple frames of reference and to see different implementations. 

Thus, once I have my bundle of HTML from `garage build`, I simply need to push that to an S3 bucket and the services I've configured in AWS do the rest. This makes for a really simple top-level picture. And all I need to do to deploy is to merge a PR to `main`! It sparks joy for me, since it's simple, secure, and very easy to build on.

*But Jacob, you've ignored the security angle!* Yes I have, and that's where all the fun is in explaining each step in more detail. 

So the overall flow is something like this:
```
content/posts/*.md     ← Writing
        │
        ▼
GitHub Actions         ← CI, then Deploy:
        │                  garage build + OIDC
        ▼
       S3              ← private bucket
        │
        ▼
   CloudFront          ← TLS, headers, cache
        │
        ▼
      you
```

Let's dig in.

## From push to deploy

### Writing

Writing is the easy part, especially since I wrote my own generator. I write a markdown flavor I choose and include frontmatter to tell the generator details about each post.

In the Garage I use [Goldmark](https://github.com/yuin/goldmark) as the Go markdown library, and bolt on the GFM extension to add some other features: tables, strikethrough, bare links, task lists. I also added a handful of others individually like footnotes. This is great because I can customize how my site supports the markdown syntax as I figure out what features I want.

I also use plain TOML for the frontmatter to include details on the post like the title, summary, date, and tags. There is also a draft field defined in the frontmatter if I'm not ready to post it, and the generator will skip it for me. Presently it looks like this, then my markdown post. That's it.

```
+++
title = "How this site is built (securely!)"
date = 2026-11-27
summary = "Designing this blog with security as a first-class citizen"
tags = ["meta", "go", "aws", "ci-cd", "github-actions", "github", "security"]
draft = false
+++
```

### Generating

The generator is a smallish Go program written with 3 capabilities: to build the `public/` output folder, to build and serve the site locally for my development, and to scaffold a new post with correct frontmatter (a nice to have).

```
$ make build # -> cmdBuild  
$ make serve # -> cmdServe  
$ make new TITLE="<post title>" # -> cmdNew
```

> 🔒 By default, Goldmark escapes any raw HTML like `<script>` and `<iframe>`, so it comes out as visible text instead of live markup. I mark Goldmark’s output as trusted HTML with a commented `#nosec`, which is only safe because of that escaping. So in this case, even trying to inject script tags wouldn't result in XSS on the Garage. Additionally, I added some build guardrails:
>    - reject any symlinks in `static/`
>    - require posts to have unique slugs
>    - check and refuse attempts to wipe directories that shouldn't be
>    - skip drafts and enforce strict frontmatter requirements to prevent broken or unintentional posts  
>
> This is defense in depth since I also define a strict CSP (to come).

### CI

Every PR runs CI to check the code before it can merge, `ci.yml`.

One reason Go is great is that it compiles down to a single binary that'll run just about anywhere. It also provides extensive tooling like testing built into the language with simple file naming. You don't have to go get `pytest` or another package and figure out how to use it. 

So CI can run `go mod verify`, `go vet`, `go test`, and `golangci-lint` (including `gosec`) from GitHub Actions to check the code is written correctly and safely.

> 🔒 GitHub Actions can be powerful but tricky. To keep this usage secure I pin the Actions to a SHA, only allow read operations from this Job, and prevent runs (in my repo's context) on fork PRs using pull_request (NOT pull_request_target). Dependabot also runs on this repo and suggests any upgrades to Go modules, Terraform, or Actions so I can properly maintain secure pins.

### Deploying

Once past the required CI success gate on the PR, I can merge. On `main`, I run a deploy step as well, `deploy.yml`. The Garage uses GitHub's OIDC to reach out to AWS and assume the deploy Role that I created in Terraform. Then the runner can use that deploy Role to run `aws s3 sync` against the site's S3 bucket. I also tell AWS to invalidate the CloudFront cache so any new requests come back to the bucket. This is separate from browser cache, which I'll get to in the CloudFront section.

> 🔒 This is where I give CI/CD access to AWS, so security is paramount. First, I wrote a tightly scoped permissions policy for the deploy Role, only allowing s3:ListBucket (for `aws s3 sync`), PutObject, DeleteObject, cloudfront:CreateInvalidation and GetInvalidation. This maintains the security principle of least privilege. This role is also mapped specifically to this Repo (with immutable IDs turned on to protect against repo renaming) and the "production" environment. OIDC is also leveraged to avoid the use of long running access keys, instead only allowing temporary access by signing the JWT with GitHub's key in exchange for short-lived secrets from AWS STS. Altogether this keeps every blast radius tight and short-lived with temporary credentials that last an hour, tops.

To drive this home, my initial deploys of the site actually failed, as they should have! I did not yet have the production environment set up in my repo to supply the variables: ARNs, Region, and other identifiers needed by the Action to request the proper role. Also note that these are variables, not secrets. I don't need to store any secrets in GitHub at all with this CI/CD deployment.

## From request to page load

### DNS and TLS

To set up a public website, there were some initial manual steps. I used AWS Route 53 to purchase the domain "gilsgarage.dev". By purchasing the domain I automatically get a hosted zone, so Terraform manages records there for me.

Once I had a domain, I created AWS ACM resources to issue the certificate for it. For CloudFront to use it, this has to be in `us-east-1`, and my CloudFront configuration requires TLS 1.2+. And with this level of control, it's easy for me to set up alias records for both "gilsgarage.dev" and the "www." prefix to point to CloudFront. 

I can do this all in Terraform so I have a record on any resources I want to build, update, or destroy later. For instance, I created DNS validation records in Route 53 so the cert renews automatically.

> 🔒 The Garage also enforces HTTP Strict Transport Security (HSTS). This informs browsers to only ever use HTTPS for my domain for 2 years from the latest visit. As you'll see shortly, the CloudFront configuration already performs the redirect to HTTPS, but that first request goes over HTTP. By using HSTS and sending the preload flag, I can ensure connections to my site are **always** encrypted after the first visit on any domain, and from the very first visit here thanks to ".dev" TLD protections. Using HSTS protects the site against SSL stripping attacks.

### CloudFront

The next resource I create in Terraform, and the real star of the show, is AWS CloudFront. Essentially, this allows me as an AWS user to leverage a CDN for the Garage. This is really slick for a small blog to use edge caching to be closer to users and limit latency. I only bias CloudFront for North America and Europe, but it costs less than a dollar a month to serve the Garage this way! Anyone can read the Garage, but readers (and likely the vast majority of them) in those regions can get a much faster experience.

There is also a small CloudFront JavaScript function I need to properly rewrite requests from `/posts/foo/` -> `/posts/foo/index.html`. This is a quirk of the security choice to maintain a private bucket. S3 would happily serve index.html from a public endpoint, but I locked it down so CloudFront needs to do the conversion itself. A small price to pay for better control of the content on the site.

The JavaScript function also performs the redirect from www -> apex, but separately CloudFront itself handles the HTTPS redirect with one line in the default cache behavior.

```
viewer_protocol_policy     = "redirect-to-https"
```

CloudFront also optimizes caching for me, which I can then invalidate on every deploy so users refetch the latest data from the bucket. Separately, I set Cache-Control on the `aws s3 sync`. New posts are retrieved immediately, while stylistic changes like CSS and images are cached for up to a day.


> 🔒 By using CloudFront, I can also set up some headers for all requests to use to enhance security. This includes HSTS (from earlier), a permissions policy to turn off unwanted browser features like geolocation and camera access, a Content-Security-Policy (CSP), plus a few other standard choices. My CSP restricts against execution by almost anything other than my site. The only exception is images so I can share online images. Outside of that, everything defaults to my origin. That means no inline JavaScript or styling. This is more protection against any potential XSS attacks. So a compromised third-party script can't run here, because none can load.

![securityheaders.com report for gilsgarage.dev: grade A+, with all six security headers present](/img/securityheaders-a-plus.webp)

*Scanned with securityheaders.com on November 15, 2026. Grades reflect the headers and the site's criteria at that time.*

### S3

Last but not least, I built a private bucket to be the home of the page content. The bucket is updated on each deploy, and its permissions are scoped by least privilege (apart from admin of course). The deploy Role writes, and CloudFront reads. That's it, no public or unnecessary access.

There's not much more to say here other than I find it really cool that I can host a site with nothing but an object store, a domain, and CloudFront with a little GitHub Actions magic.

> 🔒 CloudFront accesses the bucket with Origin Access Control (OAC) by signing every request with SigV4. The bucket has a corresponding policy that admits only the CloudFront service to access the bucket, with a condition that the ARN matches my distribution. Public access is blocked, so the only way in is through my CloudFront distribution, which also adds the headers I set, HTTPS, and the rewrite JavaScript function. In addition, AWS returns 403s for missing or inaccessible objects. To maintain the same behavior (so nothing leaks on what exists) but give readers a better message, I instead serve the 404 error page.


## Conclusion

I've written a few personal sites before, but this is the first time I feel I've struck the right balance of simple to understand, cheap and easy to maintain, and secure in implementation. 

I learned a lot about these features during the build of the site, considering I'm not a web developer by trade. But in doing so it drives home how hard it can be to get security right. Doing something that seems obvious like making the bucket private meant adding more scripting to enable the resources to work together nicely. And frankly, I'm sure there are things I haven't addressed.

More than anything it really scratches the itch of an elegant, secure design, even if only until it breaks or I outgrow it. It's all in the pursuit of knowledge, and plowing the green fields ahead.


> 🛠️ [Check out the Garage](https://github.com/JG3233/gils-garage)


### Currently
- **Reading:** [A Philosophy of Software Design](https://www.goodreads.com/book/show/39996759-a-philosophy-of-software-design), John Ousterhout
- **Studying:** Go, system design
- **Listening:** [The Infinity Machine](https://www.goodreads.com/en/book/show/241434373-the-infinity-machine)


#### A note about AI use
AI was used in the creation of this post as a spellcheck, reviewer, and technical teacher. All voice and words are my own.
