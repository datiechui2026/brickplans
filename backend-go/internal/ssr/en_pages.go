package ssr

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
)

// ── 英文静态页（/en/、/en/about、/en/faq、/en/privacy、/en/explore）─────────
//
// 这些页面是 M2 英文站的"壳"：SSR 直接给完整英文 HTML（爬虫 + 无 JS 用户），
// 前端 SPA 对这几个路径不做 hydration（见 frontend/src/main.js SSR_ONLY_PATHS），
// 因此英文壳不会被中文 SPA 覆盖。
//
// 中文站对应页一律不动：canonical/URL 全部沿用（SEO 已收录）。

// enHomeBlogLimit caps the English homepage's article module. It is much higher
// than homeBlogLimit (12) on purpose — see the comment in HomeEN.
const enHomeBlogLimit = 60

// webPageJSONLD 是英文静态页的通用 WebPage 节点。inLanguage 必须跟页面语言走，
// 否则又是一个"英文页声明 zh-CN"的错误标注。
func webPageJSONLD(public, path, name, description, lang string) template.JS {
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "WebPage",
		"@id":         public + path + "#webpage",
		"url":         public + path,
		"name":        name,
		"description": description,
		"inLanguage":  langFor(lang).HTMLLang,
		"isPartOf":    map[string]interface{}{"@type": "WebSite", "@id": public + "/#website"},
	})
}

// aboutPageJSONLDFor is aboutPageJSONLD with the page language threaded through
// (the Chinese default stays for /about).
func aboutPageJSONLDFor(public, lang string) template.JS {
	if lang != "en" {
		return aboutPageJSONLD(public)
	}
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "AboutPage",
		"@id":         public + "/en/about#aboutpage",
		"url":         public + "/en/about",
		"name":        "About BrickPlan",
		"description": "Background, team, contact details and the content policy behind the BrickPlan brick/MOC blueprint community.",
		"inLanguage":  "en",
		"isPartOf":    map[string]interface{}{"@type": "WebSite", "@id": public + "/#website"},
		"about":       map[string]interface{}{"@id": public + "/#organization"},
		"mainEntity":  map[string]interface{}{"@id": public + "/#organization"},
	})
}

// HomeEN renders the English home page at /en/.
func (h *Handler) HomeEN(c *gin.Context) {
	var posts []blog.Post
	if h.blogStore != nil {
		posts = h.blogStore.AllByLang(blog.LangEN)
		// The Chinese home caps its blog module at homeBlogLimit. The English home
		// deliberately lists the WHOLE English tree (up to enHomeBlogLimit): the
		// homepage is the strongest internal link source on the site, and the
		// English tree is small — listing every article there is what gives each
		// /en/blog/<slug> a referrer instead of leaving it sitemap-only.
		if len(posts) > enHomeBlogLimit {
			posts = posts[:enHomeBlogLimit]
		}
	}
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, webPageJSONLD(h.cfg.PublicURL, "/en/",
		"BrickPlan — MOC Blueprints, Building Guides and Brick Reviews",
		"Browse MOC blueprints and read English guides on brick parts, brands, techniques and set reviews.", "en"))

	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "BrickPlan — MOC Blueprints, Building Guides and Brick Reviews",
		Description: "Browse MOC blueprints and read English guides on brick parts, brand comparisons, building techniques and set reviews.",
		Canonical:   h.cfg.PublicURL + "/en/",
		OGType:      "website",
		JSONLD:      jsonld,
		Noscript:    enHomeNoscript(posts, h.cfg.PublicURL),
	})
}

// enHomeNoscript is the crawlable English home body: hero copy + the latest
// English articles, so /en/ is not an orphan hub.
func enHomeNoscript(posts []blog.Post, public string) template.HTML {
	var b strings.Builder
	b.WriteString("<article>")
	b.WriteString("<h1>BrickPlan — MOC Blueprints, Building Guides and Brick Reviews</h1>")
	b.WriteString("<p>BrickPlan is a blueprint-sharing community for brick and MOC builders. The English section covers buying guides, part knowledge, brand comparisons, building techniques and set reviews.</p>")
	if len(posts) > 0 {
		b.WriteString("<h2>Latest English articles</h2><ul>")
		for _, p := range posts {
			b.WriteString(fmt.Sprintf(`<li><a href="%s/en/blog/%s">%s</a> <span>(%s)</span></li>`,
				public, esc(p.Slug), esc(p.Title), esc(p.Date.Format("2006-01-02"))))
		}
		b.WriteString("</ul>")
	}
	b.WriteString(`<p><a href="/en/blog">All English articles</a> · <a href="/en/about">About</a> · <a href="/en/faq">FAQ</a> · <a href="/explore">Browse blueprints</a></p>`)
	b.WriteString("</article>")
	b.WriteString(string(homeFooterEN(public)))
	return template.HTML(b.String())
}

// AboutEN renders the English about page at /en/about.
func (h *Handler) AboutEN(c *gin.Context) {
	public := h.cfg.PublicURL
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, aboutPageJSONLDFor(public, "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "About BrickPlan — MOC blueprint community",
		Description: "Background, team, contact details and the content policy behind the BrickPlan brick/MOC blueprint community.",
		Canonical:   public + "/en/about",
		OGType:      "website",
		JSONLD:      jsonld,
		Noscript:    aboutNoscriptFor("en"),
	})
}

// FAQEN renders the English FAQ page at /en/faq.
func (h *Handler) FAQEN(c *gin.Context) {
	qa := faqDataFor("en")
	jsonld := h.siteJSONLD()
	// withInLanguage is the same helper the blog tree uses for its FAQPage block:
	// it injects the page language into an already-serialized node.
	jsonld = append(jsonld, withInLanguage(faqJSONLD(qa), "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "FAQ — BrickPlan",
		Description: "Frequently asked questions about BrickPlan: what a MOC is, how to upload, supported file types, copyright and reporting.",
		Canonical:   h.cfg.PublicURL + "/en/faq",
		OGType:      "website",
		JSONLD:      jsonld,
		Noscript:    faqNoscript(qa),
	})
}

// PrivacyEN renders the English privacy page at /en/privacy.
func (h *Handler) PrivacyEN(c *gin.Context) {
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, webPageJSONLD(h.cfg.PublicURL, "/en/privacy",
		"Privacy Policy", "What BrickPlan stores, why, and what you can do about it.", "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "Privacy Policy — BrickPlan",
		Description: "What BrickPlan stores, why we store it, which third parties are involved, and how to have your data removed.",
		Canonical:   h.cfg.PublicURL + "/en/privacy",
		OGType:      "website",
		JSONLD:      jsonld,
		Noscript:    privacyBodyEN(),
	})
}

// ExploreEN renders the English browse shell at /en/explore.
func (h *Handler) ExploreEN(c *gin.Context) {
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, webPageJSONLD(h.cfg.PublicURL, "/en/explore",
		"Explore MOC blueprints", "Browse community brick blueprints by category, tag and keyword.", "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "Explore MOC blueprints — BrickPlan",
		Description: "Browse community brick blueprints — architecture, vehicles, mecha, fantasy, sci-fi and scenery — by category, tag or keyword.",
		Canonical:   h.cfg.PublicURL + "/en/explore",
		OGType:      "website",
		JSONLD:      jsonld,
		Noscript:    exploreBodyEN(),
	})
}
