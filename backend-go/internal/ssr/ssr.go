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
	"strings"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
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
	JSONLD        []template.JS
	SiteNav       template.HTML
	Noscript      template.HTML
	// BodyClass, when non-empty, wraps Noscript in the same container skeleton
	// the SPA builds for the equivalent page:
	//   <div class="page active"><div class="main"><div class="BodyClass">…</div></div></div>
	// Only pages the SPA will NOT hydrate need it (today: every /en/* page — see
	// frontend isSsrOnlyPath). For those pages the SSR body IS the visible page,
	// and without these classes main.css has nothing to attach .main/.blog-list/
	// .blog-detail to, so the content renders as unstyled text at the top-left of
	// the viewport (P0, 2026-10-15). Chinese pages leave it empty: their markup
	// stays byte-identical because the SPA replaces #app on boot anyway.
	BodyClass string
	JSFile    string
	CSSFile   string
	Favicon   template.HTML
	// Lang is the page language code ("zh" | "en"). Empty means the default
	// language (Chinese) — most pages are Chinese-only and stay that way.
	// It drives <html lang>, the SSR nav labels and the language switcher.
	Lang string
	// HTMLLang sets <html lang>. Empty means zh-CN (every non-blog page).
	HTMLLang string
	// Alternates renders the hreflang link tags. Empty means "this page has no
	// translated counterpart": the template then emits only a self-referencing
	// zh-CN + x-default pair, which is what every Chinese page had before.
	Alternates []AltLink
}

// AltLink is one <link rel="alternate" hreflang=...> entry.
type AltLink struct {
	Hreflang string
	Href     string
}

// siteNavHTML is the server-rendered site navigation emitted into #app ahead of
// The SSR navigation is now built per request by navHTML() (see i18n.go): it is
// language-aware (labels + hrefs follow the page language) and carries the
// language switcher. It is emitted into #app ahead of the page body so crawlers
// and users without JS see a complete menu; the SPA replaces #app on boot and
// renders its own equivalent.

// Renderer parses the template once and resolves vite's hashed asset filenames
// from the build manifest (so SSR HTML references /assets/main-<hash>.js).
type Renderer struct {
	tmpl      *template.Template
	jsFile    string
	cssFile   string
	publicURL string
	store     *blog.Store
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

// SetBlogStore wires the blog store in after construction (the router loads it,
// the renderer is built earlier in cmd/server). It is only used to decide whether
// a blog article has a counterpart in the other language when building the
// language switcher's target — a nil store degrades to "switch to the blog list",
// which is still a valid page in every language.
func (r *Renderer) SetBlogStore(s *blog.Store) { r.store = s }

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
	lang := langFor(data.Lang)
	data.Lang = lang.Code
	if data.HTMLLang == "" {
		data.HTMLLang = lang.HTMLLang
	}
	data.SiteNav = navHTML(lang, r.switchTargets(c, lang))
	// Opt-in container skeleton for pages the SPA never hydrates (see BodyClass).
	if data.BodyClass != "" {
		data.Noscript = wrapBody(data.BodyClass, data.Noscript)
	}
	// Static pages (home, /explore, /blog, /about, /faq, /privacy) exist in every
	// language, so they get the full hreflang cluster without every handler
	// having to ask for it. Blog articles set Alternates themselves (and must not
	// be overridden: an article only gets alternates when a real counterpart
	// exists).
	if data.Alternates == nil {
		if neutral, ok := neutralStaticPath(c.Request.URL.Path); ok {
			data.Alternates = staticAlternates(neutral, r.publicURL)
		}
	}
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
	c.Header("Cache-Control", "public, max-age=3600") // HTML pages: 1 hour browser cache
	if err := r.tmpl.Execute(c.Writer, data); err != nil {
		log.Printf("[ssr] template execute: %v", err)
	}
}

// neutralStaticPath maps a request path to its language-neutral form when — and
// only when — that path is one of the static pages shared by all languages.
// "/en/about" → ("/about", true); "/en/blog/some-post" → ("", false).
func neutralStaticPath(p string) (string, bool) {
	trimmed := strings.TrimSuffix(p, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	for _, l := range siteLangs {
		if l.Prefix == "" {
			continue
		}
		if trimmed == l.Prefix {
			trimmed = "/"
			break
		}
		if strings.HasPrefix(trimmed, l.Prefix+"/") {
			trimmed = strings.TrimPrefix(trimmed, l.Prefix)
			break
		}
	}
	for _, sp := range staticPaths {
		if trimmed == sp {
			return sp, true
		}
	}
	return "", false
}

// switchTargets computes, for each other language, the URL the switcher should
// link to from the CURRENT page. Rule (per the product spec):
//   - the current page has a version in that language → link to it
//   - it does not → link to that language's blog list, which always has content
func (r *Renderer) switchTargets(c *gin.Context, current Lang) map[string]string {
	raw := strings.TrimSuffix(c.Request.URL.Path, "/")
	if raw == "" {
		raw = "/"
	}
	// Strip the current language's prefix to get the neutral path.
	neutral := raw
	if current.Prefix != "" {
		if raw == current.Prefix {
			neutral = "/"
		} else if strings.HasPrefix(raw, current.Prefix+"/") {
			neutral = strings.TrimPrefix(raw, current.Prefix)
		}
	}

	targets := make(map[string]string, len(siteLangs))
	for _, l := range otherLangs(current.Code) {
		switch {
		case neutral == "/" || isStaticPath(neutral):
			targets[l.Code] = l.URL(neutral)
		case strings.HasPrefix(neutral, "/blog/"):
			slug := strings.TrimPrefix(neutral, "/blog/")
			if r.store != nil && r.store.HasLang(slug, l.Code) {
				targets[l.Code] = l.URL("/blog/" + slug)
			} else {
				targets[l.Code] = l.URL("/blog")
			}
		default:
			// /detail/<id>, /tags/…, /user/… — no counterpart tree exists.
			targets[l.Code] = l.URL("/blog")
		}
	}
	return targets
}

func isStaticPath(p string) bool {
	for _, sp := range staticPaths {
		if p == sp {
			return true
		}
	}
	return false
}
