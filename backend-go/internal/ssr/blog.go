package ssr

import (
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
)

// BlogList renders the blog index page.
func (h *Handler) BlogList(c *gin.Context) {
	cat := c.Query("category")
	tag := c.Query("tag")
	page := atoiOrSSR(c.Query("page"), 1)

	// Clamp page into range so /blog?page=999 returns the last page (HTTP 200)
	// instead of an empty document. An empty index page would be a soft 404 and
	// would drop the crawlable link chain for a crawler that followed a stale
	// pagination link.
	items, total := blog.Paginate(h.blogStore.Filter(cat, tag), page, blog.SSRPageSize)
	totalPages := blog.PageCount(total, blog.SSRPageSize)
	if page > totalPages {
		page = totalPages
		items, total = blog.Paginate(h.blogStore.Filter(cat, tag), page, blog.SSRPageSize)
	}

	filterLabel := ""
	if cat != "" {
		filterLabel = "分类：" + cat
	} else if tag != "" {
		filterLabel = "标签：#" + tag
	}

	title := "博客 - BrickPlan 积木图纸社区"
	desc := "BrickPlan 博客：积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容。"
	if filterLabel != "" {
		title = filterLabel + " - BrickPlan 博客"
		desc = "BrickPlan 博客「" + filterLabel + "」分类下的文章。"
	}
	if page > 1 {
		title = fmt.Sprintf("%s（第 %d 页 / 共 %d 页）", title, page, totalPages)
		desc = fmt.Sprintf("%s（第 %d 页，共 %d 篇）", desc, page, total)
	}

	// Self-referencing canonical. Do NOT canonicalise page >= 2 back to /blog:
	// that makes search engines ignore every link on the page and severs the
	// crawl chain between pagination pages.
	canonical := h.cfg.PublicURL + blogListPath(cat, tag, page)

	h.r.Render(c, PageData{
		Title:       title,
		Description: desc,
		Canonical:   canonical,
		OGType:      "website",
		JSONLD:      h.siteJSONLD(),
		Noscript:    h.blogListNoscript(items, filterLabel, cat, tag, page, totalPages, total),
	})
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

// blogListPath builds the site-relative URL for a (possibly filtered,
// possibly paginated) blog index. Uses url.Values so Chinese categories/tags
// are percent-encoded exactly like the SPA's URLSearchParams does.
func blogListPath(cat, tag string, page int) string {
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
		return "/blog"
	}
	return "/blog?" + q.Encode()
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
func (h *Handler) blogListNoscript(items []blog.Post, filterLabel, cat, tag string, page, totalPages, total int) template.HTML {
	var b strings.Builder
	b.WriteString("<h1>BrickPlan 博客</h1>")
	b.WriteString("<p>积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容。</p>")
	if filterLabel != "" {
		b.WriteString(fmt.Sprintf("<p>筛选：%s（共 %d 篇）</p>", esc(filterLabel), total))
	}
	if totalPages > 1 {
		b.WriteString(fmt.Sprintf("<p>第 %d / %d 页</p>", page, totalPages))
	}
	if len(items) == 0 {
		b.WriteString("<p>暂无文章。</p>")
		return template.HTML(b.String())
	}
	b.WriteString("<ul>")
	for _, p := range items {
		dateStr := p.Date.Format("2006-01-02")
		b.WriteString(fmt.Sprintf(`<li><a href="%s/blog/%s">%s</a> (%s) <span>%s</span></li>`,
			h.cfg.PublicURL, esc(p.Slug), esc(p.Title), dateStr, esc(p.Category)))
	}
	b.WriteString("</ul>")
	b.WriteString(h.blogPager(cat, tag, page, totalPages))
	return template.HTML(b.String())
}

// blogPager renders prev / numbered / next pagination with real crawlable links.
// Returns "" when there is only one page.
func (h *Handler) blogPager(cat, tag string, page, totalPages int) string {
	if totalPages <= 1 {
		return ""
	}
	abs := func(p int) string { return h.cfg.PublicURL + blogListPath(cat, tag, p) }

	var b strings.Builder
	b.WriteString(`<nav aria-label="分页导航">`)
	if page > 1 {
		b.WriteString(fmt.Sprintf(`<a rel="prev" href="%s">上一页</a> `, abs(page-1)))
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
		b.WriteString(fmt.Sprintf(`<a rel="next" href="%s">下一页</a>`, abs(page+1)))
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

// BlogDetail renders a single blog post.
func (h *Handler) BlogDetail(c *gin.Context) {
	slug := c.Param("slug")
	post := h.blogStore.Get(slug)
	if post == nil {
		h.NotFound(c)
		return
	}

	prev, next := h.blogStore.PrevNext(slug)
	related := h.blogStore.Related(slug, 3)

	title := post.Title + " - BrickPlan 博客"
	desc := post.Description
	if desc == "" {
		desc = post.Title
	}

	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, blogPostJSONLD(post, h.cfg.PublicURL))
	jsonld = append(jsonld, articleJSONLD(post, h.cfg.PublicURL))
	// GEO: derive FAQPage Q&A pairs from the article's own H2/H3 sections so AI
	// answer engines get explicit question/answer pairs instead of free prose.
	if qa := buildFAQFromBody(post.Body, 6); len(qa) > 0 {
		jsonld = append(jsonld, faqJSONLD(qa))
	}
	// GEO: tutorial-style posts additionally expose their H2 sections as HowTo steps.
	if howto, ok := howToJSONLD(post, h.cfg.PublicURL); ok {
		jsonld = append(jsonld, template.JS(howto))
	}

	h.r.Render(c, PageData{
		Title:       title,
		Description: truncate(desc, 160),
		Canonical:   h.cfg.PublicURL + "/blog/" + slug,
		OGType:      "article",
		JSONLD:      jsonld,
		Noscript:    h.blogDetailNoscript(post, prev, next, related),
	})
}

// ── Noscript rendering ──

func (h *Handler) blogDetailNoscript(post *blog.Post, prev, next *blog.Post, related []blog.Post) template.HTML {
	var b strings.Builder
	b.WriteString(`<article>`)
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(post.Title)))
	b.WriteString(fmt.Sprintf(`<p><span>%s</span> · <span>%s</span> · <span>%s</span></p>`,
		post.Date.Format("2006-01-02"), esc(post.Author), esc(post.Category)))
	// Render markdown body to HTML
	b.WriteString(string(mdToHTML(post.Body)))
	if len(post.Tags) > 0 {
		b.WriteString("<p>标签：")
		for i, t := range post.Tags {
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(fmt.Sprintf(`<a href="%s/blog?tag=%s">%s</a>`, h.cfg.PublicURL, esc(t), esc(t)))
		}
		b.WriteString("</p>")
	}
	b.WriteString("</article>")

	// Related posts section (SEO internal links)
	if len(related) > 0 {
		b.WriteString(`<section style="margin-top:40px;padding-top:20px;border-top:1px solid #ddd"><h2>你可能还喜欢</h2><ul>`)
		for _, r := range related {
			b.WriteString(fmt.Sprintf(`<li><a href="%s/blog/%s">%s</a>`, h.cfg.PublicURL, esc(r.Slug), esc(r.Title)))
			if r.Category != "" {
				b.WriteString(fmt.Sprintf("（%s）", esc(r.Category)))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul></section>")
	}

	// Prev/Next navigation
	if prev != nil || next != nil {
		b.WriteString(`<div style="display:flex;justify-content:space-between;margin-top:40px;padding-top:20px;border-top:1px solid #ddd">`)
		if prev != nil {
			b.WriteString(fmt.Sprintf(`<div><small>上一篇</small><br><a href="%s/blog/%s">%s</a></div>`,
				h.cfg.PublicURL, esc(prev.Slug), esc(prev.Title)))
		} else {
			b.WriteString("<div></div>")
		}
		if next != nil {
			b.WriteString(fmt.Sprintf(`<div style="text-align:right"><small>下一篇</small><br><a href="%s/blog/%s">%s</a></div>`,
				h.cfg.PublicURL, esc(next.Slug), esc(next.Title)))
		}
		b.WriteString("</div>")
	}

	return template.HTML(b.String())
}

// ── JSON-LD ──

func blogPostJSONLD(post *blog.Post, public string) template.JS {
	jsonStr := fmt.Sprintf(`{"@context":"https://schema.org","@type":"BlogPosting","headline":%q,"description":%q,"datePublished":%q,"dateModified":%q,"author":{"@type":"Person","name":%q},"publisher":{"@type":"Organization","name":"BrickPlan","url":%q},"mainEntityOfPage":{"@type":"WebPage","@id":%q},"url":%q}`,
		post.Title,
		post.Description,
		post.Date.Format("2006-01-02T15:04:05Z07:00"),
		post.Date.Format("2006-01-02T15:04:05Z07:00"),
		post.Author,
		public,
		public+"/blog/"+post.Slug,
		public+"/blog/"+post.Slug,
	)
	return template.JS(jsonStr)
}

// articleJSONLD emits Schema.org Article structured data for blog detail pages.
// This complements BlogPosting (which is a subtype of Article) for maximum
// search engine compatibility, especially for Baidu SEO.
func articleJSONLD(post *blog.Post, public string) template.JS {
	m := map[string]interface{}{
		"@context":         "https://schema.org",
		"@type":            "Article",
		"headline":         post.Title,
		"description":      post.Description,
		"datePublished":    post.Date.Format("2006-01-02T15:04:05Z07:00"),
		"dateModified":     post.Date.Format("2006-01-02T15:04:05Z07:00"),
		"author":           map[string]interface{}{"@type": "Person", "name": post.Author},
		"publisher":        map[string]interface{}{"@type": "Organization", "name": "BrickPlan", "url": public, "logo": map[string]interface{}{"@type": "ImageObject", "url": public + "/og-default.png"}},
		"mainEntityOfPage": map[string]interface{}{"@type": "WebPage", "@id": public + "/blog/" + post.Slug},
		"url":              public + "/blog/" + post.Slug,
		"inLanguage":       "zh-CN",
	}
	if len(post.Tags) > 0 {
		m["keywords"] = strings.Join(post.Tags, ", ")
	}
	return mustJSON(m)
}
