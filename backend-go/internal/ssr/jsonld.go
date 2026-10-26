package ssr

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"strings"

	"brickplans/internal/db"
)

func mustJSON(v interface{}) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return template.JS(b)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func authorName(u *db.User) string {
	if u == nil {
		return "匿名"
	}
	return u.Username
}

// siteJSONLD returns Organization + WebSite (with SearchAction) — emitted on every page.
// The Organization node is referenced by @id from WebSite.publisher and the
// AboutPage, so every page exposes one canonical site entity to search engines
// and AI answer engines.
func (h *Handler) siteJSONLD() []template.JS {
	public := h.cfg.PublicURL
	orgID := public + "/#organization"
	org := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "Organization",
		"@id":           orgID,
		"name":          "BrickPlan",
		"alternateName": "积木图纸社区",
		"url":           public,
		"description":   "BrickPlan 是面向积木/MOC 爱好者的图纸分享社区，收录乐高及国产积木（双鹰、宇星、高砖、森宝、奇积等）的 MOC 图纸、零件知识、搭建教程与作品评测。",
		"slogan":        "发现和分享积木 MOC 图纸",
		"logo": map[string]interface{}{
			"@type":  "ImageObject",
			"url":    public + "/og-default.png",
			"width":  1200,
			"height": 630,
		},
		"image":      public + "/og-default.png",
		"inLanguage": "zh-CN",
		"areaServed": map[string]interface{}{"@type": "Country", "name": "China"},
		"knowsAbout": []string{"积木图纸", "MOC", "乐高 MOC", "积木搭建教程", "积木零件", "国产积木品牌"},
		"contactPoint": []map[string]interface{}{
			{
				"@type":             "ContactPoint",
				"contactType":       "customer support",
				"email":             "ghostjeky@hotmail.com",
				"url":               public + "/about",
				"availableLanguage": []string{"zh-CN"},
			},
		},
		"mainEntityOfPage": public + "/about",
	}
	website := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "WebSite",
		"@id":           public + "/#website",
		"url":           public,
		"name":          "BrickPlan",
		"alternateName": "积木图纸社区",
		"description":   "积木/MOC 图纸分享社区",
		"inLanguage":    "zh-CN",
		"publisher":     map[string]interface{}{"@id": orgID},
		"potentialAction": map[string]interface{}{
			"@type": "SearchAction",
			"target": map[string]interface{}{
				"@type":       "EntryPoint",
				"urlTemplate": public + "/explore?q={search_term_string}",
			},
			"query-input": "required name=search_term_string",
		},
	}
	return []template.JS{mustJSON(org), mustJSON(website)}
}

// minRatingCount 是输出 AggregateRating 的最低评分条数（工单 SEO-20261026-03 G-1）。
// 结构化数据里的评分必须是真实用户评分：1 条评分就声明一个均值，等于主动告诉
// Google 和 AI 搜索「这条内容只有 1 票、得分 2 分」，比不输出更糟；条数太少也
// 不构成聚合。因此 <3 条一律不输出整个节点。
const minRatingCount = 3

// ratingNode 按 G-1 的规则生成 AggregateRating：条数 <3 或均值 <=0 返回 nil，
// 调用方拿到 nil 就不写进 map —— 整个节点消失。均值保留一位小数。
func ratingNode(ratingCount int, ratingValue float64) map[string]interface{} {
	if ratingCount < minRatingCount || ratingValue <= 0 {
		return nil
	}
	return map[string]interface{}{
		"@type":       "AggregateRating",
		"ratingValue": math.Round(ratingValue*10) / 10,
		"bestRating":  5,
		"worstRating": 1,
		"ratingCount": ratingCount,
	}
}

// detailURLFor 返回对应语言树下的详情页地址。中文 /detail/<id>、英文
// /en/detail/<id>；两棵树的 canonical 各自独立，JSON-LD 的 url/@id 必须与页面
// 所在语言树一致，否则等于给搜索引擎一个指向另一棵树的 @id。
func detailURLFor(public, lang, id string) string {
	if lang == "en" {
		return public + "/en/detail/" + id
	}
	return public + "/detail/" + id
}

// genreLabel 让英文树的 genre 用英文。/en 界面的类目译表就是 categoryEN，
// 两处各写一套迟早跑偏（这条路径上确实出现过「英文页 genre 是『奇幻』」）。
func genreLabel(category, lang string) string {
	if lang == "en" {
		return categoryLabelEN(category)
	}
	return category
}

// blueprintProps 收集图纸的物理属性（颗粒数 / 难度）。这两个值以前分别塞进
// material 和 contentRating —— contentRating 在 schema.org 里表示内容分级
// （PG-13 那类），写「难度 3/5」属于又一处误用；改成 additionalProperty 的
// PropertyValue，语义与前端的「图纸信息」表一致。
func blueprintProps(bp *db.Blueprint, lang string) []map[string]interface{} {
	props := []map[string]interface{}{}
	if bp.PieceCount != nil {
		name := "零件数"
		if lang == "en" {
			name = "Piece count"
		}
		props = append(props, map[string]interface{}{"@type": "PropertyValue", "name": name, "value": *bp.PieceCount})
	}
	if bp.Difficulty != nil {
		name := "难度"
		if lang == "en" {
			name = "Difficulty"
		}
		props = append(props, map[string]interface{}{"@type": "PropertyValue", "name": name, "value": fmt.Sprintf("%d/5", *bp.Difficulty)})
	}
	return props
}

// creativeWorkJSONLD 是详情页的主实体（工单 G-2）：页面描述的是一份「积木搭建
// 图纸/说明书」——CreativeWork，不是可售商品。原先同一页面还并列输出
// Product + Offer{price:"0", priceCurrency:"CNY"}，但我们没有售价、没有购物车，
// 虚假 Offer 属于结构化数据政策风险，已整体删除；商品化的 sku / mpn / brand /
// offers 也换成 CreativeWork 的 identifier / creator。aggregateRating 走 G-1 的
// ratingNode 闸门（无真实评分就不输出）。BreadcrumbList 与 ImageObject 由调用
// 方另行追加，本函数不碰。
func creativeWorkJSONLD(bp *db.Blueprint, cover, public, lang string) template.JS {
	u := detailURLFor(public, lang, bp.ID)
	inLang := "zh-CN"
	if lang == "en" {
		inLang = "en"
	}
	m := map[string]interface{}{
		"@context":             "https://schema.org",
		"@type":                "CreativeWork",
		"@id":                  u,
		"name":                 bp.TitleIn(lang),
		"description":          derefStr(bp.DescriptionIn(lang)),
		"url":                  u,
		"inLanguage":           inLang,
		"isPartOf":             map[string]interface{}{"@type": "WebSite", "name": "BrickPlan", "url": public},
		"author":               map[string]interface{}{"@type": "Person", "name": authorName(bp.Author)},
		"datePublished":        bp.CreatedAt.Format("2006-01-02"),
		"dateModified":         bp.UpdatedAt.Format("2006-01-02"),
		"interactionStatistic": engagementStatistic(bp.ViewCount, bp.LikeCount),
	}
	if cover != "" {
		m["image"] = cover
	}
	if bp.Category != nil {
		// 英文树不能把中文类目原样塞进 genre（G-2「英文页混中文」的一部分）；
		// categoryLabelEN 就是 /en 界面在用的同一张译表，直接复用。
		m["genre"] = genreLabel(*bp.Category, lang)
	}
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		m["identifier"] = strings.TrimSpace(*bp.ProductCode)
	}
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		m["creator"] = map[string]interface{}{"@type": "Organization", "name": strings.TrimSpace(*bp.Brand)}
	}
	if props := blueprintProps(bp, lang); len(props) > 0 {
		m["additionalProperty"] = props
	}
	tags := []string{}
	for _, bt := range bp.Tags {
		if bt.Tag != nil {
			tags = append(tags, bt.Tag.Name)
		}
	}
	if len(tags) > 0 {
		m["keywords"] = strings.Join(tags, ", ")
	}
	// G-1：库内目前没有评分表（blueprints 只有 view_count / like_count），所以
	// 这里恒为 nil、节点不输出。接上真实评分后把两个参数换成真实条数与均值，
	// 规则本身不用再改 —— 这正是「难度当评分」那个 bug 的防线。
	if r := ratingNode(0, 0); r != nil {
		m["aggregateRating"] = r
	}
	return mustJSON(m)
}

// engagementStatistic builds the InteractionCounter array shared by detail and
// list pages. AI answer engines and search engines use these to judge how much
// real usage a design has.
func engagementStatistic(views, likes int) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"@type":                "InteractionCounter",
			"interactionType":      "https://schema.org/ViewAction",
			"userInteractionCount": views,
		},
		{
			"@type":                "InteractionCounter",
			"interactionType":      "https://schema.org/LikeAction",
			"userInteractionCount": likes,
		},
	}
}
// blueprintListItem maps one blueprint to the nested CreativeWork node used
// inside ItemList（G-2：列表项同样不是商品 —— 没有 Offer、没有 price、没有
// sku/mpn）。Image 仍然故意不带：列表 handler 不预加载图片，带上去就是每页
// 一次 N+1。
func blueprintListItem(bp db.Blueprint, public, lang string) map[string]interface{} {
	u := detailURLFor(public, lang, bp.ID)
	inLang := "zh-CN"
	if lang == "en" {
		inLang = "en"
	}
	m := map[string]interface{}{
		"@type":                "CreativeWork",
		"@id":                  u,
		"name":                 bp.TitleIn(lang),
		"url":                  u,
		"description":          derefStr(bp.DescriptionIn(lang)),
		"inLanguage":           inLang,
		"interactionStatistic": engagementStatistic(bp.ViewCount, bp.LikeCount),
	}
	if bp.Category != nil {
		// 英文树不能把中文类目原样塞进 genre（G-2「英文页混中文」的一部分）；
		// categoryLabelEN 就是 /en 界面在用的同一张译表，直接复用。
		m["genre"] = genreLabel(*bp.Category, lang)
	}
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		m["identifier"] = strings.TrimSpace(*bp.ProductCode)
	}
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		m["creator"] = map[string]interface{}{"@type": "Organization", "name": strings.TrimSpace(*bp.Brand)}
	}
	if props := blueprintProps(&bp, lang); len(props) > 0 {
		m["additionalProperty"] = props
	}
	// G-1：列表项与详情页同一条规则，没有真实评分就不输出 aggregateRating。
	if r := ratingNode(0, 0); r != nil {
		m["aggregateRating"] = r
	}
	return m
}

func breadcrumbJSONLD(category, title, public string) template.JS {
	items := []map[string]interface{}{
		{"@type": "ListItem", "position": 1, "name": "首页", "item": public + "/"},
	}
	pos := 2
	if category != "" {
		items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": category, "item": public + "/explore?category=" + category})
		pos++
	}
	items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": title})
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// breadcrumbJSONLDEN is the English variant for /en/detail: Home → category →
// title, with the category step pointing into the English explore tree.
func breadcrumbJSONLDEN(category, title, public string) template.JS {
	items := []map[string]interface{}{
		{"@type": "ListItem", "position": 1, "name": "Home", "item": public + "/en/"},
	}
	pos := 2
	if category != "" {
		items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": category, "item": public + "/en/explore?category=" + url.QueryEscape(category)})
		pos++
	}
	items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": title})
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// itemListJSONLD emits the ItemList for listing pages (home/explore/tags).
// Each entry nests a full Product node so list pages carry the same GEO signal
// (free offer, rating, interaction counters) as detail pages.
func itemListJSONLD(bps []db.Blueprint, public string) template.JS {
	return itemListJSONLDLang(bps, public, "zh")
}

// itemListJSONLDLang is itemListJSONLD for one site language: titles and
// descriptions use the English fields when they exist, while the URLs stay in
// the tree the caller chose (the /en/ pages still canonicalise to the Chinese
// detail URLs, so their ItemList links there too).
func itemListJSONLDLang(bps []db.Blueprint, public, lang string) template.JS {
	return itemListJSONLDURL(bps, public, lang, func(id string) string { return public + "/detail/" + id })
}

// itemListJSONLDEN is the English-tree variant: same shape as itemListJSONLD
// (Product/Offer/AggregateRating per item) but every URL is in the /en/detail
// space and the Offer currency is USD — the page audience is the international
// reader and USD is the only currency that tree actually implies. The CN tree
// keeps CNY, so the two ItemLists never claim different currencies for the
// same blueprint.
func itemListJSONLDEN(bps []db.Blueprint, public string) template.JS {
	return itemListJSONLDURL(bps, public, "en", func(id string) string { return public + "/en/detail/" + id })
}

func itemListJSONLDURL(bps []db.Blueprint, public, lang string, urlFor func(id string) string) template.JS {
	items := make([]map[string]interface{}, 0, len(bps))
	for i, bp := range bps {
		u := urlFor(bp.ID)
		item := blueprintListItem(bp, public, lang)
		// Only the URL needs localising now: the node carries no Offer (G-2),
		// and @id already follows the same language tree. Title/description
		// stay in their published language, matching the visible copy.
		item["url"] = u
		item["@id"] = u
		items = append(items, map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     bp.TitleIn(lang),
			"url":      u,
			"item":     item,
		})
	}
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "ItemList",
		"numberOfItems":   len(items),
		"itemListOrder":   "https://schema.org/ItemListOrderDescending",
		"itemListElement": items,
	})
}

func profileJSONLD(u *db.User, bpCount int, public string) template.JS {
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "ProfilePage",
		"url":         public + "/user/" + u.ID,
		"name":        u.Username,
		"description": derefStr(u.Bio),
		"mainEntity": map[string]interface{}{
			"@type":         "Person",
			"name":          u.Username,
			"contributions": bpCount,
		},
	})
}

func faqJSONLD(qa []QA) template.JS {
	entities := make([]map[string]interface{}, 0, len(qa))
	for _, item := range qa {
		entities = append(entities, map[string]interface{}{
			"@type": "Question",
			"name":  item.Q,
			"acceptedAnswer": map[string]interface{}{
				"@type": "Answer",
				"text":  item.A,
			},
		})
	}
	return mustJSON(map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "FAQPage",
		"mainEntity": entities,
	})
}
