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
	writePost(t, posts, "2026-01-01-first.md", "+++\ntitle = \"First\"\ndate = 2026-01-01\n+++\n# Heading\n")
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

func TestSafeOutDir(t *testing.T) {
	for _, d := range []string{"", "/", "."} {
		if err := safeOutDir(d); err == nil {
			t.Errorf("safeOutDir(%q) should refuse", d)
		}
	}
}
