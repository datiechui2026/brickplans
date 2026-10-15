package ssr

import (
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
)

// Internal-link fan-out. relatedLimit is how many sibling articles an article
// links to; homeBlogLimit is how many the homepage links. Both are named so the
// crawl-path strength is a number someone can tune, not a magic literal.
const (
	relatedLimit  = 5
	homeBlogLimit = 12
)

// isoLang maps an internal tree code to the value used in hreflang / inLanguage.
func isoLang(lang string) string {
	if lang == blog.LangEN {
		return "en"
	}
	return "zh-CN"
}

// blogDetailPath / blogListPathLang are the single source of truth for the two
// language URL shapes. Everything (canonical, hreflang, noscript links, JSON-LD,
// pagination) must go through them or the two trees drift apart.
func blogDetailPath(lang, slug string) string {
	if lang == blog.LangEN {
		return "/en/blog/" + slug
	}
	return "/blog/" + slug
}

func blogListPathLang(lang, cat, tag string, page int) string {
	base := "/blog"
	if lang == blog.LangEN {
		base = "/en/blog"
	}
	q := url.Values{}
	if cat != "" {
		q.Set("category", cat)
	} else if tag != "" {
		q.Set("tag", tag)
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}

// BlogList renders the Chinese blog index; BlogListEN renders the English one.
// Both are the same renderer with a different language tree, so the two listings
// can never disagree about filtering or pagination rules.
func (h *Handler) BlogList(c *gin.Context)   { h.blogList(c, blog.LangZH) }
func (h *Handler) BlogListEN(c *gin.Context) { h.blogList(c, blog.LangEN) }

func (h *Handler) blogList(c *gin.Context, lang string) {
	cat := c.Query("category")
	tag := c.Query("tag")
	page := atoiOrSSR(c.Query("page"), 1)
	isZH := lang == blog.LangZH

	// Clamp page into range so /blog?page=999 returns the last page (HTTP 200)
	// instead of an empty document. An empty index page would be a soft 404 and
	// would drop the crawlable link chain for a crawler that followed a stale
	// pagination link.
	items, total := blog.Paginate(h.blogStore.FilterLang(lang, cat, tag), page, blog.SSRPageSize)
	totalPages := blog.PageCount(total, blog.SSRPageSize)
	if page > totalPages {
		page = totalPages
		items, total = blog.Paginate(h.blogStore.FilterLang(lang, cat, tag), page, blog.SSRPageSize)
	}

	catPrefix, tagPrefix := "分类：", "标签：#"
	if !isZH {
		catPrefix, tagPrefix = "Category: ", "Tag: #"
	}
	filterLabel := ""
	if cat != "" {
		filterLabel = catPrefix + cat
	} else if tag != "" {
		filterLabel = tagPrefix + tag
	}

	title := "博客 - BrickPlan 积木图纸社区"
	desc := "BrickPlan 博客：积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容。"
	if !isZH {
		title = "Blog - BrickPlan: MOC blueprints, building guides and brick reviews"
		desc = "Building guides, brick part knowledge, brand comparisons and MOC reviews from the BrickPlan community."
	}
	if filterLabel != "" {
		if isZH {
			title = filterLabel + " - BrickPlan 博客"
			desc = "BrickPlan 博客「" + filterLabel + "」分类下的文章。"
		} else {
			title = filterLabel + " - BrickPlan Blog"
			desc = "Articles filed under " + filterLabel + " on the BrickPlan blog."
		}
	}
	if page > 1 {
		if isZH {
			title = fmt.Sprintf("%s（第 %d 页 / 共 %d 页）", title, page, totalPages)
			desc = fmt.Sprintf("%s（第 %d 页，共 %d 篇）", desc, page, total)
		} else {
			title = fmt.Sprintf("%s (page %d of %d)", title, page, totalPages)
			desc = fmt.Sprintf("%s (page %d, %d articles)", desc, page, total)
		}
	}

	// Self-referencing canonical. Do NOT canonicalise page >= 2 back to /blog:
	// that makes search engines ignore every link on the page and severs the
	// crawl chain between pagination pages.
	canonical := h.cfg.PublicURL + blogListPathLang(lang, cat, tag, page)

	pd := PageData{
		Title:       title,
		Description: desc,
		Canonical:   canonical,
		OGType:      "website",
		JSONLD:      h.siteJSONLD(),
		Noscript:    h.blogListNoscript(lang, items, filterLabel, cat, tag, page, totalPages, total),
	}
	if !isZH {
		pd.Lang = blog.LangEN
		pd.HTMLLang = "en"
	}
	// The two listings always exist, so they are always a valid hreflang pair.
	enURL := h.cfg.PublicURL + blogListPathLang(blog.LangEN, cat, tag, page)
	zhURL := h.cfg.PublicURL + blogListPathLang(blog.LangZH, cat, tag, page)
	pd.Alternates = []AltLink{
		{"en", enURL},
		{"zh-CN", zhURL},
		{"x-default", enURL},
	}
	h.r.Render(c, pd)
}

// atoiOrSSR parses a query int, falling back to def on garbage and clamping to
// a minimum of 1.
func atoiOrSSR(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// blogListPath keeps the Chinese signature for existing callers.
//
// Uses url.Values so Chinese categories/tags are percent-encoded exactly like
// the SPA's URLSearchParams does.
func blogListPath(cat, tag string, page int) string {
	return blogListPathLang(blog.LangZH, cat, tag, page)
}

// ── Noscript rendering ──

// blogListNoscript renders the crawlable blog index.
//
// This content is injected straight into <div id="app"> with no <noscript>
// wrapper (see template.html), so it IS what non-JS crawlers (Baidu, GPTBot,
// ClaudeBot, PerplexityBot) see as the page. Two consequences drove the design:
//
//   - Only one page of posts is emitted. Rendering all 1500+ posts cost 274 KB
//     of HTML for every request; link equity was diluted to ~0.065 % per link.
//   - The pager therefore has to use REAL <a href> links, not a JS button, or a
//     non-JS crawler can no longer walk from page 1 to page 2.
//
// sitemap.xml independently lists every post, so discovery never depends on
// this page alone.
func (h *Handler) blogListNoscript(lang string, items []blog.Post, filterLabel, cat, tag string, page, totalPages, total int) template.HTML {
	isZH := lang == blog.LangZH
	var b strings.Builder
	if isZH {
		b.WriteString("<h1>BrickPlan 博客</h1>")
		b.WriteString("<p>积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容。</p>")
	} else {
		b.WriteString("<h1>BrickPlan Blog</h1>")
		b.WriteString("<p>Building guides, brick part knowledge, brand comparisons and MOC blueprint reviews.</p>")
	}
	if filterLabel != "" {
		if isZH {
			b.WriteString(fmt.Sprintf("<p>筛选：%s（共 %d 篇）</p>", esc(filterLabel), total))
		} else {
			b.WriteString(fmt.Sprintf("<p>Filter: %s (%d articles)</p>", esc(filterLabel), total))
		}
	}
	if totalPages > 1 {
		if isZH {
			b.WriteString(fmt.Sprintf("<p>第 %d / %d 页</p>", page, totalPages))
		} else {
			b.WriteString(fmt.Sprintf("<p>Page %d of %d</p>", page, totalPages))
		}
	}
	if len(items) == 0 {
		if isZH {
			b.WriteString("<p>暂无文章。</p>")
		} else {
			b.WriteString("<p>No articles yet. <a href=\"/blog\">Read the Chinese blog</a>.</p>")
		}
		return template.HTML(b.String())
	}
	b.WriteString("<ul>")
	for _, p := range items {
		dateStr := p.Date.Format("2006-01-02")
		b.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a> (%s) <span>%s</span></li>`,
			esc(h.cfg.PublicURL+blogDetailPath(lang, p.Slug)), esc(p.Title), dateStr, esc(p.Category)))
	}
	b.WriteString("</ul>")
	b.WriteString(h.blogPager(lang, cat, tag, page, totalPages))
	return template.HTML(b.String())
}

// blogPager renders prev / numbered / next pagination with real crawlable links.
// Returns "" when there is only one page.
func (h *Handler) blogPager(lang, cat, tag string, page, totalPages int) string {
	if totalPages <= 1 {
		return ""
	}
	isZH := lang == blog.LangZH
	abs := func(p int) string { return h.cfg.PublicURL + blogListPathLang(lang, cat, tag, p) }
	prevLabel, nextLabel, navLabel := "上一页", "下一页", "分页导航"
	if !isZH {
		prevLabel, nextLabel, navLabel = "Previous", "Next", "Pagination"
	}

	var b strings.Builder
	b.WriteString(`<nav aria-label="` + navLabel + `">`)
	if page > 1 {
		b.WriteString(fmt.Sprintf(`<a rel="prev" href="%s">%s</a> `, abs(page-1), prevLabel))
	}
	for _, p := range pagerWindow(page, totalPages) {
		if p == 0 {
			b.WriteString("<span>…</span> ")
			continue
		}
		if p == page {
			b.WriteString(fmt.Sprintf(`<span aria-current="page">%d</span> `, p))
			continue
		}
		b.WriteString(fmt.Sprintf(`<a href="%s">%d</a> `, abs(p), p))
	}
	if page < totalPages {
		b.WriteString(fmt.Sprintf(`<a rel="next" href="%s">%s</a>`, abs(page+1), nextLabel))
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// pagerWindow returns the page numbers to show for the current page: always
// first and last, plus a sliding window of neighbours, with 0 marking a gap
// (rendered as an ellipsis). Keeps the emitted link count bounded so a 70-page
// blog still produces a small document.
func pagerWindow(page, totalPages int) []int {
	const neighbours = 2
	seen := make(map[int]bool)
	var pages []int
	add := func(p int) {
		if p >= 1 && p <= totalPages && !seen[p] {
			seen[p] = true
			pages = append(pages, p)
		}
	}
	add(1)
	for p := page - neighbours; p <= page+neighbours; p++ {
		add(p)
	}
	add(totalPages)

	sort.Ints(pages)

	// Insert gap markers where the numbering jumps.
	var out []int
	for i, p := range pages {
		if i > 0 && p-pages[i-1] > 1 {
			out = append(out, 0)
		}
		out = append(out, p)
	}
	return out
}

// BlogDetail renders a Chinese post; BlogDetailEN an English one.
func (h *Handler) BlogDetail(c *gin.Context)   { h.blogDetail(c, blog.LangZH) }
func (h *Handler) BlogDetailEN(c *gin.Context) { h.blogDetail(c, blog.LangEN) }

func (h *Handler) blogDetail(c *gin.Context, lang string) {
	slug := c.Param("slug")
	// GetByLang falls back to the Chinese post when the translation is missing,
	// so an aggregate link to /en/blog/<slug> renders instead of 404ing. The
	// returned post's own Lang is the truth about what language this page shows.
	post := h.blogStore.GetByLang(slug, lang)
	if post == nil {
		h.NotFound(c)
		return
	}
	// Fallback render: the page is served at a /en/ URL but contains Chinese
	// text. We keep the Chinese canonical (so the two URLs consolidate into one
	// indexed page instead of duplicating) and refuse to advertise an English
	// alternate that does not exist.
	pageLang := post.Lang
	fallback := pageLang != lang

	prev, next := h.blogStore.PrevNextLang(slug, pageLang)
	// 5 (was 3): article-to-article links are the second-strongest referral
	// source after the homepage, so the block is widened to cover more of the
	// long tail. Same scoring (category +3, shared tag +1, recency fallback).
	related := h.blogStore.RelatedLang(slug, pageLang, relatedLimit)

	suffix := " - BrickPlan 博客"
	if pageLang == blog.LangEN {
		suffix = " - BrickPlan Blog"
	}
	title := post.Title + suffix
	desc := post.Description
	if desc == "" {
		desc = post.Title
	}

	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, blogPostJSONLD(post, h.cfg.PublicURL, pageLang))
	jsonld = append(jsonld, articleJSONLD(post, h.cfg.PublicURL, pageLang))
	// GEO: derive FAQPage Q&A pairs from the article's own H2/H3 sections so AI
	// answer engines get explicit question/answer pairs instead of free prose.
	if qa := buildFAQFromBody(post.Body, 6); len(qa) > 0 {
		jsonld = append(jsonld, withInLanguage(faqJSONLD(qa), pageLang))
	}
	// GEO: tutorial-style posts additionally expose their H2 sections as HowTo steps.
	if howto, ok := howToJSONLD(post, h.cfg.PublicURL); ok {
		jsonld = append(jsonld, withInLanguage(template.JS(howto), pageLang))
	}

	canonical := h.cfg.PublicURL + blogDetailPath(pageLang, slug)
	pd := PageData{
		Title:       title,
		Description: truncate(desc, 160),
		Canonical:   canonical,
		OGType:      "article",
		OGImage:     firstImageURL(post.Body),
		JSONLD:      jsonld,
		Noscript:    h.blogDetailNoscript(pageLang, post, prev, next, related),
		// pageLang, NOT the requested lang: on a fallback render the requested
		// language has no page to point at, and annotating the Chinese URL as
		// hreflang="en" is a false annotation (Google may discard the whole
		// hreflang cluster, and Search Console reports it as a mismatch).
		Alternates: h.blogDetailAlternates(pageLang, slug, canonical),
	}
	if pageLang == blog.LangEN {
		pd.Lang = blog.LangEN
		pd.HTMLLang = "en"
	}
	if fallback {
		// Debug aid for the M2 content rollout: this page is a Chinese fallback,
		// not a translation. Search Console will attribute it to the zh canonical.
		pd.OGType = "article"
	}
	h.r.Render(c, pd)
}

// blogDetailAlternates emits the hreflang pair only when BOTH translations
// really exist. A one-sided pair (or an alternate that serves the other
// language's text) makes Google drop the annotation for the whole cluster.
//
// lang must be the language the page ACTUALLY renders (post.Lang), not the
// language that was requested. Passing the requested language here produced
// hreflang="en" pointing at the Chinese URL on every fallback page.
func (h *Handler) blogDetailAlternates(lang, slug, canonical string) []AltLink {
	enURL := h.cfg.PublicURL + blogDetailPath(blog.LangEN, slug)
	zhURL := h.cfg.PublicURL + blogDetailPath(blog.LangZH, slug)
	if h.blogStore.HasLang(slug, blog.LangEN) && h.blogStore.HasLang(slug, blog.LangZH) {
		return []AltLink{
			{"en", enURL},
			{"zh-CN", zhURL},
			{"x-default", enURL},
		}
	}
	return []AltLink{
		{isoLang(lang), canonical},
		{"x-default", canonical},
	}
}

// ── Noscript rendering ──

func (h *Handler) blogDetailNoscript(lang string, post *blog.Post, prev, next *blog.Post, related []blog.Post) template.HTML {
	isZH := lang == blog.LangZH
	var b strings.Builder
	b.WriteString(`<article>`)
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(post.Title)))
	b.WriteString(fmt.Sprintf(`<p><span>%s</span> · <span>%s</span> · <span>%s</span></p>`,
		post.Date.Format("2006-01-02"), esc(post.Author), esc(post.Category)))
	// Render markdown body to HTML. On an English page every internal link is
	// rewritten to the /en/ tree so crawlers stay inside the English cluster
	// instead of being pushed back into the Chinese one.
	b.WriteString(localizeBodyLinks(string(mdToHTML(post.Body)), lang, h.cfg.PublicURL))
	if len(post.Tags) > 0 {
		sep := "、"
		label := "标签："
		if !isZH {
			sep, label = ", ", "Tags: "
		}
		b.WriteString("<p>" + label)
		for i, t := range post.Tags {
			if i > 0 {
				b.WriteString(sep)
			}
			b.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`,
				esc(h.cfg.PublicURL+blogListPathLang(lang, "", t, 1)), esc(t)))
		}
		b.WriteString("</p>")
	}
	b.WriteString("</article>")

	// Related posts section (SEO internal links)
	if len(related) > 0 {
		heading := "你可能还喜欢"
		if !isZH {
			heading = "You may also like"
		}
		b.WriteString(`<section style="margin-top:40px;padding-top:20px;border-top:1px solid #ddd"><h2>` + heading + `</h2><ul>`)
		for _, r := range related {
			b.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a>`,
				esc(h.cfg.PublicURL+blogDetailPath(lang, r.Slug)), esc(r.Title)))
			if r.Category != "" {
				b.WriteString(fmt.Sprintf("（%s）", esc(r.Category)))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul></section>")
	}

	// Prev/Next navigation
	if prev != nil || next != nil {
		prevLabel, nextLabel := "上一篇", "下一篇"
		if !isZH {
			prevLabel, nextLabel = "Previous article", "Next article"
		}
		b.WriteString(`<div style="display:flex;justify-content:space-between;margin-top:40px;padding-top:20px;border-top:1px solid #ddd">`)
		if prev != nil {
			b.WriteString(fmt.Sprintf(`<div><small>%s</small><br><a href="%s">%s</a></div>`,
				prevLabel, esc(h.cfg.PublicURL+blogDetailPath(lang, prev.Slug)), esc(prev.Title)))
		} else {
			b.WriteString("<div></div>")
		}
		if next != nil {
			b.WriteString(fmt.Sprintf(`<div style="text-align:right"><small>%s</small><br><a href="%s">%s</a></div>`,
				nextLabel, esc(h.cfg.PublicURL+blogDetailPath(lang, next.Slug)), esc(next.Title)))
		}
		b.WriteString("</div>")
	}

	return template.HTML(b.String())
}

// localizeBodyLinks rewrites absolute and root-relative /blog/ links inside a
// rendered English body to the English tree. Chinese pages are returned
// untouched (their links are already correct, and rewriting them would be a
// silent content change on 1500 live pages). Links to anything other than the
// blog (blueprints, /explore) are intentionally left alone: those sections have
// no English version yet.
func localizeBodyLinks(html, lang, publicURL string) string {
	if lang != blog.LangEN {
		return html
	}
	out := strings.ReplaceAll(html, publicURL+"/blog/", publicURL+"/en/blog/")
	out = strings.ReplaceAll(out, publicURL+"/blog?", publicURL+"/en/blog?")
	out = strings.ReplaceAll(out, `href="/blog/`, `href="/en/blog/`)
	out = strings.ReplaceAll(out, `href="/blog?`, `href="/en/blog?`)
	return out
}

// firstImageURL returns the first image referenced by a post body, or "".
//
// Why: the article pages were shipping og:image = /og-default.png even after
// illustrations landed in the markdown, so every social/AI preview showed the
// site logo instead of the article's hero image. The body itself is the only
// source of truth — there is no separate cover field on blog posts.
func firstImageURL(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if m := mdImageRE.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

var mdImageRE = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^)\s]+)\)`)

// ── JSON-LD ──

// withInLanguage injects inLanguage into an already-serialized JSON object.
//
// Used for the builders that live outside this file (FAQPage, HowTo) so adding
// the language field does not require changing four signatures. Every builder
// funnels through mustJSON and therefore returns a single JSON object starting
// with "{", which is what makes the splice safe; a document that already
// declares inLanguage is returned unchanged.
func withInLanguage(js template.JS, lang string) template.JS {
	s := strings.TrimSpace(string(js))
	if !strings.HasPrefix(s, "{") || strings.Contains(s, `"inLanguage"`) {
		return js
	}
	return template.JS(`{"inLanguage":` + strconv.Quote(isoLang(lang)) + `,` + s[1:])
}

func blogPostJSONLD(post *blog.Post, public string, lang string) template.JS {
	pageURL := public + blogDetailPath(lang, post.Slug)
	jsonStr := fmt.Sprintf(`{"@context":"https://schema.org","@type":"BlogPosting","headline":%q,"description":%q,"datePublished":%q,"dateModified":%q,"author":{"@type":"Person","name":%q},"publisher":{"@type":"Organization","name":"BrickPlan","url":%q},"mainEntityOfPage":{"@type":"WebPage","@id":%q},"url":%q,"inLanguage":%q}`,
		post.Title,
		post.Description,
		post.Date.Format("2006-01-02T15:04:05Z07:00"),
		post.Date.Format("2006-01-02T15:04:05Z07:00"),
		post.Author,
		public,
		pageURL,
		pageURL,
		isoLang(lang),
	)
	return template.JS(jsonStr)
}

// articleJSONLD emits Schema.org Article structured data for blog detail pages.
// This complements BlogPosting (which is a subtype of Article) for maximum
// search engine compatibility, especially for Baidu SEO.
func articleJSONLD(post *blog.Post, public string, lang string) template.JS {
	pageURL := public + blogDetailPath(lang, post.Slug)
	m := map[string]interface{}{
		"@context":         "https://schema.org",
		"@type":            "Article",
		"headline":         post.Title,
		"description":      post.Description,
		"datePublished":    post.Date.Format("2006-01-02T15:04:05Z07:00"),
		"dateModified":     post.Date.Format("2006-01-02T15:04:05Z07:00"),
		"author":           map[string]interface{}{"@type": "Person", "name": post.Author},
		"publisher":        map[string]interface{}{"@type": "Organization", "name": "BrickPlan", "url": public, "logo": map[string]interface{}{"@type": "ImageObject", "url": public + "/og-default.png"}},
		"mainEntityOfPage": map[string]interface{}{"@type": "WebPage", "@id": pageURL},
		"url":              pageURL,
		"inLanguage":       isoLang(lang),
	}
	if len(post.Tags) > 0 {
		m["keywords"] = strings.Join(post.Tags, ", ")
	}
	if img := firstImageURL(post.Body); img != "" {
		m["image"] = []string{img}
	}
	return mustJSON(m)
}
