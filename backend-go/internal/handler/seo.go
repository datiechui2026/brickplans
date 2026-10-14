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
}

func NewSEOHandler(cfg *config.Config, gdb *gorm.DB, blogStore *blog.Store) *SEOHandler {
	return &SEOHandler{cfg: cfg, gdb: gdb, blogStore: blogStore}
}

// Child sitemap paths. The root /sitemap.xml is a sitemap index that points at
// these; each child is a plain <urlset>.
//
// Why split: the single 654 KB sitemap mixed static pages, ~1900 blueprint
// detail pages and ~1500 blog posts into one file, so Search Console could only
// report one aggregate coverage number and a single parse error would invalidate
// every URL at once. Splitting by topic gives a per-topic coverage report and
// keeps each file small. (Bing/IndexNow consumption is unaffected: the
// collector follows the index recursively — see
// /home/ubuntu/.hermes/scripts/brickplan_indexnow_urls.sh.)
const (
	sitemapPagesPath      = "/sitemap-pages.xml"
	sitemapBlueprintsPath = "/sitemap-blueprints.xml"
	sitemapBlogPrefix     = "/sitemap-blog-"
	sitemapBlogSuffix     = ".xml"
)

// RegisterRoutes mounts the sitemaps at the site root (not under /api).
//
// The blog child routes are registered statically per bucket instead of as
// "/sitemap-blog-:bucket.xml": with gin/httprouter a named parameter swallows
// the rest of its path segment, so ":bucket.xml" would be the parameter *name*
// (c.Param("bucket.xml")), not "bucket" + suffix. One handler per known slug
// removes that class of bug entirely, and adding a bucket is a table edit in
// internal/blog/categories.go — the index and the routes both iterate that
// table, so they cannot drift apart.
func (h *SEOHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/sitemap.xml", h.sitemapIndex)
	r.GET(sitemapPagesPath, h.sitemapPages)
	r.GET(sitemapBlueprintsPath, h.sitemapBlueprints)
	for _, slug := range blog.BucketSlugs() {
		slug := slug
		r.GET(sitemapBlogPrefix+slug+sitemapBlogSuffix, func(c *gin.Context) {
			h.sitemapBlogBucket(c, slug)
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

func writeURL(b *strings.Builder, loc, lastmod, prio, freq string) {
	b.WriteString("  <url>\n    <loc>" + xmlEscape(loc) + "</loc>\n")
	if lastmod != "" {
		b.WriteString("    <lastmod>" + lastmod + "</lastmod>\n")
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

// ── Root: sitemap index ──

func (h *SEOHandler) sitemapIndex(c *gin.Context) {
	base := h.cfg.PublicURL
	contentDay := h.latestContentDay()

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	writeSitemapRef(&b, base+sitemapPagesPath, contentDay)
	writeSitemapRef(&b, base+sitemapBlueprintsPath, h.latestBlueprintDay())
	for _, st := range h.bucketStats() {
		writeSitemapRef(&b, base+sitemapBlogPrefix+st.Slug+sitemapBlogSuffix, st.LatestDay)
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

// ── Child: static pages, category pages, tag pages, blog listing ──

func (h *SEOHandler) sitemapPages(c *gin.Context) {
	base := h.cfg.PublicURL
	contentDay := h.latestContentDay()

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	// Static pages. No lastmod: these are stable, and an invented lastmod is
	// what makes Google start ignoring the field for the whole file.
	writeURL(&b, base+"/", "", "1.0", "daily")
	writeURL(&b, base+"/explore", contentDay, "0.9", "daily")
	writeURL(&b, base+"/faq", "", "0.6", "weekly")
	writeURL(&b, base+"/about", "", "0.6", "monthly")
	writeURL(&b, base+"/privacy", "", "0.3", "yearly")

	// Category pages
	for _, cat := range []string{"建筑", "车辆", "机甲", "奇幻", "科幻", "场景"} {
		writeURL(&b, base+"/explore?category="+url.QueryEscape(cat), contentDay, "0.7", "weekly")
	}

	// Tag pages
	var tags []db.Tag
	h.gdb.Order("name ASC").Find(&tags)
	for _, t := range tags {
		writeURL(&b, base+"/tags/"+url.PathEscape(t.Name), contentDay, "0.6", "weekly")
	}

	// Blog listing + every pagination page. These are real crawl targets: each
	// pagination page is self-canonical and links to the next one, so listing
	// them here is what lets a crawler walk the whole archive.
	writeURL(&b, base+"/blog", contentDay, "0.9", "daily")
	if h.blogStore != nil {
		total := len(h.blogStore.All())
		if pages := blog.PageCount(total, blog.SSRPageSize); pages > 1 {
			for p := 2; p <= pages; p++ {
				writeURL(&b, base+"/blog?page="+strconv.Itoa(p), contentDay, "0.5", "weekly")
			}
		}
	}

	b.WriteString("</urlset>\n")
	serveXML(c, b.String())
}

// ── Child: blueprint detail pages ──

func (h *SEOHandler) sitemapBlueprints(c *gin.Context) {
	base := h.cfg.PublicURL

	var bps []db.Blueprint
	h.gdb.Where("is_published = ?", true).Order("updated_at DESC").Find(&bps)

	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, bp := range bps {
		writeURL(&b, base+"/detail/"+bp.ID, bp.UpdatedAt.Format("2006-01-02"), "0.8", "weekly")
	}
	b.WriteString("</urlset>\n")
	serveXML(c, b.String())
}

// ── Child: blog posts of one category bucket ──

func (h *SEOHandler) sitemapBlogBucket(c *gin.Context, slug string) {
	// Slug comes from the route table, so it is always a known bucket; the
	// check is here to fail loudly if a route and the bucket table ever drift.
	if blog.BucketLabel(slug) == "" {
		c.Status(http.StatusNotFound)
		return
	}

	base := h.cfg.PublicURL
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	if h.blogStore != nil {
		for _, post := range h.blogStore.PostsInBucket(slug) {
			writeURL(&b, base+"/blog/"+post.Slug, post.Date.Format("2006-01-02"), "0.8", "weekly")
		}
	}
	b.WriteString("</urlset>\n")
	serveXML(c, b.String())
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

// latestContentDay is the newest change across blueprints and blog posts — the
// honest lastmod for listing pages (/explore, /blog, tag pages), which change
// whenever any content changes.
func (h *SEOHandler) latestContentDay() string {
	day := h.latestBlueprintDay()
	if h.blogStore != nil {
		for _, p := range h.blogStore.All() {
			if d := p.Date.Format("2006-01-02"); d > day {
				day = d
			}
		}
	}
	return day
}

// bucketStats returns per-bucket post counts/newest dates, or the table defaults
// when no blog store is loaded.
func (h *SEOHandler) bucketStats() []blog.BucketStat {
	if h.blogStore == nil {
		stats := make([]blog.BucketStat, 0, len(blog.BucketSlugs()))
		for _, slug := range blog.BucketSlugs() {
			stats = append(stats, blog.BucketStat{Slug: slug, Label: blog.BucketLabel(slug)})
		}
		return stats
	}
	return h.blogStore.Stats()
}
