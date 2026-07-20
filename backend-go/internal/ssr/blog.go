package ssr

import (
	"fmt"
	"strings"
	"html/template"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
)

// BlogList renders the blog index page.
func (h *Handler) BlogList(c *gin.Context) {
	cat := c.Query("category")
	tag := c.Query("tag")

	var posts []blog.Post
	filterLabel := ""
	if cat != "" {
		posts = h.blogStore.FilterByCategory(cat)
		filterLabel = "分类：" + cat
	} else if tag != "" {
		posts = h.blogStore.FilterByTag(tag)
		filterLabel = "标签：#" + tag
	} else {
		posts = h.blogStore.All()
	}

	title := "博客 - BrickPlans 积木图纸社区"
	desc := "BrickPlans 博客：积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容。"
	if filterLabel != "" {
		title = filterLabel + " - BrickPlans 博客"
		desc = "BrickPlans 博客「" + filterLabel + "」分类下的文章。"
	}

	canonical := h.cfg.PublicURL + "/blog"
	if cat != "" {
		canonical += "?category=" + cat
	} else if tag != "" {
		canonical += "?tag=" + tag
	}

	h.r.Render(c, PageData{
		Title:       title,
		Description: desc,
		Canonical:   canonical,
		OGType:      "website",
		JSONLD:      h.siteJSONLD(),
		Noscript:    h.blogListNoscript(posts, filterLabel),
	})
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

	title := post.Title + " - BrickPlans 博客"
	desc := post.Description
	if desc == "" {
		desc = post.Title
	}

	h.r.Render(c, PageData{
		Title:       title,
		Description: truncate(desc, 160),
		Canonical:   h.cfg.PublicURL + "/blog/" + slug,
		OGType:      "article",
		JSONLD:      append(h.siteJSONLD(), blogPostJSONLD(post, h.cfg.PublicURL)),
		Noscript:    h.blogDetailNoscript(post, prev, next),
	})
}

// ── Noscript rendering ──

func (h *Handler) blogListNoscript(posts []blog.Post, filterLabel string) template.HTML {
	var b strings.Builder
	b.WriteString("<h1>BrickPlans 博客</h1>")
	b.WriteString("<p>积木 MOC 入门指南、零件知识、品牌推荐、搭建教程等精彩内容。</p>")
	if filterLabel != "" {
		b.WriteString(fmt.Sprintf("<p>筛选：%s</p>", esc(filterLabel)))
	}
	if len(posts) == 0 {
		b.WriteString("<p>暂无文章。</p>")
		return template.HTML(b.String())
	}
	b.WriteString("<ul>")
	for _, p := range posts {
		dateStr := p.Date.Format("2006-01-02")
		b.WriteString(fmt.Sprintf(`<li><a href="%s/blog/%s">%s</a> (%s) <span>%s</span></li>`,
			h.cfg.PublicURL, esc(p.Slug), esc(p.Title), dateStr, esc(p.Category)))
	}
	b.WriteString("</ul>")
	return template.HTML(b.String())
}

func (h *Handler) blogDetailNoscript(post *blog.Post, prev, next *blog.Post) template.HTML {
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

func blogPostJSONLD(post *blog.Post, public string) template.HTML {
	jsonStr := fmt.Sprintf(`{"@context":"https://schema.org","@type":"BlogPosting","headline":%q,"description":%q,"datePublished":%q,"dateModified":%q,"author":{"@type":"Person","name":%q},"publisher":{"@type":"Organization","name":"BrickPlans","url":%q},"mainEntityOfPage":{"@type":"WebPage","@id":%q},"url":%q}`,
		post.Title,
		post.Description,
		post.Date.Format("2006-01-02T15:04:05Z07:00"),
		post.Date.Format("2006-01-02T15:04:05Z07:00"),
		post.Author,
		public,
		public+"/blog/"+post.Slug,
		public+"/blog/"+post.Slug,
	)
	return template.HTML(jsonStr)
}
