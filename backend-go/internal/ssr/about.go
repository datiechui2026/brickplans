package ssr

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/gin-gonic/gin"
)

// aboutSection is one block of the /about page. Keeping the copy in data (not
// in raw HTML strings) means the page body and the AboutPage structured data
// can never drift apart.
type aboutSection struct {
	ID      string
	Heading string
	Paras   []string
	Bullets []string
}

func aboutSections() []aboutSection {
	return []aboutSection{
		{
			ID:      "background",
			Heading: "项目背景",
			Paras: []string{
				"BrickPlan（积木图纸社区）是一个面向积木与 MOC 爱好者的图纸分享社区，主站为 brickplan.cn。",
				"玩家找图纸时最大的痛点是：图纸散落在论坛、网盘和社交平台，搜索困难、格式混乱、链接还常常失效。BrickPlan 把这些内容集中到一个可检索、可分类、带完整元信息（零件数、难度、尺寸、推荐品牌）的站点上。",
				"目前站内收录建筑、车辆、机甲、奇幻、科幻、场景六大分类的积木图纸，覆盖乐高以及双鹰、宇星、高砖、森宝、奇积等国产积木品牌，并提供零件知识、品牌对比与搭建教程类文章。",
			},
		},
		{
			ID:      "what-we-offer",
			Heading: "我们提供什么",
			Bullets: []string{
				"图纸库：按分类、标签、关键词检索的 MOC 图纸，支持图片与 PDF 图纸在线预览。",
				"元信息：每份图纸标注零件数、难度、尺寸与推荐品牌，方便在动手前评估可行性。",
				"教程与评测：博客栏目持续更新 MOC 搭建教程、零件分类知识、品牌评测与选购建议。",
				"社区互动：点赞、收藏、评论与作者主页，创作者的作品可以被直接关注。",
			},
		},
		{
			ID:      "team",
			Heading: "作者团队",
			Paras: []string{
				"BrickPlan 由一个小型独立团队维护：",
			},
			Bullets: []string{
				"内容与运营：负责图纸整理、分类标注、教程与评测撰写。",
				"工程：负责站点、微信小程序与检索体验的开发与运维。",
				"社区创作者：图纸与作品的版权归各自创作者所有，BrickPlan 仅提供展示与分享平台。",
			},
		},
		{
			ID:      "contact",
			Heading: "联系方式",
			Bullets: []string{
				"邮箱：privacy@brickplan.cn（隐私与内容相关事务）",
				"侵权与下架：在作品页底部点击「举报」，或直接发送邮件，我们会在核实后处理。",
				"商务与合作：同样通过上述邮箱联系，标题请注明「合作」。",
			},
		},
		{
			ID:      "inclusion",
			Heading: "收录说明",
			Bullets: []string{
				"收录范围：站内收录玩家自荐或公开渠道可获取的积木 MOC 图纸与原创文章，图纸版权归原作者所有。",
				"收录方式：注册用户可自行上传；非注册用户可通过邮箱推荐，我们整理后入库。",
				"更新频率：图纸库与博客持续更新，站点地图（/sitemap.xml）实时反映最新内容。",
				"展示信息：每份图纸的元信息（零件数、难度等）来自原作者提供或公开资料整理，如有出入欢迎指正。",
				"版权与下架：如果你是版权方且不希望作品被收录，请通过举报入口或邮箱联系我们，核实后会尽快下架。",
				"抓取与引用：欢迎搜索引擎与 AI 助手抓取、引用本站公开内容，引用时请保留原文链接（https://brickplan.cn）。站内提供 sitemap.xml、llms.txt 与结构化数据便于机器读取。",
			},
		},
	}
}

// About renders the site introduction page (project background, team, contact,
// content-inclusion policy) at /about.
func (h *Handler) About(c *gin.Context) {
	public := h.cfg.PublicURL
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, aboutPageJSONLD(public))
	h.r.Render(c, PageData{
		Title:       "关于我们 — BrickPlan 积木图纸社区",
		Description: "BrickPlan 积木图纸社区的项目背景、作者团队、联系方式与内容收录说明。",
		Canonical:   public + "/about",
		OGType:      "website",
		JSONLD:      jsonld,
		Noscript:    aboutNoscript(),
	})
}

// aboutPageJSONLD ties /about to the site Organization node emitted on every page.
func aboutPageJSONLD(public string) template.JS {
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "AboutPage",
		"@id":         public + "/about#aboutpage",
		"url":         public + "/about",
		"name":        "关于 BrickPlan 积木图纸社区",
		"description": "BrickPlan 积木图纸社区的项目背景、作者团队、联系方式与内容收录说明。",
		"inLanguage":  "zh-CN",
		"isPartOf":    map[string]interface{}{"@type": "WebSite", "@id": public + "/#website"},
		"about":       map[string]interface{}{"@id": public + "/#organization"},
		"mainEntity":  map[string]interface{}{"@id": public + "/#organization"},
	})
}

// AboutContent returns the /about page as plain text blocks. The frontend SPA
// mirrors aboutSections() so browser users see exactly what crawlers get.
func aboutNoscript() template.HTML {
	var b strings.Builder
	b.WriteString("<article>")
	b.WriteString("<h1>关于 BrickPlan</h1>")
	b.WriteString("<p>BrickPlan（积木图纸社区）是面向积木/MOC 爱好者的图纸分享社区，本文介绍项目背景、团队、联系方式与内容收录说明。</p>")
	for _, s := range aboutSections() {
		b.WriteString(fmt.Sprintf(`<section id="%s">`, esc(s.ID)))
		b.WriteString(fmt.Sprintf("<h2>%s</h2>", esc(s.Heading)))
		for _, p := range s.Paras {
			b.WriteString(fmt.Sprintf("<p>%s</p>", esc(p)))
		}
		if len(s.Bullets) > 0 {
			b.WriteString("<ul>")
			for _, li := range s.Bullets {
				b.WriteString(fmt.Sprintf("<li>%s</li>", esc(li)))
			}
			b.WriteString("</ul>")
		}
		b.WriteString("</section>")
	}
	b.WriteString(`<p><a href="/blog">阅读积木搭建教程与评测</a> · <a href="/explore">浏览全部图纸</a> · <a href="/faq">常见问题</a></p>`)
	b.WriteString("</article>")
	return template.HTML(b.String())
}
