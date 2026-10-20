package site

import (
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

// Options controls a single build.
type Options struct {
	ContentDir    string // directory holding posts/*.md
	StaticDir     string // copied verbatim into the output root (optional)
	OutDir        string // wiped and regenerated
	IncludeDrafts bool
}

type pageData struct {
	Site  Config
	Title string
	Posts []Post
	Post  *Post
	Tag   string
	Tags  []tagCount
	Year  int
}

type tagCount struct {
	Name  string
	Count int
}

// Build renders the whole site into opts.OutDir.
func Build(cfg Config, opts Options) error {
	if err := safeOutDir(opts.OutDir); err != nil {
		return err
	}
	posts, err := LoadPosts(filepath.Join(opts.ContentDir, "posts"), NewMarkdown(), opts.IncludeDrafts)
	if err != nil {
		return err
	}
	tmpl, err := parseTemplates()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(opts.OutDir); err != nil {
		return err
	}

	year := time.Now().Year()
	byTag := map[string][]Post{}
	for _, p := range posts {
		for _, t := range p.Tags {
			byTag[t] = append(byTag[t], p)
		}
	}
	tags := make([]tagCount, 0, len(byTag))
	for t, ps := range byTag {
		tags = append(tags, tagCount{t, len(ps)})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })

	render := func(name, rel string, d pageData) error {
		d.Site, d.Year = cfg, year
		return writeTemplate(tmpl[name], filepath.Join(opts.OutDir, rel), d)
	}

	if err := render("index", "index.html", pageData{Posts: posts, Tags: tags}); err != nil {
		return err
	}
	for i := range posts {
		p := posts[i]
		if err := render("post", filepath.Join("posts", p.Slug, "index.html"), pageData{Title: p.Title, Post: &p}); err != nil {
			return err
		}
	}
	for _, t := range tags {
		if err := render("tag", filepath.Join("tags", t.Name, "index.html"), pageData{Title: "#" + t.Name, Tag: t.Name, Posts: byTag[t.Name]}); err != nil {
			return err
		}
	}
	if err := render("notfound", "404.html", pageData{Title: "Not found"}); err != nil {
		return err
	}
	if err := writeFeed(cfg, posts, filepath.Join(opts.OutDir, "feed.xml")); err != nil {
		return err
	}
	if opts.StaticDir != "" {
		if err := copyDir(opts.StaticDir, opts.OutDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

func safeOutDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	wd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	if dir == "" || abs == "/" || abs == wd || abs == home {
		return fmt.Errorf("refusing to use %q as output dir (it gets wiped)", dir)
	}
	return nil
}

func parseTemplates() (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"date": func(t time.Time) string { return t.Format("January 2, 2006") },
		"iso":  func(t time.Time) string { return t.Format("2006-01-02") },
	}
	out := map[string]*template.Template{}
	for _, name := range []string{"index", "post", "tag", "notfound"} {
		t, err := template.New("base.html").Funcs(funcs).ParseFS(templateFS, "templates/base.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		out[name] = t
	}
	return out, nil
}

func writeTemplate(t *template.Template, path string, data pageData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.Create(path) // #nosec G304 -- path is under OutDir
	if err != nil {
		return err
	}
	if err := t.Execute(f, data); err != nil {
		_ = f.Close()
		return fmt.Errorf("render %s: %w", path, err)
	}
	return f.Close()
}

// --- Atom feed ---

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Link    atomLink `xml:"link"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary,omitempty"`
	Content struct {
		Type string `xml:"type,attr"`
		Body string `xml:",chardata"`
	} `xml:"content"`
}

type atomFeed struct {
	XMLName xml.Name   `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string     `xml:"title"`
	ID      string     `xml:"id"`
	Links   []atomLink `xml:"link"`
	Updated string     `xml:"updated"`
	Author  struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Entries []atomEntry `xml:"entry"`
}

func writeFeed(cfg Config, posts []Post, path string) error {
	feed := atomFeed{
		Title: cfg.Title,
		ID:    cfg.BaseURL + "/",
		Links: []atomLink{{Href: cfg.BaseURL + "/"}, {Href: cfg.BaseURL + "/feed.xml", Rel: "self"}},
	}
	feed.Author.Name = cfg.Author
	feed.Updated = time.Unix(0, 0).UTC().Format(time.RFC3339)
	if len(posts) > 0 {
		feed.Updated = posts[0].Date.UTC().Format(time.RFC3339)
	}
	for i, p := range posts {
		if i == 20 {
			break
		}
		e := atomEntry{
			Title:   p.Title,
			ID:      cfg.BaseURL + p.URL(),
			Link:    atomLink{Href: cfg.BaseURL + p.URL()},
			Updated: p.Date.UTC().Format(time.RFC3339),
			Summary: p.Summary,
		}
		e.Content.Type = "html"
		e.Content.Body = string(p.Body)
		feed.Entries = append(feed.Entries, e)
	}
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), out...), 0o600)
}

// copyDir copies src into dst through an os.Root so symlinks can't escape src.
// os.CopyFS refuses to overwrite, so a static file can't clobber a generated page.
func copyDir(src, dst string) error {
	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return os.CopyFS(dst, root.FS())
}
