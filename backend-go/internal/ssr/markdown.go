package ssr

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
)

// mdToHTML converts a markdown string to safe HTML for SSR rendering.
// Used for blueprint descriptions and blog posts (trusted content).
func mdToHTML(md string) template.HTML {
	if md == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := goldmark.New().Convert([]byte(md), &buf); err != nil {
		return template.HTML(esc(md))
	}
	return template.HTML(buf.String())
}
