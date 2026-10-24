package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/blog"
	"brickplans/internal/config"
	"brickplans/internal/db"
)

type SEOHandler struct {
	cfg       *config.Config
	gdb       *gorm.DB
	blogStore *blog.Store
	exposure  *blog.ExposureLedger
}

func NewSEOHandler(cfg *config.Config, gdb *gorm.DB, blogStore *blog.Store) *SEOHandler {
	return &SEOHandler{cfg: cfg, gdb: gdb, blogStore: blogStore}
}

// WithExposureLedger installs the English article exposure gate (see
// blog.ExposureLedger). A nil ledger — or a ledger whose file is missing —
// advertises every English post, which is the pre-2026-10-24 behaviour.
//
// Only the English article sitemaps are gated. The Chinese tree has its own
// indexable history and must not be affected by an English rollout knob.
func (h *SEOHandler) WithExposureLedger(l *blog.ExposureLedger) *SEOHandler {
	h.exposure = l
	return h
}

// enExposed reports whether an English post may be advertised in
// sitemap-en-blog-*.xml. Nil-safe: no ledger means "yes".
func (h *SEOHandler) enExposed(slug string) bool {
	return h.exposure.Allows(slug)
}

// Sitemap layout (v1.0.31 — language × category):
//
//	/sitemap.xml                     sitemapindex
//	├── /sitemap-zh.xml              中文站资源：静态页/分类页/标签页/博客列表+分页/图纸详情
//	├── /sitemap-zh-blog-<bucket>.xml ×10   中文博客文章，按规范分类分桶
//	├── /sitemap-en.xml              英文站资源：/en/blog 列表 + 分页
//	└── /sitemap-en-blog-<bucket>.xml ×N    英文博客文章（仅非空桶才进 index）
//
// Why two axes and not one flat file: the 2026-10-14 split existed so Search
// Console could report coverage per topic; the multilingual rollout adds a
// second axis (language) because English and Chinese pages index at wildly
// different rates and a merged number hides which tree is the problem. Keeping
// the axes as *separate files* (rather than one file per language) preserves
// both reports and keeps every file small.
//
// Invariant: a URL appears in exactly one child sitemap. That is why the
// language hubs hold the non-article resources and the article files hold the
// posts — merging them would double-submit ~1500 URLs.
//
// Every article entry carries xhtml:link hreflang alternates, but only when the
// counterpart really exists in both trees (see writeURLAlternates).
const (
	sitemapZHPath     = "/sitemap-zh.xml"
	sitemapENPath     = "/sitemap-en.xml"
	sitemapBlogPrefix = "/sitemap-"
	sitemapBlogSuffix = ".xml"
	xhtmlNS           = `xmlns:xhtml="http://www.w3.org/1999/xhtml"`
)

// blogBucketPath builds e.g. "/sitemap-zh-blog-moc-review.xml".
func blogBucketPath(lang, bucket string) string {
	return sitemapBlogPrefix + lang + "-blog-" + bucket + sitemapBlogSuffix
}

// RegisterRoutes mounts the sitemaps at the site root (not under /api).
//
// The per-bucket routes are registered statically instead of as a path
// parameter: with gin/httprouter a named parameter swallows the rest of its
// path segment, so ":bucket.xml" would be the parameter *name*
// (c.Param("bucket.xml")), not "bucket" + suffix. One handler per known slug
// removes that class of bug entirely, and adding a bucket is a table edit in
// internal/blog/categories.go — the index and the routes both iterate that
// table, so they cannot drift apart.
func (h *SEOHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/sitemap.xml", h.sitemapIndex)
	r.GET(sitemapZHPath, func(c *gin.Context) { h.sitemapSite(c, blog.LangZH) })
	r.GET(sitemapENPath, func(c *gin.Context) { h.sitemapSite(c, blog.LangEN) })
	for _, slug := range blog.BucketSlugs() {
		slug := slug
		r.GET(blogBucketPath(blog.LangZH, slug), func(c *gin.Context) {
			h.sitemapBlogBucket(c, blog.LangZH, slug)
		})
		r.GET(blogBucketPath(blog.LangEN, slug), func(c *gin.Context) {
			h.sitemapBlogBucket(c, blog.LangEN, slug)
		})
	}
}

// ── XML helpers ──

const xmlDecl = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// xmlEscape escapes the five XML predefined entities. Query strings (&) in
// <loc> MUST be escaped or the document is not well-formed XML and every engine
// rejects the whole file.
var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

func xmlEscape(s string) string { return xmlEscaper.Replace(s) }

type altLink struct {
	Hreflang string
	Href     string
}

func writeURL(b *strings.Builder, loc, lastmod, prio, freq string, alts ...altLink) {
	b.WriteString("  <url>\n    <loc>" + xmlEscape(loc) + "</loc>\n")
	if lastmod != "" {
		b.WriteString("    <lastmod>" + lastmod + "</lastmod>\n")
	}
	for _, a := range alts {
		b.WriteString("    <xhtml:link rel=\"alternate\" hreflang=\"" + a.Hreflang + "\" href=\"" + xmlEscape(a.Href) + "\"/>\n")
	}
	if freq != "" {
		b.WriteString("    <changefreq>" + freq + "</changefreq>\n")
	}
	if prio != "" {
		b.WriteString("    <priority>" + prio + "</priority>\n")
	}
	b.WriteString("  </url>\n")
}

// serveXML writes an XML body with the content type nginx is configured to gzip.
func serveXML(c *gin.Context, body string) {
	c.Data(http.StatusOK, "application/xml", []byte(body))
}

// openURLSet writes the declaration + <urlset> with the xhtml namespace needed
// for hreflang annotations.
func openURLSet(b *strings.Builder) {
	b.WriteString(xmlDecl)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" ` + xhtmlNS + `>` + "\n")
}

// ── Root: sitemap index ──

func (h *SEOHandler) sitemapIndex(c *gin.Context) {
	base := h.cfg.PublicURL

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	writeSitemapRef(&b, base+sitemapZHPath, h.latestContentDay(blog.LangZH))
	for _, st := range h.bucketStats(blog.LangZH) {
		writeSitemapRef(&b, base+blogBucketPath(blog.LangZH, st.Slug), st.LatestDay)
	}
	writeSitemapRef(&b, base+sitemapENPath, h.latestContentDay(blog.LangEN))
	// English buckets appear only once they hold at least one *exposed* post: an
	// empty <urlset> is valid but shows up in Search Console as "0 discovered
	// pages", which reads like a bug rather than "this topic has not been
	// released yet". The counts therefore come from the exposure gate, not from
	// the raw corpus — otherwise a freshly-seeded ledger would advertise eight
	// empty child sitemaps on day one.
	for _, st := range h.enBucketExposedStats() {
		writeSitemapRef(&b, base+blogBucketPath(blog.LangEN, st.Slug), st.LatestDay)
	}

	b.WriteString("</sitemapindex>\n")
	serveXML(c, b.String())
}

// writeSitemapRef appends one <sitemap> entry. lastmod may be empty (unknown),
// which is honest for sitemaps whose newest content date we cannot derive.
func writeSitemapRef(b *strings.Builder, loc, lastmod string) {
	b.WriteString("  <sitemap>\n    <loc>" + xmlEscape(loc) + "</loc>\n")
	if lastmod != "" {
		b.WriteString("    <lastmod>" + lastmod + "</lastmod>\n")
	}
	b.WriteString("  </sitemap>\n")
}

// ── Child: per-language site resources (pages + blueprint details) ──

func (h *SEOHandler) sitemapSite(c *gin.Context, lang string) {
	base := h.cfg.PublicURL
	contentDay := h.latestContentDay(lang)

	var b strings.Builder
	openURLSet(&b)

	if lang == blog.LangZH {
		// Static pages. No lastmod: these are stable, and an invented lastmod is
		// what makes Google start ignoring the field for the whole file.
		writeURL(&b, base+"/", "", "1.0", "daily")
		writeURL(&b, base+"/explore", contentDay, "0.9", "daily")
		writeURL(&b, base+"/faq", "", "0.6", "weekly")
		writeURL(&b, base+"/about", "", "0.6", "monthly")
		writeURL(&b, base+"/privacy", "", "0.3", "yearly")

		// Category pages. Derived from what authors actually used instead of a
		// hardcoded taxonomy: the old list (车辆/机甲/奇幻/科幻 …) matched zero
		// rows while the real values authors pick (星战/城市/朋友/图标/机械/创意/
		// 赛车/其他) were never advertised, so those filters were reachable only
		// through the UI.
		var cats []string
		h.gdb.Model(&db.Blueprint{}).
			Where("is_published = ? AND category IS NOT NULL AND category <> ''", true).
			Distinct().Order("category ASC").Pluck("category", &cats)
		for _, cat := range cats {
			writeURL(&b, base+"/explore?category="+url.QueryEscape(cat), contentDay, "0.7", "weekly")
		}

		// Tag pages
		var tags []db.Tag
		h.gdb.Order("name ASC").Find(&tags)
		for _, t := range tags {
			writeURL(&b, base+"/tags/"+url.PathEscape(t.Name), contentDay, "0.6", "weekly")
		}
	}

	// Blueprint detail pages: CN hub lists /detail/... (priority 0.8).
	var bps []db.Blueprint
	h.gdb.Where("is_published = ?", true).Order("updated_at DESC").Find(&bps)
	for _, bp := range bps {
		writeURL(&b, base+"/detail/"+bp.ID, bp.UpdatedAt.Format("2006-01-02"), "0.8", "weekly")
	}

	if lang == blog.LangEN {
		// P0-2: /en/detail/{uuid} is a real, crawlable page (EN chrome, /en/detail
		// JSON-LD), so it now belongs in the English hub sitemap — otherwise it
		// is only discoverable by crawling the EN tree. The CN hub above already
		// lists the same record, so the two hubs form a valid hreflang cluster.
		//
		// Only translated rows are advertised (v1.1.1): an untranslated row's
		// /en/detail page still canonicalises to /detail/{uuid}, so submitting it
		// here would feed Google a URL that folds into another one. Keep this
		// predicate identical to ssr.blueprintHasEnglish.
		for _, bp := range bps {
			if bp.TitleEN == nil || strings.TrimSpace(*bp.TitleEN) == "" {
				continue
			}
			writeURL(&b, base+"/en/detail/"+bp.ID, bp.UpdatedAt.Format("2006-01-02"), "0.7", "weekly",
				altLink{"zh-CN", base + "/detail/" + bp.ID},
				altLink{"en", base + "/en/detail/" + bp.ID},
				altLink{"x-default", base + "/en/detail/" + bp.ID})
		}
	}

	// Blog listing + every pagination page of THIS language. These are real
	// crawl targets: each pagination page is self-canonical and links to the
	// next one, so listing them here is what lets a crawler walk the archive.
	if lang == blog.LangEN {
		// English static pages (the /en/ home and its mirrors). Without these the
		// English tree would only advertise its blog, leaving the shell pages
		// discoverable solely through the nav.
		for _, sp := range staticLangPaths {
			writeURL(&b, base+"/en"+sp.Path, contentDay, sp.Prio, sp.Freq, staticAlts(sp.Path, base)...)
		}
	}

	if h.blogStore != nil {
		listPath := "/blog"
		if lang == blog.LangEN {
			listPath = "/en/blog"
		}
		writeURL(&b, base+listPath, contentDay, "0.9", "daily")
		total := h.blogStore.CountByLang(lang)
		if pages := blog.PageCount(total, blog.SSRPageSize); pages > 1 {
			for p := 2; p <= pages; p++ {
				writeURL(&b, base+listPath+"?page="+strconv.Itoa(p), contentDay, "0.5", "weekly")
			}
		}
	}

	b.WriteString("</urlset>\n")
	serveXML(c, b.String())
}

// ── Child: blog posts of one (language, bucket) ──

func (h *SEOHandler) sitemapBlogBucket(c *gin.Context, lang, slug string) {
	// Slug comes from the route table, so it is always a known bucket; the
	// check is here to fail loudly if a route and the bucket table ever drift.
	if blog.BucketLabel(slug) == "" || (lang != blog.LangZH && lang != blog.LangEN) {
		c.Status(http.StatusNotFound)
		return
	}

	base := h.cfg.PublicURL
	var b strings.Builder
	openURLSet(&b)
	if h.blogStore != nil {
		for _, post := range h.blogStore.PostsInBucketLang(lang, slug) {
			// The exposure gate applies to the English tree only: 1,537 EN
			// translations are released 100/day (see blog.ExposureLedger), while
			// the Chinese corpus is fully indexed and must not change shape.
			if lang == blog.LangEN && !h.enExposed(post.Slug) {
				continue
			}
			writeURL(&b, base+blogDetailPath(lang, post.Slug), post.Date.Format("2006-01-02"), "0.8", "weekly",
				h.postAlternates(lang, post.Slug)...)
		}
	}
	b.WriteString("</urlset>\n")
	serveXML(c, b.String())
}

// blogDetailPath mirrors the SSR layer's URL shape. It is duplicated here
// deliberately: handler must not import ssr (ssr imports handler's siblings and
// the dependency would be circular), and the test suite asserts both trees
// produce the same URL for the same (lang, slug).
func blogDetailPath(lang, slug string) string {
	if lang == blog.LangEN {
		return "/en/blog/" + slug
	}
	return "/blog/" + slug
}

// staticLangPaths are the language-neutral paths that exist in BOTH trees, and
// the priority/changefreq each carries in a language hub sitemap.
//
// Duplicated from internal/ssr's lang table on purpose: handler must not import
// ssr (circular — see blogDetailPath above). Keep the two lists in sync when a
// language is added; sitemap_test asserts the English hub contains all of them.
var staticLangPaths = []struct {
	Path string
	Prio string
	Freq string
}{
	{"/", "1.0", "daily"},
	{"/explore", "0.9", "daily"},
	{"/about", "0.6", "monthly"},
	{"/faq", "0.6", "weekly"},
	{"/privacy", "0.3", "yearly"},
}

// staticAlts is the hreflang cluster for a language-neutral static path. The
// x-default follows the blog tree's existing choice (English).
func staticAlts(neutral, base string) []altLink {
	return []altLink{
		{"zh-CN", base + neutral},
		{"en", base + "/en" + neutral},
		{"x-default", base + "/en" + neutral},
	}
}

// postAlternates returns the hreflang annotations for one article — and nothing
// at all when only one language version exists. An hreflang pointing at a page
// that serves the *other* language's text makes Google discard the annotation
// for the whole cluster, so a missing translation must stay silent.
func (h *SEOHandler) postAlternates(lang, slug string) []altLink {
	if h.blogStore == nil {
		return nil
	}
	hasZH := h.blogStore.HasLang(slug, blog.LangZH)
	hasEN := h.blogStore.HasLang(slug, blog.LangEN)
	if !hasZH || !hasEN {
		return nil
	}
	base := h.cfg.PublicURL
	// Same gate as ssr.blogDetailAlternates: an English translation the
	// exposure ledger has not released answers noindex, and an <xhtml:link>
	// pointing at a noindex URL gets the whole cluster discarded as
	// "Alternate page with noindex". Keep the Chinese entry with its own
	// hreflang only — no en, no x-default pointing at an unindexable page.
	if !h.enExposed(slug) {
		return []altLink{{hreflangOf(lang), base + blogDetailPath(lang, slug)}}
	}
	return []altLink{
		{"zh-CN", base + blogDetailPath(blog.LangZH, slug)},
		{"en", base + blogDetailPath(blog.LangEN, slug)},
		{"x-default", base + blogDetailPath(blog.LangEN, slug)},
	}
}

// hreflangOf maps a content language to the hreflang value used in annotations.
func hreflangOf(lang string) string {
	if lang == blog.LangEN {
		return "en"
	}
	return "zh-CN"
}

// ── lastmod derivation ──

// latestBlueprintDay returns the newest published-blueprint update date, or "".
func (h *SEOHandler) latestBlueprintDay() string {
	var bp db.Blueprint
	if err := h.gdb.Where("is_published = ?", true).Order("updated_at DESC").First(&bp).Error; err != nil {
		return ""
	}
	return bp.UpdatedAt.Format("2006-01-02")
}

// latestContentDay is the newest change across blueprints and the blog posts of
// one language — the honest lastmod for listing pages (/explore, /blog,
// /en/blog, tag pages), which change whenever any of their content changes.
func (h *SEOHandler) latestContentDay(lang string) string {
	day := ""
	if lang == blog.LangZH {
		day = h.latestBlueprintDay()
	}
	if h.blogStore != nil {
		for _, p := range h.blogStore.AllByLang(lang) {
			if d := p.Date.Format("2006-01-02"); d > day {
				day = d
			}
		}
	}
	return day
}

// bucketStats returns per-bucket post counts/newest dates for one language, or
// the table defaults when no blog store is loaded.
func (h *SEOHandler) bucketStats(lang string) []blog.BucketStat {
	if h.blogStore == nil {
		stats := make([]blog.BucketStat, 0, len(blog.BucketSlugs()))
		for _, slug := range blog.BucketSlugs() {
			stats = append(stats, blog.BucketStat{Slug: slug, Label: blog.BucketLabel(slug)})
		}
		return stats
	}
	return h.blogStore.StatsLang(lang)
}

// enBucketExposedStats is bucketStats(EN) recomputed through the exposure gate:
// counts and newest dates reflect only the posts the ledger currently exposes,
// so a bucket with nothing released yet is omitted from the sitemap index
// instead of advertising an empty child sitemap.
//
// When no ledger is active this returns exactly bucketStats(EN), which keeps the
// pre-rollout output byte-identical.
func (h *SEOHandler) enBucketExposedStats() []blog.BucketStat {
	if h.blogStore == nil || !h.exposure.Active() {
		out := h.bucketStats(blog.LangEN)
		if h.blogStore == nil {
			return out
		}
		filtered := out[:0]
		for _, st := range out {
			if st.Count > 0 {
				filtered = append(filtered, st)
			}
		}
		return filtered
	}

	raw := h.bucketStats(blog.LangEN)
	out := make([]blog.BucketStat, 0, len(raw))
	for _, st := range raw {
		count := 0
		latest := ""
		for _, p := range h.blogStore.PostsInBucketLang(blog.LangEN, st.Slug) {
			if !h.enExposed(p.Slug) {
				continue
			}
			count++
			if d := p.Date.Format("2006-01-02"); d > latest {
				latest = d
			}
		}
		if count == 0 {
			continue
		}
		out = append(out, blog.BucketStat{Slug: st.Slug, Label: st.Label, Count: count, LatestDay: latest})
	}
	return out
}
