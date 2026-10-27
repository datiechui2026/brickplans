package handler

import (
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/blog"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/taxonomy"
)

type SEOHandler struct {
	cfg       *config.Config
	gdb       *gorm.DB
	blogStore *blog.Store
	exposure  *blog.ExposureLedger

	// Snapshot of every sitemap. Requests are served from here (see
	// sitemap_cache.go), so nothing on the request path touches the database.
	cache *sitemapCache

	// buildMu serialises renders so a burst of requests cannot run the
	// blueprint query N times in parallel.
	buildMu sync.Mutex
	// ledgerVersion is the exposure-ledger version the current snapshot was
	// rendered with; renderedDirty is the sitemapDirty counter it was rendered
	// at; built says whether a snapshot exists yet. All three are read on the
	// request path, hence atomic.
	ledgerVersion atomic.Uint64
	renderedDirty atomic.Int64
	built         atomic.Bool
}

func NewSEOHandler(cfg *config.Config, gdb *gorm.DB, blogStore *blog.Store) *SEOHandler {
	return &SEOHandler{
		cfg:       cfg,
		gdb:       gdb,
		blogStore: blogStore,
		cache:     newSitemapCache(cfg.SitemapDir),
	}
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

// Sitemap layout (v1.0.31 — language × category, v1.1.2 — static snapshot):
//
//	/sitemap.xml                     sitemapindex
//	├── /sitemap-zh.xml              中文站资源：静态页/分类页/标签页/博客列表+分页/图纸详情
//	├── /sitemap-zh-blog-<bucket>.xml ×10   中文博客文章，按规范分类分桶
//	├── /sitemap-en.xml              英文站资源：/en 静态页 + /en/detail + /en/blog 列表+分页
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
// posts — merging them would double-submit ~1500 URLs. The corollary, fixed
// 2026-10-25 (P0-1): a hub may only advertise URLs under its own language
// prefix, so /sitemap-en.xml carries no /detail/{id} entries. Those are Chinese
// pages, already listed exactly once by /sitemap-zh.xml; the English hub lists
// their /en/detail/{id} counterparts instead.
//
// Every article entry carries xhtml:link hreflang alternates, but only when the
// counterpart really exists in both trees (see postAlternates).
const (
	sitemapIndexPath  = "/sitemap.xml"
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

// RegisterRoutes mounts the sitemaps at the site root (not under /api), renders
// the first snapshot, and starts the refresh ticker.
//
// The per-bucket routes are registered statically instead of as a path
// parameter: with gin/httprouter a named parameter swallows the rest of its
// path segment, so ":bucket.xml" would be the parameter *name*
// (c.Param("bucket.xml")), not "bucket" + suffix. One handler per known slug
// removes that class of bug entirely, and adding a bucket is a table edit in
// internal/blog/categories.go — the index and the routes both iterate that
// table, so they cannot drift apart.
func (h *SEOHandler) RegisterRoutes(r *gin.Engine) {
	serve := func(path string) gin.HandlerFunc {
		return func(c *gin.Context) { h.serveSitemap(c, path) }
	}
	r.GET(sitemapIndexPath, serve(sitemapIndexPath))
	r.GET(sitemapZHPath, serve(sitemapZHPath))
	r.GET(sitemapENPath, serve(sitemapENPath))
	for _, slug := range blog.BucketSlugs() {
		r.GET(blogBucketPath(blog.LangZH, slug), serve(blogBucketPath(blog.LangZH, slug)))
		r.GET(blogBucketPath(blog.LangEN, slug), serve(blogBucketPath(blog.LangEN, slug)))
	}

	// Render before the listener accepts traffic: an empty cache answers 503,
	// and a crawler that lands in that gap records it as a fetch error.
	h.Rebuild()
	h.startSitemapRefresher()
}

// serveSitemap answers one sitemap from the snapshot.
//
// Two cheap checks run first, both a single atomic load / stat in the common
// case: the exposure ledger's version (append a slug → next request sees it, no
// restart) and the write counter bumped by blueprint create/update/delete
// (publish a blueprint → next request sees it). Everything else — new blog
// posts, which are files loaded at startup — arrives with the ticker, at most
// SitemapRefreshMin late.
func (h *SEOHandler) serveSitemap(c *gin.Context, path string) {
	h.rebuildIfStale()
	h.cache.serve(c, strings.TrimPrefix(path, "/"))
}

// rebuildIfStale re-renders the snapshot when the exposure ledger or the
// blueprint set moved since the last build.
func (h *SEOHandler) rebuildIfStale() {
	ledger, dirty := h.exposure.Version(), sitemapDirty.Load()
	if h.built.Load() && ledger == h.ledgerVersion.Load() && dirty == h.renderedDirty.Load() {
		return
	}
	h.buildMu.Lock()
	defer h.buildMu.Unlock()
	// Re-read under the lock: whoever queued behind us may have rendered this
	// exact state already.
	ledger, dirty = h.exposure.Version(), sitemapDirty.Load()
	if h.built.Load() && ledger == h.ledgerVersion.Load() && dirty == h.renderedDirty.Load() {
		return
	}
	h.renderSnapshot(ledger, dirty)
}

// Rebuild re-renders every sitemap unconditionally. Called once at startup and
// by the refresh ticker: changes the two counters cannot see (a blog file edited
// in place, a row updated by a script) still need a periodic full build.
func (h *SEOHandler) Rebuild() {
	h.buildMu.Lock()
	defer h.buildMu.Unlock()
	h.renderSnapshot(h.exposure.Version(), sitemapDirty.Load())
}

// startSitemapRefresher rebuilds the snapshot every SitemapRefreshMin minutes so
// a blueprint published just after a build is advertised within the hour.
//
// Only runs when a snapshot directory is configured: with SitemapDir empty
// (tests) the cache is in-memory, a background ticker would be pure noise, and
// a test engine must not leak goroutines.
func (h *SEOHandler) startSitemapRefresher() {
	if h.cache.dir == "" || h.cfg.SitemapRefreshMin <= 0 {
		return
	}
	interval := time.Duration(h.cfg.SitemapRefreshMin) * time.Minute
	log.Printf("[sitemap] snapshot dir=%s refresh=%s", h.cache.dir, interval)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			h.Rebuild()
		}
	}()
}

// renderSnapshot renders every child sitemap, then the index. Caller holds
// buildMu. version/dirty are the counter values this snapshot corresponds to.
//
// Children come first because each index entry advertises its child's
// content-change time (the snapshot's mtime, see sitemapCache.put) instead of a
// hand-maintained date that drifts from the file it describes.
func (h *SEOHandler) renderSnapshot(version uint64, dirty int64) {
	// Counters are read before rendering: if either moves while we render, the
	// next request sees a mismatch and re-renders instead of keeping a half-old
	// view.

	// Every bucket of both languages is rendered even when it holds nothing, so
	// a direct hit on a submitted child keeps answering 200 with a valid (empty)
	// <urlset> instead of 503. That was the pre-snapshot behaviour, and Search
	// Console has those URLs on file.
	for _, slug := range blog.BucketSlugs() {
		h.cache.put(strings.TrimPrefix(blogBucketPath(blog.LangZH, slug), "/"),
			[]byte(h.renderSitemapBlogBucket(blog.LangZH, slug)))
		h.cache.put(strings.TrimPrefix(blogBucketPath(blog.LangEN, slug), "/"),
			[]byte(h.renderSitemapBlogBucket(blog.LangEN, slug)))
	}
	h.cache.put(strings.TrimPrefix(sitemapZHPath, "/"), []byte(h.renderSitemapSite(blog.LangZH)))
	h.cache.put(strings.TrimPrefix(sitemapENPath, "/"), []byte(h.renderSitemapSite(blog.LangEN)))

	base := h.cfg.PublicURL
	refs := make([]sitemapRef, 0, 2+2*len(blog.BucketSlugs()))
	addRef := func(path string) {
		if doc := h.cache.get(strings.TrimPrefix(path, "/")); doc != nil {
			refs = append(refs, sitemapRef{loc: base + path, mod: doc.mod})
		}
	}

	addRef(sitemapZHPath)
	for _, st := range h.bucketStats(blog.LangZH) {
		addRef(blogBucketPath(blog.LangZH, st.Slug))
	}
	addRef(sitemapENPath)
	// English buckets appear only once they hold at least one *exposed* post: an
	// empty <urlset> is valid but shows up in Search Console as "0 discovered
	// pages", which reads like a bug rather than "this topic has not been
	// released yet". The counts therefore come from the exposure gate, not from
	// the raw corpus — otherwise a freshly-seeded ledger would advertise eight
	// empty child sitemaps on day one.
	for _, st := range h.enBucketExposedStats() {
		addRef(blogBucketPath(blog.LangEN, st.Slug))
	}

	h.cache.put(strings.TrimPrefix(sitemapIndexPath, "/"), []byte(h.renderSitemapIndex(refs)))
	h.ledgerVersion.Store(version)
	h.renderedDirty.Store(dirty)
	h.built.Store(true)
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

// openURLSet writes the declaration + <urlset> with the xhtml namespace needed
// for hreflang annotations.
func openURLSet(b *strings.Builder) {
	b.WriteString(xmlDecl)
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" ` + xhtmlNS + `>` + "\n")
}

// ── Root: sitemap index ──

// sitemapRef is one index entry: the child's absolute URL and the time its
// content last changed (the child file's mtime; render time in memory-only mode).
type sitemapRef struct {
	loc string
	mod time.Time
}

func (h *SEOHandler) renderSitemapIndex(refs []sitemapRef) string {
	var b strings.Builder
	b.WriteString(xmlDecl)
	b.WriteString(`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, r := range refs {
		writeSitemapRef(&b, r.loc, r.mod)
	}
	b.WriteString("</sitemapindex>\n")
	return b.String()
}

// writeSitemapRef appends one <sitemap> entry.
//
// lastmod is a full W3C datetime, not a bare date: the index used to advertise
// a date carried over from the newest post, which had already drifted from the
// file it described (sitemap-en-blog-brand.xml claimed 2026-10-01 long after its
// content changed). A child that genuinely did not change keeps its old mtime,
// because the snapshot refuses to rewrite byte-identical content.
func writeSitemapRef(b *strings.Builder, loc string, mod time.Time) {
	b.WriteString("  <sitemap>\n    <loc>" + xmlEscape(loc) + "</loc>\n")
	if !mod.IsZero() {
		b.WriteString("    <lastmod>" + mod.UTC().Format("2006-01-02T15:04:05-07:00") + "</lastmod>\n")
	}
	b.WriteString("  </sitemap>\n")
}

// ── Child: per-language site resources (pages + blueprint details) ──

// writeAggregateURLs advertises the 13 aggregate landing pages of
// SEO-20261024 — six categories, two brands, five difficulty levels — in one
// language tree.
//
// Only page 1 is written. Every pagination page is self-canonical and linked
// from its predecessor, so a crawler walks them on its own; listing 17 pages per
// category here would spend the hub's crawl budget on list pages and bury the
// URLs that are supposed to rank.
func (h *SEOHandler) writeAggregateURLs(b *strings.Builder, lang string) {
	base := h.cfg.PublicURL
	contentDay := h.latestContentDay(lang)
	prefix := ""
	if lang == blog.LangEN {
		prefix = "/en"
	}
	// No xhtml:link block here on purpose. The pair is already declared in the
	// <head> of both pages (hreflang zh-CN / en / x-default), which is the form
	// Google documents as sufficient; repeating it in the sitemap adds 3 lines
	// per URL that then have to be kept in sync by hand, and it makes
	// "count the /category/ locs in sitemap-zh.xml" a misleading measurement.
	for _, e := range taxonomy.AggregateEntries {
		writeURL(b, base+prefix+taxonomy.Path(e), contentDay, "0.8", "weekly")
	}
}

// renderSitemapSite renders a language hub: static pages, taxonomy, that
// language's blueprint details and its blog listing (with pagination).
func (h *SEOHandler) renderSitemapSite(lang string) string {
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

		// Aggregate landing pages (SEO-20261024). This block used to advertise
		// every distinct category as /explore?category=<value> — 36 URLs that all
		// canonicalised back to /explore, so they competed with the page they
		// pointed at and could never rank. They are replaced by the 13 real
		// landing pages; the parameter URLs still work for the SPA but are no
		// longer advertised.
		h.writeAggregateURLs(&b, blog.LangZH)

		// Tag pages
		var tags []db.Tag
		h.gdb.Order("name ASC").Find(&tags)
		for _, t := range tags {
			writeURL(&b, base+"/tags/"+url.PathEscape(t.Name), contentDay, "0.6", "weekly")
		}
	}

	// Blueprint detail pages. The query feeds both hubs; which hub may advertise
	// which URL is a separate decision (below).
	var bps []db.Blueprint
	h.gdb.Where("is_published = ?", true).Order("updated_at DESC").Find(&bps)

	if lang == blog.LangZH {
		// P0-1 (2026-10-25): this loop used to run for BOTH languages, so
		// /sitemap-en.xml carried all 2,134 Chinese /detail/{id} URLs a second
		// time — GSC submitted 8,037 against 6,138 unique URLs, and the English
		// pages competed for crawl budget with duplicates of their own twins. A
		// hub may only advertise URLs under its own prefix; the Chinese tree is
		// unchanged and still lists every /detail/ URL exactly once.
		for _, bp := range bps {
			writeURL(&b, base+"/detail/"+bp.ID, bp.UpdatedAt.Format("2006-01-02"), "0.8", "weekly")
		}
	}

	if lang == blog.LangEN {
		// /en/detail/{uuid} is a real, crawlable page (EN chrome, /en/detail
		// JSON-LD), so it belongs in the English hub sitemap — otherwise it is
		// only discoverable by crawling the EN tree. The CN hub lists the same
		// record, so the two hubs form a valid hreflang cluster.
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

		// English static pages (the /en/ home and its mirrors). Without these the
		// English tree would only advertise its blog, leaving the shell pages
		// discoverable solely through the nav.
		for _, sp := range staticLangPaths {
			writeURL(&b, base+"/en"+sp.Path, contentDay, sp.Prio, sp.Freq, staticAlts(sp.Path, base)...)
		}

		// The English twins of the aggregate pages. Each one is a genuine
		// translation of its Chinese counterpart (English chrome + English
		// labels) and carries the same hreflang cluster, so it belongs in the
		// English hub rather than being reachable only by crawling.
		h.writeAggregateURLs(&b, blog.LangEN)
	}

	// Blog listing + every pagination page of THIS language. These are real
	// crawl targets: each pagination page is self-canonical and links to the
	// next one, so listing them here is what lets a crawler walk the archive.
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
	return b.String()
}

// ── Child: blog posts of one (language, bucket) ──

func (h *SEOHandler) renderSitemapBlogBucket(lang, slug string) string {
	var b strings.Builder
	openURLSet(&b)
	// The slug always comes from the bucket table; the label lookup is here so a
	// table edit that forgot a label renders an empty file instead of inventing a
	// bucket.
	if blog.BucketLabel(slug) == "" || (lang != blog.LangZH && lang != blog.LangEN) {
		b.WriteString("</urlset>\n")
		return b.String()
	}

	base := h.cfg.PublicURL
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
	return b.String()
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
