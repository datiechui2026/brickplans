package ssr

import (
	"html/template"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
	"brickplans/internal/db"
)

// categoryEN maps the stored Chinese category value to the English label the
// English UI shows. The VALUE stays Chinese everywhere it is sent back to the
// API (filters, forms) — only display copy is translated.
//
// The first six are the taxonomy the site advertises on /explore. The rest is
// what the table actually holds (SELECT DISTINCT category, 2026-10-24): uploads
// arrive tagged with LEGO themes, so 其他 alone covers 1,801 of 2,135 rows and
// the English tree used to print it untranslated.
var categoryEN = map[string]string{
	"建筑": "Architecture",
	"车辆": "Vehicles",
	"机甲": "Mecha",
	"奇幻": "Fantasy",
	"科幻": "Sci-Fi",
	"场景": "Scenery",
	"其他": "Other",
	"星战": "Star Wars",
	"城市": "City",
	"朋友": "Friends",
	"图标": "Icons",
	"机械": "Technic",
	"创意": "Creator",
	"赛车": "Speed Champions",
}

func categoryLabelEN(cat string) string {
	if en, ok := categoryEN[cat]; ok {
		return en
	}
	return cat
}

// breadcrumbJSONLDFor is breadcrumbJSONLD with English labels and an English
// trail for /en pages. Every crumb stays inside the /en tree: a trail whose
// first item is /en/ but whose second item is the Chinese /explore would claim
// a hierarchy that does not exist. /en/explore?category=<stored value> is a real
// page, and the Chinese value in the query is the filter the API expects.
func breadcrumbJSONLDFor(category, title, public, lang string) template.JS {
	if lang != "en" {
		return breadcrumbJSONLD(category, title, public)
	}
	items := []map[string]interface{}{
		{"@type": "ListItem", "position": 1, "name": "Home", "item": public + "/en/"},
	}
	pos := 2
	if category != "" {
		items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos,
			"name": categoryLabelEN(category), "item": public + "/en/explore?category=" + url.QueryEscape(category)})
		pos++
	}
	items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": title})
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// ── 英文站页面（/en/、/en/explore、/en/detail/:id、/en/user/:id、/en/tags/:name…）
//
// 设计原则（2026-10 改版）：
//   - 每个 EN 路由都由 Go 输出完整正文 → 非 JS 爬虫（GPTBot/ClaudeBot/…）与
//     Googlebot 首屏都能读到内容，SEO/GEO 不依赖前端。
//   - SPA 之后接管同一套页面并渲染同样的 CN 版式（见 frontend/src/main.js）。
//   - 正文与 CN 页同构，只是标签走英文；用户内容（标题/描述/标签）按原文展示。
//   - 与中文页内容相同的 UGC 页面（detail/user/tags）canonical 回中文 URL，
//     不输出 hreflang、不进 sitemap —— 它们是英文 UI 变体，不是译文，不能被
//     当成两份独立内容收录。

// enHomeBlogLimit caps the English homepage's article module. It is much higher
// than homeBlogLimit (12) on purpose — see the comment in HomeEN.
const enHomeBlogLimit = 60

// webPageJSONLD 是英文静态页的通用 WebPage 节点。inLanguage 必须跟页面语言走，
// 否则又是一个"英文页声明 zh-CN"的错误标注。
func webPageJSONLD(public, path, name, description, lang string) template.JS {
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "WebPage",
		"@id":         public + path + "#webpage",
		"url":         public + path,
		"name":        name,
		"description": description,
		"inLanguage":  langFor(lang).HTMLLang,
		"isPartOf":    map[string]interface{}{"@type": "WebSite", "@id": public + "/#website"},
	})
}

// aboutPageJSONLDFor is aboutPageJSONLD with the page language threaded through
// (the Chinese default stays for /about).
func aboutPageJSONLDFor(public, lang string) template.JS {
	if lang != "en" {
		return aboutPageJSONLD(public)
	}
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "AboutPage",
		"@id":         public + "/en/about#aboutpage",
		"url":         public + "/en/about",
		"name":        "About BrickPlan",
		"description": "Background, team, contact details and the content policy behind the BrickPlan brick/MOC blueprint community.",
		"inLanguage":  "en",
		"isPartOf":    map[string]interface{}{"@type": "WebSite", "@id": public + "/#website"},
		"about":       map[string]interface{}{"@id": public + "/#organization"},
		"mainEntity":  map[string]interface{}{"@id": public + "/#organization"},
	})
}

// HomeEN renders the English home page at /en/.
func (h *Handler) HomeEN(c *gin.Context) {
	featured, latest := h.homeBlueprints()

	var posts []blog.Post
	if h.blogStore != nil {
		posts = h.blogStore.AllByLang(blog.LangEN)
		// The Chinese home caps its blog module at homeBlogLimit. The English home
		// deliberately lists the WHOLE English tree (up to enHomeBlogLimit): the
		// homepage is the strongest internal link source on the site, and the
		// English tree is small — listing every article there is what gives each
		// /en/blog/<slug> a referrer instead of leaving it sitemap-only.
		//
		// "Whole tree" now means the *released* tree: an article the exposure
		// ledger has not released answers noindex, and linking it from the
		// homepage spends crawl budget and PageRank on a URL we deliberately do
		// not advertise. Same filter as /en/blog, so nav and sitemap agree.
		posts = blog.FilterVisible(posts, blog.LangEN, h.exposure)
		if len(posts) > enHomeBlogLimit {
			posts = posts[:enHomeBlogLimit]
		}
	}
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, webPageJSONLD(h.cfg.PublicURL, "/en/",
		"BrickPlan — MOC Blueprints, Building Guides and Brick Reviews",
		"Browse MOC blueprints and read English guides on brick parts, brands, techniques and set reviews.", "en"))
	jsonld = append(jsonld, itemListJSONLDLang(latest, h.cfg.PublicURL, "en"))

	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "BrickPlan — MOC Blueprints, Building Guides and Brick Reviews",
		Description: "Browse MOC blueprints and read English guides on brick parts, brand comparisons, building techniques and set reviews.",
		Canonical:   h.cfg.PublicURL + "/en/",
		OGType:      "website",
		JSONLD:      jsonld,
		BodyWrap:    true,
		Noscript:    homeBodyFor("en", featured, latest, posts, h.cfg.PublicURL, "/en/blog"),
	})
}

// AboutEN renders the English about page at /en/about.
func (h *Handler) AboutEN(c *gin.Context) {
	public := h.cfg.PublicURL
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, aboutPageJSONLDFor(public, "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "About BrickPlan — MOC blueprint community",
		Description: "Background, team, contact details and the content policy behind the BrickPlan brick/MOC blueprint community.",
		Canonical:   public + "/en/about",
		OGType:      "website",
		JSONLD:      jsonld,
		BodyClass:   "blog-detail",
		Noscript:    markdownBody(aboutNoscriptFor("en")),
	})
}

// FAQEN renders the English FAQ page at /en/faq.
func (h *Handler) FAQEN(c *gin.Context) {
	qa := faqDataFor("en")
	jsonld := h.siteJSONLD()
	// withInLanguage is the same helper the blog tree uses for its FAQPage block:
	// it injects the page language into an already-serialized node.
	jsonld = append(jsonld, withInLanguage(faqJSONLD(qa), "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "FAQ — BrickPlan",
		Description: "Frequently asked questions about BrickPlan: what a MOC is, how to upload, supported file types, copyright and reporting.",
		Canonical:   h.cfg.PublicURL + "/en/faq",
		OGType:      "website",
		JSONLD:      jsonld,
		BodyClass:   "blog-detail",
		Noscript:    markdownBody(faqNoscriptFor("en", qa)),
	})
}

// PrivacyEN renders the English privacy page at /en/privacy.
func (h *Handler) PrivacyEN(c *gin.Context) {
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, webPageJSONLD(h.cfg.PublicURL, "/en/privacy",
		"Privacy Policy", "What BrickPlan stores, why, and what you can do about it.", "en"))
	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "Privacy Policy — BrickPlan",
		Description: "What BrickPlan stores, why we store it, which third parties are involved, and how to have your data removed.",
		Canonical:   h.cfg.PublicURL + "/en/privacy",
		OGType:      "website",
		JSONLD:      jsonld,
		BodyClass:   "blog-detail",
		Noscript:    markdownBody(privacyBodyEN()),
	})
}

// ExploreEN renders /en/explore. It used to be a text shell that sent English
// readers back to the Chinese catalogue; it now renders the same crawlable list
// the Chinese /explore page does, with English labels (blueprint titles stay as
// their authors wrote them).
func (h *Handler) ExploreEN(c *gin.Context) {
	cat := c.Query("category")
	brand := strings.TrimSpace(c.Query("brand"))
	var bps []db.Blueprint
	qry := h.gdb.Where("is_published = ?", true)
	if cat != "" {
		qry = qry.Where("category = ?", cat)
	}
	if brand != "" {
		// Same stored value the SPA sends (乐高/森宝/...), so the SSR body a
		// crawler receives matches the filtered view the user lands on.
		qry = qry.Where("brand = ?", brand)
	}
	qry.Order("created_at DESC").Limit(50).Find(&bps)

	title := "Explore MOC blueprints — BrickPlan"
	desc := "Browse community brick blueprints — architecture, vehicles, mecha, fantasy, sci-fi and scenery — by category, tag or keyword."
	if cat != "" {
		title = categoryLabelEN(cat) + " blueprints — BrickPlan"
		desc = "Browse community brick blueprints in the " + categoryLabelEN(cat) + " category."
	}

	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, webPageJSONLD(h.cfg.PublicURL, "/en/explore",
		"Explore MOC blueprints", "Browse community brick blueprints by category, tag and keyword.", "en"))
	jsonld = append(jsonld, itemListJSONLDLang(bps, h.cfg.PublicURL, "en"))

	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       title,
		Description: desc,
		Canonical:   h.cfg.PublicURL + "/en/explore",
		OGType:      "website",
		JSONLD:      jsonld,
		BodyWrap:    true,
		Noscript:    markdownBody(listNoscriptFor("en", bps, h.cfg.PublicURL)),
	})
}

// blueprintHasEnglish reports whether the row carries a real English title,
// i.e. cmd/translate-blueprints (or the author) supplied one. Without it both
// trees render the Chinese text and /en/detail stays a UI variant.
func blueprintHasEnglish(bp *db.Blueprint) bool {
	return bp.TitleEN != nil && strings.TrimSpace(*bp.TitleEN) != ""
}

// blueprintDetailAlternates is the hreflang cluster for a translated blueprint.
// It must stay identical to the altLink set sitemap-en emits for the same URL
// (handler/seo.go sitemapSite) — a one-sided pair gets the whole cluster
// discarded, and x-default follows the sitemap: the English page.
func blueprintDetailAlternates(id, public string) []AltLink {
	zhURL := public + "/detail/" + id
	enURL := public + "/en/detail/" + id
	return []AltLink{
		{"en", enURL},
		{"zh-CN", zhURL},
		{"x-default", enURL},
	}
}

// DetailEN renders /en/detail/:id.
//
// Before 2026-10-24 every row's English page carried the author's Chinese text,
// so it was a UI variant: canonical to /detail/:id, no hreflang. That premise is
// gone — 2,135 rows now have real English titles and descriptions, so a
// translated row is a genuine alternate: self-canonical, in the hreflang
// cluster, and (already) listed in sitemap-en.xml. Rows the translator has not
// reached keep the old consolidation behaviour; announcing them as English
// would recreate exactly the duplicate this page used to avoid.
func (h *Handler) DetailEN(c *gin.Context) {
	id := c.Param("id")
	var bp db.Blueprint
	if err := h.gdb.Preload("Author").Preload("Images", db.OrderImages).Preload("Tags.Tag").First(&bp, "id = ?", id).Error; err != nil {
		h.NotFound(c)
		return
	}
	if !bp.IsPublished {
		h.NotFound(c)
		return
	}
	cover := coverURL(bp.Images, h.cfg.PublicURL)
	titleEN := bp.TitleIn("en")
	desc := "BrickPlan MOC blueprint: " + titleEN
	if d := bp.DescriptionIn("en"); d != nil && *d != "" {
		desc = *d
	}
	category := ""
	if bp.Category != nil {
		category = *bp.Category
	}

	var relatedBps []db.Blueprint
	rq := h.gdb.Where("is_published = ? AND id <> ?", true, id)
	if category != "" {
		rq = rq.Where("category = ?", category)
	} else {
		rq = rq.Where("author_id = ?", bp.AuthorID)
	}
	rq.Preload("Images", db.OrderImages).Order("view_count DESC").Limit(4).Find(&relatedBps)

	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, creativeWorkJSONLD(&bp, cover, h.cfg.PublicURL, "en"))
	jsonld = append(jsonld, productJSONLD(&bp, cover, h.cfg.PublicURL, "en"))
	jsonld = append(jsonld, breadcrumbJSONLDFor(category, titleEN, h.cfg.PublicURL, "en"))

	// Translated rows are alternates of the Chinese original: self-canonical and
	// in the hreflang cluster. Untranslated rows stay consolidated into
	// /detail/:id with no annotations (see the func comment).
	canonical := h.cfg.PublicURL + "/detail/" + bp.ID
	var alts []AltLink
	noHreflang := true
	if blueprintHasEnglish(&bp) {
		canonical = h.cfg.PublicURL + "/en/detail/" + bp.ID
		alts = blueprintDetailAlternates(bp.ID, h.cfg.PublicURL)
		noHreflang = false
	}

	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       titleEN + " — BrickPlan MOC blueprint",
		Description: truncate(desc, 160),
		Canonical:   canonical,
		OGUrl:       h.cfg.PublicURL + "/en/detail/" + bp.ID,
		OGType:      "article",
		OGImage:     cover,
		JSONLD:      jsonld,
		Alternates:  alts,
		NoHreflang:  noHreflang,
		BodyWrap:    true,
		Noscript:    detailNoscriptFor("en", &bp, cover, h.cfg.PublicURL, relatedBps),
	})
}

// TagEN renders /en/tags/:name with the same consolidation rules as DetailEN.
func (h *Handler) TagEN(c *gin.Context) {
	name := c.Param("name")
	var bps []db.Blueprint
	h.gdb.Where("is_published = ? AND id IN (SELECT bt.blueprint_id FROM blueprint_tags bt JOIN tags t ON t.id = bt.tag_id WHERE t.name = ?)", true, name).
		Order("created_at DESC").Limit(50).Find(&bps)

	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, itemListJSONLDLang(bps, h.cfg.PublicURL, "en"))

	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       "#" + name + " tag — BrickPlan",
		Description: "Brick and MOC blueprints tagged “" + name + "”.",
		Canonical:   h.cfg.PublicURL + "/tags/" + name,
		OGUrl:       h.cfg.PublicURL + "/en/tags/" + name,
		OGType:      "website",
		JSONLD:      jsonld,
		NoHreflang:  true,
		BodyWrap:    true,
		Noscript: template.HTML("<h1>#"+esc(name)+"</h1>") +
			listNoscriptFor("en", bps, h.cfg.PublicURL),
	})
}

// UserEN renders /en/user/:id with the same consolidation rules as DetailEN.
func (h *Handler) UserEN(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	if err := h.gdb.Where("id = ? OR username = ?", id, id).First(&user).Error; err != nil {
		h.NotFound(c)
		return
	}
	var bps []db.Blueprint
	h.gdb.Where("author_id = ? AND is_published = ?", user.ID, true).Order("created_at DESC").Limit(50).Find(&bps)
	var bpCount int64
	h.gdb.Model(&db.Blueprint{}).Where("author_id = ? AND is_published = ?", user.ID, true).Count(&bpCount)

	h.r.Render(c, PageData{
		Lang:        "en",
		Title:       user.Username + "'s builds — BrickPlan",
		Description: "Browse brick and MOC blueprints shared by " + user.Username + ".",
		Canonical:   h.cfg.PublicURL + "/user/" + user.ID,
		OGUrl:       h.cfg.PublicURL + "/en/user/" + user.ID,
		OGType:      "profile",
		JSONLD:      append(h.siteJSONLD(), profileJSONLD(&user, int(bpCount), h.cfg.PublicURL)),
		NoHreflang:  true,
		BodyWrap:    true,
		Noscript: template.HTML("<h1>"+esc(user.Username)+"’s builds</h1>") +
			listNoscriptFor("en", bps, h.cfg.PublicURL),
	})
}

// SimpleEN renders an English utility page (upload/edit/notifications/admin).
// It is noindex: these pages are private or meaningless to a crawler, but the
// SPA still needs a server-rendered shell so a direct visit or reload works.
func (h *Handler) SimpleEN(title, desc string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.r.Render(c, PageData{
			Lang:        "en",
			Title:       title + " — BrickPlan",
			Description: desc,
			Canonical:   h.cfg.PublicURL + c.Request.URL.Path,
			OGType:      "website",
			JSONLD:      h.siteJSONLD(),
			Noindex:     true,
			NoHreflang:  true,
			BodyWrap:    true,
			Noscript:    template.HTML("<h1>" + esc(title) + "</h1><p>" + esc(desc) + "</p>"),
		})
	}
}
