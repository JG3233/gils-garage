package site

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// Markdown renders Markdown to HTML. Raw HTML in posts is escaped (goldmark's safe default).
type Markdown struct{ gm goldmark.Markdown }

// NewMarkdown returns a renderer with GFM tables, strikethrough, autolinks, task lists and footnotes.
func NewMarkdown() Markdown {
	return Markdown{gm: goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.Typographer),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)}
}

// Render converts Markdown source to HTML.
func (m Markdown) Render(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := m.gm.Convert(src, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
