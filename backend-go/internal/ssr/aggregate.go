package ssr

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/db"
	"brickplans/internal/taxonomy"
)

// ── 聚合落地页（工单 SEO-20261024）────────────────────────────────────────
//
// /category/{slug}, /brand/{slug}, /difficulty/{n} + their /en twins.
//
// Before this file the six categories were reachable only as
// /explore?category=场景, and every one of those parameter URLs canonicalised
// back to /explore (handler.go), so none of them could ever rank for its own
// long tail; sitemap-zh advertised 36 of those URLs, and 2,134 detail pages
// pointed their breadcrumb at one of them. The pages here are the replacement:
// stable paths, self-canonical, self-describing (count in title/H1/H2), with a
// full hreflang cluster to the /en twin.

const aggPageSize = 48

// aggHotCount is the size of the 热门 / 最新 blocks. Both are cross-links, not
// the main list: they must not repeat all 48 results, or the page turns into a
// keyword-stuffed wall with no crawl priority.
const aggHotCount = 12

// aggSpec is one resolved landing page. Every heading, title and description is
// composed from these fields, so title/H1/H2 can never disagree about what the
// page is about.
type aggSpec struct {
	Entry taxonomy.Entry
	// ZhNoun / EnNoun carry the page name: 场景类积木图纸 / Scene LEGO & Brick Blueprints.
	ZhNoun, EnNoun string
	// ZhShort / EnShort carry the short name used in count lines and headings:
	// 场景类图纸 / Scene blueprints.
	ZhShort, EnShort string
	// ZhHot is the same noun without the 类 suffix: the ticket asks for
	// 热门场景图纸, not 热门场景类图纸.
	ZhHot string
}

func newAggSpec(e taxonomy.Entry) aggSpec {
	switch e.Family {
	case taxonomy.Difficulty:
		return aggSpec{
			Entry:   e,
			ZhNoun:  "难度 " + e.Value + " 积木图纸",
			EnNoun:  "Difficulty " + e.Value + " Brick Blueprints",
			ZhShort: "难度 " + e.Value + " 图纸",
			ZhHot:   "难度 " + e.Value + " 图纸",
			EnShort: "Difficulty " + e.Value + " blueprints",
		}
	case taxonomy.Brand:
		return aggSpec{
			Entry:   e,
			ZhNoun:  e.Value + "品牌积木图纸",
			EnNoun:  e.LabelEN + " Brick Blueprints",
			ZhShort: e.Value + "图纸",
			ZhHot:   e.Value + "图纸",
			EnShort: e.LabelEN + " blueprints",
		}
	default:
		return aggSpec{
			Entry:   e,
			ZhNoun:  e.Value + "类积木图纸",
			EnNoun:  e.LabelEN + " LEGO & Brick Blueprints",
			ZhShort: e.Value + "类图纸",
			ZhHot:   e.Value + "图纸",
			EnShort: e.LabelEN + " blueprints",
		}
	}
}

// aggPath builds the page URL. page<=1 is the clean, canonical URL; page>1 adds
// ?page=N because pagination pages are self-canonical (ticket 3.2).
func aggPath(lang string, e taxonomy.Entry, page int) string {
	p := langPath(lang, taxonomy.Path(e))
	if page > 1 {
		return p + "?page=" + strconv.Itoa(page)
	}
	return p
}

func (s aggSpec) title(lang string, total int64) string {
	if lang == "en" {
		return fmt.Sprintf("%s (%d) — BrickPlan", s.EnNoun, total)
	}
	return fmt.Sprintf("%s（%d 个）— BrickPlan", s.ZhNoun, total)
}

// zhBrowse / enBrowse name the shortcut blocks that follow the list. They differ
// per family (a category page links brands first, a brand page links categories
// first), so the description sentence and the actual H2 blocks stay in step.
func (s aggSpec) zhBrowse() string {
	switch s.Entry.Family {
	case taxonomy.Brand:
		return "分类、难度与其他品牌"
	case taxonomy.Difficulty:
		return "分类、品牌与其他难度"
	default:
		return "品牌、难度与其他分类"
	}
}

func (s aggSpec) enBrowse() string {
	switch s.Entry.Family {
	case taxonomy.Brand:
		return "category, difficulty and other brands"
	case taxonomy.Difficulty:
		return "category, brand and other difficulty levels"
	default:
		return "brand, difficulty and other categories"
	}
}

func (s aggSpec) description(lang string, total int64) string {
	if lang == "en" {
		return fmt.Sprintf("%d %s on BrickPlan — hottest builds, latest uploads, and shortcuts by %s.",
			total, s.EnShort, s.enBrowse())
	}
	return fmt.Sprintf("%s共 %d 个，含热门作品与最新上传，可按%s继续浏览。",
		s.ZhShort, total, s.zhBrowse())
}

// ── 面包屑（可见 + JSON-LD 同源）──────────────────────────────────────────

func (s aggSpec) crumbs(lang string, public string) []crumbItem {
	home, explore, _ := crumbsFor(lang)
	homeHref := public + langPath(lang, "/")
	name := s.ZhNoun
	if lang == "en" {
		name = s.EnNoun
	}
	// 首页 › 探索 › 当前页. The middle level is kept so the hub links back into
	// the browsable index (and so the JSON-LD trail has an item URL for every
	// level except the page itself, as on the detail pages).
	return []crumbItem{
		{Name: home, Href: homeHref},
		{Name: explore, Href: public + langPath(lang, "/explore")},
		{Name: name},
	}
}

func (s aggSpec) breadcrumbHTML(lang string, public string) string {
	_, _, aria := crumbsFor(lang)
	items := s.crumbs(lang, public)
	parts := make([]string, 0, len(items))
	for i, c := range items {
		switch {
		case c.Href != "":
			parts = append(parts, fmt.Sprintf(`<a href="%s">%s</a>`, esc(c.Href), esc(c.Name)))
		case i == len(items)-1:
			parts = append(parts, fmt.Sprintf(`<span class="crumb-current">%s</span>`, esc(c.Name)))
		default:
			parts = append(parts, fmt.Sprintf(`<span>%s</span>`, esc(c.Name)))
		}
	}
	return fmt.Sprintf(`<nav class="breadcrumb" aria-label="%s">%s</nav>`, esc(aria), strings.Join(parts, " › "))
}

func (s aggSpec) breadcrumbJSONLD(lang string, public string) template.JS {
	items := make([]map[string]interface{}, 0, 2)
	for i, c := range s.crumbs(lang, public) {
		e := map[string]interface{}{"@type": "ListItem", "position": i + 1, "name": c.Name}
		if c.Href != "" {
			e["item"] = c.Href
		}
		items = append(items, e)
	}
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// ── 正文 ────────────────────────────────────────────────────────────────

// aggBody is the server-rendered body. Section order is the ticket contract:
// count → the 48 results → hottest → latest → brand → difficulty → other
// categories. The 48 results sit directly under the count so a reader (and a
// crawler ranking the page for "场景 积木图纸") hits real links in the first
// screen instead of a wall of navigation.
func aggBody(lang string, s aggSpec, public string, total int64, page, pages int, items, hot, latest []db.Blueprint) template.HTML {
	en := lang == "en"
	// Parts are joined with "\n" on purpose: the HTML stays readable in
	// view-source, and the acceptance check for pagination can slice the result
	// block with a line-based tool (`sed -n '/class="agg-results"/,/^<\/div>/p'`)
	// instead of guessing where the list ends.
	parts := []string{"<article>"}
	parts = append(parts, s.breadcrumbHTML(lang, public))
	parts = append(parts, fmt.Sprintf("<h1>%s</h1>", esc(pick(en, s.EnNoun, s.ZhNoun))))

	// 首屏统计：条数 + 分页位置，H1 之下第一段，爬虫不用滚动就能读到总量。
	if en {
		parts = append(parts, fmt.Sprintf(`<p class="agg-stat">%d %s · page %d / %d · %d per page</p>`,
			total, s.EnShort, page, pages, aggPageSize))
	} else {
		parts = append(parts, fmt.Sprintf(`<p class="agg-stat">%s共 %d 个 · 第 %d / %d 页 · 每页 %d 个</p>`,
			s.ZhShort, total, page, pages, aggPageSize))
	}

	// H2 #1
	if en {
		parts = append(parts, fmt.Sprintf("<h2>%d %s</h2>", total, s.EnShort))
	} else {
		parts = append(parts, fmt.Sprintf("<h2>%s共 %d 个</h2>", s.ZhShort, total))
	}

	// 结果区单独包一层：热门/最新两块在所有分页上重复，是全页 detail 链接的
	// 固定重叠源。没有这个边界，"page1 ∩ page2" 只能测出热点块的重复，测不出
	// 分页是否正确——验收 A4 取的正是这个 div 里的链接集合。
	parts = append(parts, `<div class="agg-results">`)
	if len(items) == 0 {
		parts = append(parts, fmt.Sprintf("<p>%s</p>", esc(pick(en, "No blueprints on this page.", "这一页没有图纸。"))))
	} else {
		parts = append(parts, string(listNoscriptFor(lang, items, public)))
	}
	parts = append(parts, aggPager(lang, s, public, page, pages))
	parts = append(parts, "</div>")

	// H2 #2 热门
	parts = append(parts, fmt.Sprintf("<h2>%s</h2>", esc(pick(en, "Popular "+s.EnShort, "热门"+s.ZhHot))))
	parts = append(parts, string(listNoscriptFor(lang, hot, public)))

	// H2 #3 最新上传
	parts = append(parts, fmt.Sprintf("<h2>%s</h2>", esc(pick(en, "Latest uploads", "最新上传"))))
	parts = append(parts, string(listNoscriptFor(lang, latest, public)))

	// H2 #4/#5/#6：横向入口，全部指向同语言的聚合页（英文页不留中文树的链接，
	// 否则英文页面会给中文 URL 传权重，hreflang 集群反而变糊）。
	parts = append(parts, aggLinkBlock(lang, s, public, taxonomy.Brand, "按品牌浏览", "Browse by brand"))
	parts = append(parts, aggLinkBlock(lang, s, public, taxonomy.Difficulty, "按难度浏览", "Browse by difficulty"))
	parts = append(parts, aggLinkBlock(lang, s, public, taxonomy.Category, "其他分类", "Other categories"))
	parts = append(parts, "</article>")
	return template.HTML(strings.Join(parts, "\n"))
}

// aggLinkBlock renders one "browse by X" block. The heading follows the family:
// on a brand page the category block is the primary one ("按分类浏览"), and the
// block that would link back to the current family is the "other" one.
func aggLinkBlock(lang string, s aggSpec, public string, fam taxonomy.Family, zhLabel, enLabel string) string {
	entries := taxonomy.InFamily(fam, "")
	if fam == s.Entry.Family {
		// "other" block: exclude the page we are on, and rename it.
		entries = taxonomy.InFamily(fam, s.Entry.Slug)
		switch fam {
		case taxonomy.Brand:
			zhLabel, enLabel = "其他品牌", "Other brands"
		case taxonomy.Difficulty:
			zhLabel, enLabel = "其他难度", "Other difficulty levels"
		default:
			zhLabel, enLabel = "其他分类", "Other categories"
		}
	} else if s.Entry.Family != taxonomy.Category && fam == taxonomy.Category {
		// A brand/difficulty page links all six categories, not "the other five".
		zhLabel, enLabel = "按分类浏览", "Browse by category"
	}
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<h2>%s</h2><ul class=\"agg-links\">", esc(pick(lang == "en", enLabel, zhLabel))))
	for _, e := range entries {
		name := e.Value
		if lang == "en" {
			name = e.LabelEN
		}
		if fam == taxonomy.Difficulty && lang == "en" {
			name = e.LabelEN
		}
		if fam == taxonomy.Difficulty && lang != "en" {
			name = "难度 " + e.Value
		}
		b.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`,
			esc(public+langPath(lang, taxonomy.Path(e))), esc(name)))
	}
	b.WriteString("</ul>")
	return b.String()
}

func aggPager(lang string, s aggSpec, public string, page, pages int) string {
	if pages <= 1 {
		return ""
	}
	en := lang == "en"
	var b strings.Builder
	b.WriteString(`<nav class="pager">`)
	if page > 1 {
		b.WriteString(fmt.Sprintf(`<a rel="prev" href="%s">%s</a>`,
			esc(public+aggPath(lang, s.Entry, page-1)), esc(pick(en, "Previous", "上一页"))))
	}
	b.WriteString(fmt.Sprintf(`<span>%d / %d</span>`, page, pages))
	if page < pages {
		b.WriteString(fmt.Sprintf(`<a rel="next" href="%s">%s</a>`,
			esc(public+aggPath(lang, s.Entry, page+1)), esc(pick(en, "Next", "下一页"))))
	}
	b.WriteString("</nav>")
	return b.String()
}

func pick(en bool, a, b string) string {
	if en {
		return a
	}
	return b
}

// ── handlers ────────────────────────────────────────────────────────────

// Category renders /category/{slug}; Brand and Difficulty are the same page with
// a different filter. The EN variants are separate handlers rather than one
// handler reading a path prefix, matching how every other /en page is wired.
func (h *Handler) Category(c *gin.Context)   { h.aggregate(c, "zh", taxonomy.Category, c.Param("slug")) }
func (h *Handler) Brand(c *gin.Context)      { h.aggregate(c, "zh", taxonomy.Brand, c.Param("slug")) }
func (h *Handler) Difficulty(c *gin.Context) { h.aggregate(c, "zh", taxonomy.Difficulty, c.Param("n")) }

func (h *Handler) CategoryEN(c *gin.Context) {
	h.aggregate(c, "en", taxonomy.Category, c.Param("slug"))
}
func (h *Handler) BrandEN(c *gin.Context) { h.aggregate(c, "en", taxonomy.Brand, c.Param("slug")) }
func (h *Handler) DifficultyEN(c *gin.Context) {
	h.aggregate(c, "en", taxonomy.Difficulty, c.Param("n"))
}

// aggregate renders one landing page in one language.
//
// Out-of-range ?page=N returns 200 with an empty list (ticket 3.2 forbids 404:
// the URL is still a real page, just past the end). An unknown slug is a real
// 404, because those URLs must never be indexed.
func (h *Handler) aggregate(c *gin.Context, lang string, family taxonomy.Family, key string) {
	e, ok := taxonomy.Lookup(family, key)
	if !ok {
		if lang == "en" {
			h.NotFoundEN(c)
			return
		}
		h.NotFound(c)
		return
	}
	s := newAggSpec(e)
	public := h.cfg.PublicURL

	page := 1
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 1 {
			page = n
		}
	}

	column, value := aggFilter(e)
	// A fresh query per call: GORM statements carry their clauses forward, so
	// reusing one *gorm.DB for count + three finds leaks LIMIT/OFFSET into the
	// count and returns the wrong total on every page after the first.
	query := func() *gorm.DB {
		return h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", true).Where(column, value)
	}

	var total int64
	query().Count(&total)
	pages := int((total + aggPageSize - 1) / aggPageSize)
	if pages < 1 {
		pages = 1
	}

	var items, hot, latest []db.Blueprint
	if page <= pages {
		query().Order("created_at DESC, id ASC").
			Limit(aggPageSize).Offset((page - 1) * aggPageSize).Find(&items)
	}
	query().Order("view_count DESC, id ASC").Limit(aggHotCount).Find(&hot)
	query().Order("created_at DESC, id ASC").Limit(aggHotCount).Find(&latest)

	canonical := public + aggPath(lang, e, page)
	jsonld := h.siteJSONLD()
	if lang == "en" {
		jsonld = append(jsonld, webPageJSONLD(public, taxonomy.Path(e),
			s.EnNoun, s.description("en", total), "en"))
	}
	jsonld = append(jsonld, itemListJSONLDLang(items, public, lang))
	jsonld = append(jsonld, s.breadcrumbJSONLD(lang, public))

	data := PageData{
		Lang:        lang,
		Title:       s.title(lang, total),
		Description: s.description(lang, total),
		Canonical:   canonical,
		// The Chinese page is the x-default: it is the one that carries the
		// stored Chinese values and the one the sitemap has been advertising.
		Alternates: []AltLink{
			{Hreflang: "zh-CN", Href: public + aggPath("zh", e, page)},
			{Hreflang: "en", Href: public + aggPath("en", e, page)},
			{Hreflang: "x-default", Href: public + aggPath("zh", e, page)},
		},
		OGType:   "website",
		JSONLD:   jsonld,
		Noscript: aggBody(lang, s, public, total, page, pages, items, hot, latest),
	}
	if page > 1 {
		data.PrevURL = public + aggPath(lang, e, page-1)
	}
	if page < pages {
		data.NextURL = public + aggPath(lang, e, page+1)
	}
	if lang == "en" {
		data.BodyWrap = true
	}
	h.r.Render(c, data)
}

// aggFilter maps a landing page to its SQL predicate. Difficulty is stored as a
// number, so it is converted rather than compared as a string (MySQL would do it
// implicitly, SQLite would not).
func aggFilter(e taxonomy.Entry) (string, interface{}) {
	switch e.Family {
	case taxonomy.Category:
		return "category = ?", e.Value
	case taxonomy.Brand:
		return "brand = ?", e.Value
	default:
		n, _ := strconv.Atoi(e.Value)
		return "difficulty = ?", n
	}
}
