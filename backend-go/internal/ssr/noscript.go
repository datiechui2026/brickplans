package ssr

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"brickplans/internal/db"
)

func esc(s string) string { return html.EscapeString(s) }

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

// detailNoscript renders a simplified, crawlable article body.
func detailNoscript(bp *db.Blueprint, cover, public string) template.HTML {
	var b strings.Builder
	b.WriteString(`<article>`)
	b.WriteString(fmt.Sprintf("<h1>%s</h1>", esc(bp.Title)))
	if bp.Author != nil {
		b.WriteString(fmt.Sprintf(`<p>作者：<a href="%s/user/%s">%s</a></p>`, public, esc(bp.Author.ID), esc(bp.Author.Username)))
	}
	if bp.Description != nil && *bp.Description != "" {
		b.WriteString(string(mdToHTML(*bp.Description)))
	}
	if cover != "" {
		b.WriteString(fmt.Sprintf(`<p><img src="%s" alt="%s" /></p>`, esc(cover), esc(bp.Title)))
	}
	b.WriteString("<ul>")
	if bp.Category != nil {
		b.WriteString(fmt.Sprintf("<li>分类：%s</li>", esc(*bp.Category)))
	}
	if bp.Difficulty != nil {
		b.WriteString(fmt.Sprintf("<li>难度：%d / 5</li>", *bp.Difficulty))
	}
	if bp.PieceCount != nil {
		b.WriteString(fmt.Sprintf("<li>零件数：%d</li>", *bp.PieceCount))
	}
	if bp.Dimensions != nil {
		b.WriteString(fmt.Sprintf("<li>尺寸：%s</li>", esc(*bp.Dimensions)))
	}
	b.WriteString(fmt.Sprintf("<li>浏览：%d</li>", bp.ViewCount))
	b.WriteString(fmt.Sprintf("<li>点赞：%d</li>", bp.LikeCount))
	b.WriteString("</ul>")
	if len(bp.Tags) > 0 {
		b.WriteString("<p>标签：")
		for i, bt := range bp.Tags {
			if bt.Tag == nil {
				continue
			}
			if i > 0 {
				b.WriteString("、")
			}
			b.WriteString(fmt.Sprintf(`<a href="%s/tags/%s">%s</a>`, public, esc(bt.Tag.Name), esc(bt.Tag.Name)))
		}
		b.WriteString("</p>")
	}
	b.WriteString("</article>")
	return template.HTML(b.String())
}

// listNoscript renders a simple list of blueprint cards for explore/tag/user pages.
func listNoscript(bps []db.Blueprint, public string) template.HTML {
	if len(bps) == 0 {
		return template.HTML("<p>暂无作品。</p>")
	}
	var b strings.Builder
	b.WriteString("<ul>")
	for _, bp := range bps {
		b.WriteString(fmt.Sprintf(`<li><a href="%s/detail/%s">%s</a>`, public, bp.ID, esc(bp.Title)))
		if bp.Category != nil {
			b.WriteString(fmt.Sprintf("（%s）", esc(*bp.Category)))
		}
		if bp.PieceCount != nil {
			b.WriteString(fmt.Sprintf(" · %d 零件", *bp.PieceCount))
		}
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
	return template.HTML(b.String())
}

func homeNoscript(bps []db.Blueprint, public string) template.HTML {
	var b strings.Builder
	b.WriteString(`<h1>BrickPlan — 积木图纸分享社区</h1>`)
	b.WriteString("<p>发现和分享乐高 MOC 创意作品。浏览建筑、车辆、机甲、奇幻、科幻、场景等各类积木图纸。</p>")
	if len(bps) > 0 {
		b.WriteString("<h2>热门作品</h2>")
		b.WriteString(string(listNoscript(bps, public)))
	}
	b.WriteString(fmt.Sprintf(`<p><a href="%s/explore">浏览全部作品</a></p>`, public))
	return template.HTML(b.String())
}

func faqNoscript(qa []struct{ Q, A string }) template.HTML {
	var b strings.Builder
	b.WriteString("<h1>常见问题</h1>")
	for _, item := range qa {
		b.WriteString(fmt.Sprintf("<h2>%s</h2>", esc(item.Q)))
		b.WriteString(fmt.Sprintf("<p>%s</p>", esc(item.A)))
	}
	return template.HTML(b.String())
}
