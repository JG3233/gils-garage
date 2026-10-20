+++
title = "Hello, garage"
date = 2026-10-19
summary = "First post. What this blog is for and how it's built."
tags = ["meta", "go"]
draft = true
+++

This is the first post on **Gil's Garage**, a small static site generator written in Go,
deployed to S3 + CloudFront by GitHub Actions using OIDC (no long-lived AWS keys).

Flip `draft = true` to `false` when it's ready to publish.

```go
fmt.Println("hello, garage")
```
