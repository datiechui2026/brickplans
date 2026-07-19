// Package ssr renders server-side HTML for SEO/GEO: page-specific <title>/meta,
// JSON-LD structured data, and a simplified <noscript> body that AI crawlers
// (which don't execute JS) can read. The SPA script then hydrates #app as usual.
package ssr

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

//go:embed template.html
var templateHTML string

const favicon = "/favicon.svg" // SVG brick icon, served from frontend/public/favicon.svg

// PageData is the template payload.
type PageData struct {
	Title         string
	Description   string
	Canonical     string
	OGTitle       string
	OGDescription string
	OGImage       string
	OGUrl         string
	OGType        string
	JSONLD        []template.HTML
	Noscript      template.HTML
	JSFile        string
	CSSFile       string
	Favicon       template.HTML
}

// Renderer parses the template once and resolves vite's hashed asset filenames
// from the build manifest (so SSR HTML references /assets/main-<hash>.js).
type Renderer struct {
	tmpl      *template.Template
	jsFile    string
	cssFile   string
	publicURL string
}

func NewRenderer(distDir, publicURL string) *Renderer {
	t, err := template.New("page").Parse(templateHTML)
	if err != nil {
		log.Fatalf("ssr template parse: %v", err)
	}
	js, css := loadManifest(distDir)
	if js == "" {
		log.Printf("[ssr] warning: vite manifest not found under %s — SSR HTML will load no SPA script (set FRONTEND_DIST)", distDir)
	}
	return &Renderer{tmpl: t, jsFile: js, cssFile: css, publicURL: publicURL}
}

type manifestEntry struct {
	File string   `json:"file"`
	CSS  []string `json:"css"`
}

func loadManifest(distDir string) (js, css string) {
	b, err := os.ReadFile(filepath.Join(distDir, "manifest.json"))
	if err != nil {
		return
	}
	var m map[string]manifestEntry
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("[ssr] manifest parse: %v", err)
		return
	}
	if entry, ok := m["index.html"]; ok {
		js = entry.File
		if len(entry.CSS) > 0 {
			css = entry.CSS[0]
		}
	}
	return
}

// Render fills asset/favicon defaults and writes the HTML.
func (r *Renderer) Render(c *gin.Context, data PageData) {
	data.JSFile = r.jsFile
	data.CSSFile = r.cssFile
	data.Favicon = favicon
	if data.OGImage == "" {
		data.OGImage = r.publicURL + "/og-default.png"
	}
	if data.OGUrl == "" {
		data.OGUrl = data.Canonical
	}
	if data.OGTitle == "" {
		data.OGTitle = data.Title
	}
	if data.OGDescription == "" {
		data.OGDescription = data.Description
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := r.tmpl.Execute(c.Writer, data); err != nil {
		log.Printf("[ssr] template execute: %v", err)
	}
}
