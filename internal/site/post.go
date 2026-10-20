package site

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Post is a single Markdown post with TOML front matter delimited by +++ lines.
type Post struct {
	Title   string    `toml:"title"`
	Date    time.Time `toml:"date"`
	Summary string    `toml:"summary"`
	Tags    []string  `toml:"tags"`
	Slug    string    `toml:"slug"`
	Draft   bool      `toml:"draft"`

	Body       template.HTML `toml:"-"`
	SourcePath string        `toml:"-"`
}

// URL is the site-relative path of the rendered post.
func (p Post) URL() string { return "/posts/" + p.Slug + "/" }

var (
	frontMatterDelim = []byte("+++")
	nonSlug          = regexp.MustCompile(`[^a-z0-9]+`)
	datePrefix       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)
)

// Slugify turns arbitrary text into a URL-safe slug.
func Slugify(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// splitFrontMatter separates a leading TOML block delimited by "+++" lines from the body.
func splitFrontMatter(src []byte) (meta, body []byte, err error) {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(src, append(frontMatterDelim, '\n')) {
		return nil, nil, errors.New("missing front matter: file must start with +++")
	}
	rest := src[len(frontMatterDelim)+1:]
	end := bytes.Index(rest, []byte("\n+++\n"))
	if end < 0 {
		if bytes.HasSuffix(rest, []byte("\n+++")) {
			return rest[:len(rest)-4], nil, nil
		}
		return nil, nil, errors.New("unterminated front matter")
	}
	return rest[:end], rest[end+5:], nil
}

// ParsePost reads front matter and renders the Markdown body of a post file.
func ParsePost(path string, md Markdown) (Post, error) {
	src, err := os.ReadFile(path) // #nosec G304 -- paths come from the content dir walk
	if err != nil {
		return Post{}, err
	}
	meta, body, err := splitFrontMatter(src)
	if err != nil {
		return Post{}, fmt.Errorf("%s: %w", path, err)
	}
	var p Post
	if err := toml.Unmarshal(meta, &p); err != nil {
		return Post{}, fmt.Errorf("%s: front matter: %w", path, err)
	}
	if p.Title == "" {
		return Post{}, fmt.Errorf("%s: title is required", path)
	}
	if p.Date.IsZero() {
		return Post{}, fmt.Errorf("%s: date is required", path)
	}
	if p.Slug == "" {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		p.Slug = datePrefix.ReplaceAllString(base, "")
	}
	p.Slug = Slugify(p.Slug)
	for i, t := range p.Tags {
		p.Tags[i] = Slugify(t)
	}
	html, err := md.Render(body)
	if err != nil {
		return Post{}, fmt.Errorf("%s: markdown: %w", path, err)
	}
	p.Body = template.HTML(html) // #nosec G203 -- goldmark escapes raw HTML (unsafe mode off)
	p.SourcePath = path
	return p, nil
}

// LoadPosts parses every *.md file in dir, newest first. Drafts are skipped unless includeDrafts.
func LoadPosts(dir string, md Markdown, includeDrafts bool) ([]Post, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	posts := make([]Post, 0, len(files))
	seen := map[string]string{}
	for _, f := range files {
		p, err := ParsePost(f, md)
		if err != nil {
			return nil, err
		}
		if p.Draft && !includeDrafts {
			continue
		}
		if other, dup := seen[p.Slug]; dup {
			return nil, fmt.Errorf("duplicate slug %q in %s and %s", p.Slug, other, f)
		}
		seen[p.Slug] = f
		posts = append(posts, p)
	}
	sort.SliceStable(posts, func(i, j int) bool { return posts[i].Date.After(posts[j].Date) })
	return posts, nil
}
