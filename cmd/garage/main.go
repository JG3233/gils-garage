// Command garage builds, previews and scaffolds posts for Gil's Garage.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JG3233/gils-garage/internal/site"
)

const usage = `garage — static blog generator

Usage:
  garage build [-drafts] [-out public]   render the site
  garage serve [-addr 127.0.0.1:8080]   build with drafts and serve locally
  garage new "Post title"               create a draft post
`

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "build":
		err = cmdBuild(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "new":
		err = cmdNew(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("garage: %v", err)
	}
}

type common struct {
	config, content, static, out string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", "site.toml", "site config file")
	fs.StringVar(&c.content, "content", "content", "content directory")
	fs.StringVar(&c.static, "static", "static", "static assets directory")
	fs.StringVar(&c.out, "out", "public", "output directory (wiped on build)")
}

func (c *common) build(drafts bool) error {
	cfg, err := site.LoadConfig(c.config)
	if err != nil {
		return err
	}
	start := time.Now()
	if err := site.Build(cfg, site.Options{ContentDir: c.content, StaticDir: c.static, OutDir: c.out, IncludeDrafts: drafts}); err != nil {
		return err
	}
	log.Printf("built %s in %s", c.out, time.Since(start).Round(time.Millisecond))
	return nil
}

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	var c common
	c.register(fs)
	drafts := fs.Bool("drafts", false, "include draft posts")
	_ = fs.Parse(args)
	return c.build(*drafts)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var c common
	c.register(fs)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	_ = fs.Parse(args)
	if err := c.build(true); err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           http.FileServer(http.Dir(c.out)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("serving http://%s (drafts included; re-run to rebuild)", *addr)
	return srv.ListenAndServe()
}

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	content := fs.String("content", "content", "content directory")
	_ = fs.Parse(args)
	title := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if title == "" {
		return errors.New(`usage: garage new "Post title"`)
	}
	today := time.Now().Format("2006-01-02")
	path := filepath.Join(*content, "posts", today+"-"+site.Slugify(title)+".md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	body := fmt.Sprintf("+++\ntitle = %q\ndate = %s\nsummary = \"\"\ntags = []\ndraft = true\n+++\n\n", title, today)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}
