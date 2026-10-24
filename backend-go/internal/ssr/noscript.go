package ssr

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"brickplans/internal/blog"
	"brickplans/internal/db"
)

func esc(s string) string { return html.EscapeString(s) }

// wrapBody gives an SSR body the same container skeleton the SPA builds for the
// equivalent page — <div class="page active"><div class="main"><div class="class">…
//
// Why it exists: template.html drops the SSR body straight into <div id="app">,
// so on an EN page (which the SPA hydrates after boot) the body is what the user
// and non-JS crawlers see first. The class names are the only contract the
// stylesheet has — without .page/.main the rules in main.css never match and the
// content renders unstyled at the top-left of the viewport (P0, 2026-10-15).
//
// class is always one of our own literals ("blog-list", "blog-detail"), never
// request data.
func wrapBody(class string, inner template.HTML) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="page active"><div class="main">`)
	if class != "" {
		b.WriteString(`<div class="` + class + `">`)
	}
	b.WriteString(string(inner))
	if class != "" {
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

// markdownBody wraps prose in .markdown-body so headings, lists and links pick
// up the typography rules instead of browser defaults.
func markdownBody(inner template.HTML) template.HTML {
	return template.HTML(`<div class="markdown-body">` + string(inner) + `</div>`)
}

func absURL(u, public string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return public + u
}

// coverURL mirrors the frontend cover logic, returning an absolute URL.
func coverURL(imgs []db.BlueprintImage, public string) string {
	var firstNonPDF string
	for _, im := range imgs {
		ft := im.FileType
		if ft == "" {
			ft = "image"
		}
		if ft == "pdf" {
			continue
		}
		if firstNonPDF == "" {
			firstNonPDF = im.URL
		}
		if im.IsCover {
			return absURL(im.URL, public)
		}
	}
	if firstNonPDF != "" {
		return absURL(firstNonPDF, public)
	}
	if len(imgs) > 0 {
		return absURL(imgs[0].URL, public)
	}
	return public + "/og-default.png"
}

// ── 语言化正文标签 ─────────────────────────────────────────────────────────
//
// The Chinese strings below are byte-for-byte what detailNoscript/listNoscript/
// homeNoscript emitted before this table existed (the EN work must not change a
// single Chinese page). Every label ends with its separator so the call sites
// stay `<li>{{label}}{{value}}</li>` for both languages.

type bodyLabels struct {
	Author      string
	Category    string
	Difficulty  string
	Parts       string
	Dimensions  string
	Brand       string
	SetNumber   string
	Views       string
	Likes       string
	Tags        string
	Related     string
	NoItems     string
	PartUnit    string
	TagSep      string
	CatOpen     string
	CatClose    string
	HomeTitle   string
	HomeLead    string
	Featured    string
	Latest      string
	LatestPosts string
	AllBuilds   string
	AllPosts    string
	FAQTitle    string
}

func bodyLabelsFor(lang string) bodyLabels {
	if lang == "en" {
		return bodyLabels{
			Author:      "Author: ",
			Category:    "Category: ",
			Difficulty:  "Difficulty: ",
			Parts:       "Parts: ",
			Dimensions:  "Dimensions: ",
			Brand:       "Brand: ",
			SetNumber:   "Set number: ",
			Views:       "Views: ",
			Likes:       "Likes: ",
			Tags:        "Tags: ",
			Related:     "Related builds",
			NoItems:     "No blueprints yet.",
			PartUnit:    " parts",
			TagSep:      ", ",
			CatOpen:     " (",
			CatClose:    ")",
			HomeTitle:   "BrickPlan — MOC Blueprints & Building Guides",
			HomeLead:    "Discover and share LEGO MOC creations — architecture, vehicles, mecha, fantasy, sci-fi and scenery blueprints.",
			Featured:    "Featured builds",
			Latest:      "Latest builds",
			LatestPosts: "Latest articles",
			AllBuilds:   "Browse all builds",
			AllPosts:    "Read all articles",
			FAQTitle:    "Frequently asked questions",
		}
	}
	return bodyLabels{
		Author:      "作者：",
		Category:    "分类：",
		Difficulty:  "难度：",
		Parts:       "零件数：",
		Dimensions:  "尺寸：",
		Brand:       "品牌：",
		SetNumber:   "编号：",
		Views:       "浏览：",
		Likes:       "点赞：",
		Tags:        "标签：",
		Related:     "相关作品",
		NoItems:     "暂无作品。",
		PartUnit:    " 零件",
		TagSep:      "、",
		CatOpen:     "（",
		CatClose:    "）",
		HomeTitle:   "BrickPlan — 积木图纸分享社区",
		HomeLead:    "发现和分享乐高 MOC 创意作品。浏览建筑、车辆、机甲、奇幻、科幻、场景等各类积木图纸。",
		Featured:    "推荐作品",
		Latest:      "最新作品",
		LatestPosts: "最新博客",
		AllBuilds:   "浏览全部作品",
		AllPosts:    "查看全部博客文章",
		FAQTitle:    "常见问题",
	}
}

// langPath builds a site path inside a language tree. Chinese ("") returns the
// path unchanged, which is what keeps every already-indexed Chinese URL stable.
func langPath(lang, path string) string { return langFor(lang).URL(path) }

// detailNoscript renders a simplified, crawlable article body.
func detailNoscript(bp *db.Blueprint, cover, public string, related []db.Blueprint) template.HTML {
	return detailNoscriptFor("zh", bp, cover, public, related)
}

func detailNoscriptFor(lang string, bp *db.Blueprint, cover, public string, related []db.Blueprint) template.HTML {
	l := bodyLabelsFor(lang)
	var b strings.Builder
	b.WriteString(`<article>`)
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(bp.Title)))
	if bp.Author != nil {
		b.WriteString(fmt.Sprintf(`<p>%s<a href="%s">%s</a></p>`, l.Author,
			public+langPath(lang, "/user/"+bp.Author.ID), esc(bp.Author.Username)))
	}
	if bp.Description != nil && *bp.Description != "" {
		b.WriteString(string(mdToHTML(*bp.Description)))
	}
	if cover != "" {
		b.WriteString(fmt.Sprintf(`<p><img src="%s" alt="%s" /></p>`, esc(cover), esc(bp.Title)))
	}
	b.WriteString("<ul>")
	if bp.Category != nil {
		b.WriteString(fmt.Sprintf("<li>%s%s</li>", l.Category, esc(*bp.Category)))
	}
	if bp.Difficulty != nil {
		b.WriteString(fmt.Sprintf("<li>%s%d / 5</li>", l.Difficulty, *bp.Difficulty))
	}
	if bp.PieceCount != nil {
		b.WriteString(fmt.Sprintf("<li>%s%d</li>", l.Parts, *bp.PieceCount))
	}
	if bp.Dimensions != nil {
		b.WriteString(fmt.Sprintf("<li>%s%s</li>", l.Dimensions, esc(*bp.Dimensions)))
	}
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		b.WriteString(fmt.Sprintf("<li>%s%s</li>", l.Brand, esc(strings.TrimSpace(*bp.Brand))))
	}
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		b.WriteString(fmt.Sprintf("<li>%s%s</li>", l.SetNumber, esc(strings.TrimSpace(*bp.ProductCode))))
	}
	b.WriteString(fmt.Sprintf("<li>%s%d</li>", l.Views, bp.ViewCount))
	b.WriteString(fmt.Sprintf("<li>%s%d</li>", l.Likes, bp.LikeCount))
	b.WriteString("</ul>")
	if len(bp.Tags) > 0 {
		b.WriteString(fmt.Sprintf("<p>%s", l.Tags))
		first := true
		for _, bt := range bp.Tags {
			if bt.Tag == nil {
				continue
			}
			if !first {
				b.WriteString(l.TagSep)
			}
			first = false
			b.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`,
				public+langPath(lang, "/tags/"+bt.Tag.Name), esc(bt.Tag.Name)))
		}
		b.WriteString("</p>")
	}
	b.WriteString("</article>")

	// Related blueprints section (SEO internal links)
	if len(related) > 0 {
		b.WriteString(fmt.Sprintf(`<section style="margin-top:40px;padding-top:20px;border-top:1px solid #ddd"><h2>%s</h2><ul>`, esc(l.Related)))
		for _, r := range related {
			b.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a>`,
				public+langPath(lang, "/detail/"+r.ID), esc(r.Title)))
			if r.Category != nil {
				b.WriteString(fmt.Sprintf("%s%s%s", l.CatOpen, esc(*r.Category), l.CatClose))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul></section>")
	}

	return template.HTML(b.String())
}

// listNoscript renders a simple list of blueprint cards for explore/tag/user pages.
func listNoscript(bps []db.Blueprint, public string) template.HTML {
	return listNoscriptFor("zh", bps, public)
}

func listNoscriptFor(lang string, bps []db.Blueprint, public string) template.HTML {
	l := bodyLabelsFor(lang)
	if len(bps) == 0 {
		return template.HTML("<p>" + esc(l.NoItems) + "</p>")
	}
	var b strings.Builder
	b.WriteString("<ul>")
	for _, bp := range bps {
		b.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a>`,
			public+langPath(lang, "/detail/"+bp.ID), esc(bp.Title)))
		if bp.Category != nil {
			b.WriteString(fmt.Sprintf("%s%s%s", l.CatOpen, esc(*bp.Category), l.CatClose))
		}
		if bp.PieceCount != nil {
			b.WriteString(fmt.Sprintf(" · %d%s", *bp.PieceCount, l.PartUnit))
		}
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
	return template.HTML(b.String())
}

// detailNoscriptEN renders the English blueprint detail body (/en/detail).
// Mirrors detailNoscript one-for-one — same fields, same order, same 4 related
// builds — but with English labels and /en/detail links. The blueprint's own
// title/description stay in their published language.
func detailNoscriptEN(bp *db.Blueprint, cover, public string, related []db.Blueprint) template.HTML {
	var b strings.Builder
	b.WriteString(`<article>`)
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(bp.Title)))
	if bp.Author != nil {
		b.WriteString(fmt.Sprintf(`<p>By <a href="%s/user/%s">%s</a></p>`, public, esc(bp.Author.ID), esc(bp.Author.Username)))
	}
	if bp.Description != nil && *bp.Description != "" {
		b.WriteString(string(mdToHTML(*bp.Description)))
	}
	if cover != "" {
		b.WriteString(fmt.Sprintf(`<p><img src="%s" alt="%s" /></p>`, esc(cover), esc(bp.Title)))
	}
	b.WriteString("<ul>")
	if bp.Category != nil {
		b.WriteString(fmt.Sprintf("<li>Category: %s</li>", esc(*bp.Category)))
	}
	if bp.Difficulty != nil {
		b.WriteString(fmt.Sprintf("<li>Difficulty: %d / 5</li>", *bp.Difficulty))
	}
	if bp.PieceCount != nil {
		b.WriteString(fmt.Sprintf("<li>Pieces: %d</li>", *bp.PieceCount))
	}
	if bp.Dimensions != nil {
		b.WriteString(fmt.Sprintf("<li>Dimensions: %s</li>", esc(*bp.Dimensions)))
	}
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		b.WriteString(fmt.Sprintf("<li>Brand: %s</li>", esc(strings.TrimSpace(*bp.Brand))))
	}
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		b.WriteString(fmt.Sprintf("<li>Set code: %s</li>", esc(strings.TrimSpace(*bp.ProductCode))))
	}
	b.WriteString(fmt.Sprintf("<li>Views: %d</li>", bp.ViewCount))
	b.WriteString(fmt.Sprintf("<li>Likes: %d</li>", bp.LikeCount))
	b.WriteString("</ul>")
	if len(bp.Tags) > 0 {
		b.WriteString("<p>Tags: ")
		for i, bt := range bp.Tags {
			if bt.Tag == nil {
				continue
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf(`<a href="%s/tags/%s">%s</a>`, public, esc(bt.Tag.Name), esc(bt.Tag.Name)))
		}
		b.WriteString("</p>")
	}
	b.WriteString("</article>")

	// Related blueprints section — English labels, /en/detail links.
	if len(related) > 0 {
		b.WriteString(`<section style="margin-top:40px;padding-top:20px;border-top:1px solid #ddd"><h2>Related builds</h2><ul>`)
		for _, r := range related {
			b.WriteString(fmt.Sprintf(`<li><a href="%s/en/detail/%s">%s</a>`, public, r.ID, esc(r.Title)))
			if r.Category != nil {
				b.WriteString(fmt.Sprintf(" (%s)", esc(*r.Category)))
			}
			b.WriteString("</li>")
		}
		b.WriteString("</ul></section>")
	}
	b.WriteString(`<p><a href="` + public + `/en/explore` + `">← Back to the blueprint library</a> · <a href="` + public + `/en/blog` + `">English blog</a></p>`)
	return template.HTML(b.String())
}

func homeNoscript(featured, latest []db.Blueprint, posts []blog.Post, public string) template.HTML {
	return homeBodyFor("zh", featured, latest, posts, public, "")
}

// homeBodyFor is the crawlable home body in one language. postBase is the blog
// tree prefix ("/blog" or "/en/blog") so the article links point into the same
// language the page is written in.
func homeBodyFor(lang string, featured, latest []db.Blueprint, posts []blog.Post, public, postBase string) template.HTML {
	l := bodyLabelsFor(lang)
	if postBase == "" {
		postBase = langPath(lang, "/blog")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(l.HomeTitle)))
	b.WriteString(fmt.Sprintf("<p>%s</p>", esc(l.HomeLead)))
	if len(featured) > 0 {
		b.WriteString(fmt.Sprintf("<h2>%s</h2>", esc(l.Featured)))
		b.WriteString(string(listNoscriptFor(lang, featured, public)))
	}
	if len(latest) > 0 {
		b.WriteString(fmt.Sprintf("<h2>%s</h2>", esc(l.Latest)))
		b.WriteString(string(listNoscriptFor(lang, latest, public)))
	}
	b.WriteString(fmt.Sprintf(`<p><a href="%s">%s</a></p>`, public+langPath(lang, "/explore"), esc(l.AllBuilds)))
	// 最新博客：首页是本站权重最高的已收录页，这里直接给文章页提供引荐来源。
	// 之前首页 0 个 /blog/ 链接，是 GSC「已发现-尚未编入索引」的主因之一。
	if len(posts) > 0 {
		b.WriteString(fmt.Sprintf("<h2>%s</h2><ul>", esc(l.LatestPosts)))
		for _, p := range posts {
			b.WriteString(fmt.Sprintf(`<li><a href="%s/%s">%s</a> <small>%s</small> <span>%s</span></li>`,
				public+postBase, esc(p.Slug), esc(p.Title), p.Date.Format("2006-01-02"), esc(p.Category)))
		}
		b.WriteString("</ul>")
		b.WriteString(fmt.Sprintf(`<p><a href="%s">%s</a></p>`, public+postBase, esc(l.AllPosts)))
	}
	// CN footer (P1-1): mirrors homeFooterEN on /en/ so both homepages end with
	// the same structure — site line + nav links + a cross-language link.
	b.WriteString(string(homeFooterCN(public)))
	return template.HTML(b.String())
}

func faqNoscript(qa []QA) template.HTML {
	return faqNoscriptFor("zh", qa)
}

func faqNoscriptFor(lang string, qa []QA) template.HTML {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(bodyLabelsFor(lang).FAQTitle)))
	for _, item := range qa {
		b.WriteString(fmt.Sprintf("<h2>%s</h2>", esc(item.Q)))
		b.WriteString(fmt.Sprintf("<p>%s</p>", esc(item.A)))
	}
	return template.HTML(b.String())
}
