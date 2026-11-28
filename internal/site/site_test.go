package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":             "hello-world",
		"  OIDC + GitHub Actions  ": "oidc-github-actions",
		"Go 1.24":                   "go-1-24",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFrontMatter(t *testing.T) {
	meta, body, err := splitFrontMatter([]byte("+++\r\ntitle = \"x\"\r\n+++\r\nbody\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(meta) != `title = "x"` || string(body) != "body\n" {
		t.Fatalf("meta=%q body=%q", meta, body)
	}
	if _, _, err := splitFrontMatter([]byte("no front matter")); err == nil {
		t.Fatal("expected error for missing front matter")
	}
	if _, _, err := splitFrontMatter([]byte("+++\ntitle = \"x\"\n")); err == nil {
		t.Fatal("expected error for unterminated front matter")
	}
}

func writePost(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuild(t *testing.T) {
	root := t.TempDir()
	posts := filepath.Join(root, "content", "posts")
	if err := os.MkdirAll(posts, 0o750); err != nil {
		t.Fatal(err)
	}
	writePost(t, posts, "2026-01-02-second.md", "+++\ntitle = \"Second\"\ndate = 2026-01-02\ntags = [\"Cloud Security\"]\n+++\nHi <script>alert(1)</script>\n")
	writePost(t, posts, "2026-01-01-first.md", "+++\ntitle = \"First\"\ndate = 2026-01-01\nimage = \"/img/first.png\"\n+++\n# Heading\n")
	writePost(t, posts, "2026-01-03-wip.md", "+++\ntitle = \"WIP\"\ndate = 2026-01-03\ndraft = true\n+++\nsecret\n")

	out := filepath.Join(root, "public")
	cfg := Config{Title: "Test", BaseURL: "https://example.com", Author: "me"}
	if err := Build(cfg, Options{ContentDir: filepath.Join(root, "content"), OutDir: out}); err != nil {
		t.Fatal(err)
	}

	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(out, rel)) // #nosec G304 -- test temp dir
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	index := read("index.html")
	if strings.Index(index, "Second") > strings.Index(index, "First") {
		t.Error("posts should be newest first")
	}
	if strings.Contains(index, "WIP") {
		t.Error("draft leaked into index")
	}
	if _, err := os.Stat(filepath.Join(out, "posts", "wip")); !os.IsNotExist(err) {
		t.Error("draft page was rendered")
	}
	if p := read("posts/second/index.html"); strings.Contains(p, "<script>alert") {
		t.Error("raw HTML in markdown must be escaped")
	}
	second := read("posts/second/index.html")
	for _, want := range []string{
		`<meta property="og:url" content="https://example.com/posts/second/">`,
		`<meta property="og:image" content="https://example.com/img/og-default.png">`,
		`<meta property="og:type" content="article">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	} {
		if !strings.Contains(second, want) {
			t.Errorf("post page missing %s", want)
		}
	}
	if p := read("posts/first/index.html"); !strings.Contains(p, `<meta property="og:image" content="https://example.com/img/first.png">`) {
		t.Error("per-post image not used for og:image")
	}
	if !strings.Contains(index, `<meta property="og:type" content="website">`) {
		t.Error("index should be og:type website")
	}
	read("tags/cloud-security/index.html")
	read("404.html")
	if feed := read("feed.xml"); !strings.Contains(feed, "https://example.com/posts/second/") {
		t.Error("feed missing absolute post URL")
	}
}

func TestDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	writePost(t, dir, "a.md", "+++\ntitle = \"A\"\ndate = 2026-01-01\nslug = \"same\"\n+++\n")
	writePost(t, dir, "b.md", "+++\ntitle = \"B\"\ndate = 2026-01-02\nslug = \"same\"\n+++\n")
	if _, err := LoadPosts(dir, NewMarkdown(), false); err == nil {
		t.Fatal("expected duplicate slug error")
	}
}

func TestEmptySlugOrTag(t *testing.T) {
	dir := t.TempDir()
	writePost(t, dir, "slug.md", "+++\ntitle = \"A\"\ndate = 2026-01-01\nslug = \"!!!\"\n+++\n")
	writePost(t, dir, "tag.md", "+++\ntitle = \"B\"\ndate = 2026-01-01\ntags = [\"ok\", \"???\"]\n+++\n")
	for _, f := range []string{"slug.md", "tag.md"} {
		if _, err := ParsePost(filepath.Join(dir, f), NewMarkdown()); err == nil {
			t.Errorf("%s: expected error for slug/tag with no letters or digits", f)
		}
	}
}

func TestImageMustBeSiteRelative(t *testing.T) {
	dir := t.TempDir()
	for i, img := range []string{"img/x.png", "//evil.example/x.png", "https://evil.example/x.png"} {
		f := filepath.Join(dir, "p"+string(rune('a'+i))+".md")
		writePost(t, dir, filepath.Base(f), "+++\ntitle = \"A\"\ndate = 2026-01-01\nimage = \""+img+"\"\n+++\n")
		if _, err := ParsePost(f, NewMarkdown()); err == nil {
			t.Errorf("image %q: expected error", img)
		}
	}
}

func TestSafeOutDir(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("HOME", filepath.Join(root, "home"))
	for _, d := range []string{"content/posts", "static", "home", "notbuild", "empty", "prev"} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writePost(t, "notbuild", "main.go", "package main\n")
	writePost(t, "prev", "404.html", "")
	writePost(t, "prev", "feed.xml", "")

	for _, d := range []string{"", "/", ".", "..", "home", "content", "content/posts", "static", "static/out", "notbuild"} {
		if err := safeOutDir(Options{OutDir: d, ContentDir: "content", StaticDir: "static"}); err == nil {
			t.Errorf("safeOutDir(%q) should refuse", d)
		}
	}
	for _, d := range []string{"public", "empty", "prev"} {
		if err := safeOutDir(Options{OutDir: d, ContentDir: "content", StaticDir: "static"}); err != nil {
			t.Errorf("safeOutDir(%q) = %v, want ok", d, err)
		}
	}
}

func TestStaticSymlinkRejectedBeforeWipe(t *testing.T) {
	root := t.TempDir()
	posts := filepath.Join(root, "content", "posts")
	static := filepath.Join(root, "static")
	out := filepath.Join(root, "public")
	for _, d := range []string{posts, static, out} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	writePost(t, posts, "2026-01-01-a.md", "+++\ntitle = \"A\"\ndate = 2026-01-01\n+++\n")
	writePost(t, root, "secret", "TOPSECRET")
	if err := os.Symlink(filepath.Join(root, "secret"), filepath.Join(static, "leak")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	// A previous build that must survive the failed one.
	writePost(t, out, "404.html", "")
	writePost(t, out, "feed.xml", "")

	cfg := Config{Title: "Test", BaseURL: "https://example.com"}
	err := Build(cfg, Options{ContentDir: filepath.Join(root, "content"), StaticDir: static, OutDir: out})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "feed.xml")); err != nil {
		t.Error("failed build wiped the previous output")
	}
}
