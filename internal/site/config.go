package site

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the site-wide configuration loaded from site.toml.
type Config struct {
	Title       string `toml:"title"`
	Description string `toml:"description"`
	BaseURL     string `toml:"base_url"`
	Author      string `toml:"author"`
}

// LoadConfig reads and validates site.toml.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := toml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Title == "" {
		return Config{}, errors.New("site.toml: title is required")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Config{}, fmt.Errorf("site.toml: base_url must be an absolute URL, got %q", c.BaseURL)
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	return c, nil
}
