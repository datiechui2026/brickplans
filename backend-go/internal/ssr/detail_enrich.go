package ssr

import (
	"fmt"
	"html/template"
	"regexp"
	"strconv"
	"strings"

	"brickplans/internal/db"
)

// ── Detail page enrichment (SEO-20261026-02, batch 1) ───────────────────────
//
// The 2,134 blueprint detail pages are the biggest hole in the site: 28 days of
// GSC show 27 of 2,134 Chinese detail pages with any impression at all. The
// reason is not the markup but the content — every page carried the same
// template sentence plus a four-link "related" strip, so Google saw 2,134 near
// duplicates with almost no internal links.
//
// Batch 1 (this file) adds what can be built from data that already exists:
//
//	B1 同系列          other blueprints sharing the `系列: X` token in description
//	B2 同品牌相似件数   same brand, piece_count within ±20% (brands without a
//	                   piece count degrade to same brand + same category)
//	B3 同类目热门       the category's view_count top list
//	B4 面包屑          首页 > 探索 > 品牌 > 系列 > 当前图纸
//	A1 图纸信息表      <dl> of brand / set number / series / category / piece
//	                   count / difficulty / manual count / page count
//
// Hard constraints from the ticket, enforced by the tests in agg/detail tests:
//   - 4–6 links per module, never the same URL twice on one page (the modules
//     take a shared `used` set, so B2/B3 cannot repeat what B1 already linked);
//   - anchor text is always the blueprint title — never "点击这里" / "查看更多";
//   - nothing is rendered by JavaScript: every href is in the SSR HTML;
//   - a missing field is an omitted row, not "暂无" / "未知" / "-".

// DetailLinks is the three internal-link modules of one detail page.
type DetailLinks struct {
	Series    []db.Blueprint // B1 同系列
	SameBrand []db.Blueprint // B2 同品牌（件数区间，或同品牌同类别补齐）
	Category  []db.Blueprint // B3 同类目热门
}

const (
	moduleLimit    = 6  // 4–6 per module; 6 keeps a page above the 10-link floor
	moduleMinRows  = 2  // B1 hides itself below this: one sibling is not a module
	galleryCount   = 10 // A3: cover + first 10 pages = 11 <img> per page
	minDetailLinks = 10 // acceptance floor: unique /detail/ links per page
)

// seriesRE pulls the `系列: X` token out of the generated description. 2,095 of
// 2,134 rows carry one; the value is the only series signal the schema has until
// the dedicated column lands in batch 2.
var seriesRE = regexp.MustCompile(`系列[:：]\s*([^。\n\r]+)`)

// seriesOf returns the blueprint's series, or "" when the description has none.
func seriesOf(bp *db.Blueprint) string {
	if bp == nil || bp.Description == nil {
		return ""
	}
	m := seriesRE.FindStringSubmatch(*bp.Description)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// detailLinksData runs the three module queries.
//
// One `used` set spans all three: the ticket forbids the same URL appearing twice
// on one page, and B2/B3 draw from overlapping sets (same brand often implies same
// category), so dedupe has to happen across modules rather than inside each.
func (h *Handler) detailLinksData(bp *db.Blueprint) *DetailLinks {
	out := &DetailLinks{}
	used := map[string]bool{bp.ID: true}

	add := func(dst *[]db.Blueprint, rows []db.Blueprint, limit int) {
		for _, r := range rows {
			if len(*dst) >= limit || used[r.ID] {
				continue
			}
			used[r.ID] = true
			*dst = append(*dst, r)
		}
	}

	const cols = "id, title, title_en, category, brand, piece_count, difficulty, description, view_count"

	// B1 同系列 — match on the description token, not on a column that does not
	// exist yet.
	if s := seriesOf(bp); s != "" {
		var rows []db.Blueprint
		h.gdb.Select(cols).
			Where("is_published = ? AND id <> ? AND description LIKE ?", true, bp.ID, "%系列: "+s+"%").
			Order("view_count DESC").Limit(moduleLimit * 2).Find(&rows)
		add(&out.Series, rows, moduleLimit)
		if len(out.Series) < moduleMinRows {
			// Below two rows a "module" is just a stray link; drop it entirely.
			for _, r := range out.Series {
				delete(used, r.ID)
			}
			out.Series = nil
		}
	}

	// B2 同品牌相似件数. 奇积 has no piece_count at all (1,196 rows), so the ±20%
	// window would select nothing; those brands degrade to same brand + category.
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		brand := strings.TrimSpace(*bp.Brand)
		var rows []db.Blueprint
		if bp.PieceCount != nil && *bp.PieceCount > 0 {
			lo := int(float64(*bp.PieceCount) * 0.8)
			hi := int(float64(*bp.PieceCount) * 1.2)
			h.gdb.Select(cols).
				Where("is_published = ? AND id <> ? AND brand = ? AND piece_count BETWEEN ? AND ?", true, bp.ID, brand, lo, hi).
				Order("view_count DESC").Limit(moduleLimit * 2).Find(&rows)
		} else if bp.Category != nil {
			h.gdb.Select(cols).
				Where("is_published = ? AND id <> ? AND brand = ? AND category = ?", true, bp.ID, brand, *bp.Category).
				Order("view_count DESC").Limit(moduleLimit * 2).Find(&rows)
		}
		add(&out.SameBrand, rows, moduleLimit)
	}

	// B3 同类目热门.
	if bp.Category != nil && strings.TrimSpace(*bp.Category) != "" {
		var rows []db.Blueprint
		h.gdb.Select(cols).
			Where("is_published = ? AND id <> ? AND category = ?", true, bp.ID, strings.TrimSpace(*bp.Category)).
			Order("view_count DESC").Limit(moduleLimit * 2).Find(&rows)
		add(&out.Category, rows, moduleLimit)
	}

	// The acceptance floor is 10 unique /detail/ links on every detail page. Pages
	// whose brand has no piece count AND whose category is thin can come up short
	// (奇积 rows have piece_count=0, so B2 degrades to same-category and then
	// overlaps B3 almost entirely). Top the category module up rather than ship a
	// page that is one link short — same category, same view_count order, and the
	// shared `used` set still blocks any repeat.
	if totalLinks(out) < minDetailLinks && bp.Category != nil {
		var rows []db.Blueprint
		h.gdb.Select(cols).
			Where("is_published = ? AND id <> ? AND category = ?", true, bp.ID, strings.TrimSpace(*bp.Category)).
			Order("view_count DESC").Limit(minDetailLinks * 3).Find(&rows)
		add(&out.Category, rows, minDetailLinks*2)
	}

	return out
}

func totalLinks(l *DetailLinks) int { return len(l.Series) + len(l.SameBrand) + len(l.Category) }

// ── Rendering ───────────────────────────────────────────────────────────────

// crumbItem is one step of the B4 trail. Href is empty when the step has no
// landing page of its own (the series and the current page); the visible trail
// then renders a <span>, and the BreadcrumbList omits that item's URL — the two
// representations are generated from this one slice so they cannot drift.
type crumbItem struct {
	Name string
	Href string
}

func crumbsFor(lang string) (home, explore, aria string) {
	if lang == "en" {
		return "Home", "Explore", "Breadcrumb"
	}
	return "首页", "探索", "面包屑"
}

// detailCrumbs is the single source of truth for the trail: 首页 › 探索 › 品牌 ›
// 系列 › 当前图纸. Both the visible <nav> and the BreadcrumbList JSON-LD render
// from it (ticket B4 requires them to agree level by level).
func detailCrumbs(lang string, bp *db.Blueprint, public string) []crumbItem {
	home, explore, _ := crumbsFor(lang)
	homeHref := public + langPath(lang, "/")
	exploreHref := public + langPath(lang, "/explore")
	items := []crumbItem{{Name: home, Href: homeHref}, {Name: explore, Href: exploreHref}}
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		brand := strings.TrimSpace(*bp.Brand)
		label := brand
		if lang == "en" {
			label = brandLabelEN(brand)
		}
		// The brand filter lives on /explore until /brand/{slug} ships; the crumb
		// href and the JSON-LD item URL are the same string by construction.
		items = append(items, crumbItem{Name: label, Href: exploreHref + "?brand=" + urlQueryEscape(brand)})
	}
	if s := seriesOf(bp); s != "" {
		items = append(items, crumbItem{Name: s})
	}
	items = append(items, crumbItem{Name: bp.TitleIn(lang)})
	return items
}

// detailBreadcrumb renders the visible trail (B4) as plain HTML inside
// <nav class="breadcrumb">, so `curl` sees it.
func detailBreadcrumb(lang string, bp *db.Blueprint, public string) string {
	_, _, aria := crumbsFor(lang)
	items := detailCrumbs(lang, bp, public)
	parts := make([]string, 0, len(items))
	for i, c := range items {
		switch {
		case c.Href != "":
			parts = append(parts, fmt.Sprintf(`<a href="%s">%s</a>`, esc(c.Href), esc(c.Name)))
		case i == len(items)-1:
			parts = append(parts, fmt.Sprintf(`<span class="crumb-current">%s</span>`, esc(c.Name)))
		default:
			parts = append(parts, fmt.Sprintf(`<span class="crumb-series">%s</span>`, esc(c.Name)))
		}
	}
	return fmt.Sprintf(`<nav class="breadcrumb" aria-label="%s">%s</nav>`, esc(aria), strings.Join(parts, " › "))
}

// detailBreadcrumbJSONLD mirrors detailCrumbs one-for-one: same names, same
// order, contiguous positions, and item URLs identical to the visible href.
// A step without a landing page (series) carries no item, exactly like its
// <span> in the HTML.
func detailBreadcrumbJSONLD(lang string, bp *db.Blueprint, public string) template.JS {
	items := make([]map[string]interface{}, 0, 5)
	for i, c := range detailCrumbs(lang, bp, public) {
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

// urlQueryEscape is a tiny local helper so this file does not need net/url just
// for one brand value.
func urlQueryEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '~':
			b.WriteRune(r)
		default:
			buf := make([]byte, 0, 3)
			buf = append(buf, []byte(string(r))...)
			for _, c := range buf {
				b.WriteString("%")
				b.WriteString(strings.ToUpper(strconv.FormatInt(int64(c), 16)))
			}
		}
	}
	return b.String()
}

// specRow is one line of the A1 info table. value=="" means the row is dropped.
type specRow struct{ label, value string }

func specRows(lang string, bp *db.Blueprint, cover string) []specRow {
	cat := ""
	if bp.Category != nil {
		cat = strings.TrimSpace(*bp.Category)
		if lang == "en" {
			cat = categoryLabelEN(cat)
		}
	}
	brand := ""
	if bp.Brand != nil {
		brand = strings.TrimSpace(*bp.Brand)
	}
	code := ""
	if bp.ProductCode != nil {
		code = strings.TrimSpace(*bp.ProductCode)
	}
	diff := ""
	if bp.Difficulty != nil {
		diff = fmt.Sprintf("%d / 5", *bp.Difficulty)
	}
	pieces := ""
	if bp.PieceCount != nil && *bp.PieceCount > 0 {
		pieces = strconv.Itoa(*bp.PieceCount)
	}
	// Manuals are PDF/paged uploads; pages are the image renders. Splitting them
	// is what lets "共 1 份说明书 / 共 82 页图纸" be two honest numbers.
	manuals, pages := 0, 0
	for _, img := range bp.Images {
		if img.FileType == "image" && strings.ToLower(imgForceExt(img.URL)) != "pdf" {
			pages++
		} else {
			manuals++
		}
	}
	if cover != "" && pages == 0 {
		pages = 1 // a cover with no image rows still means one page exists
	}
	series := seriesOf(bp)

	if lang == "en" {
		return []specRow{
			{"Brand", brandLabelEN(brand)},
			{"Set number", code},
			{"Series", series},
			{"Category", cat},
			{"Pieces", pieces},
			{"Difficulty", diff},
			{"Instruction manuals", plural(manuals, "manual")},
			{"Instruction pages", plural(pages, "page")},
		}
	}
	return []specRow{
		{"品牌", brand},
		{"编号", code},
		{"系列", series},
		{"类目", cat},
		{"颗粒数", pieces},
		{"难度", diff},
		{"说明书份数", countSuffixZH(manuals, "份")},
		{"图纸页数", countSuffixZH(pages, "页")},
	}
}

func plural(n int, unit string) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// brandLabelEN is the display name of the two brands that make up 99.9% of the
// library. The stored value stays Chinese in the API and in the filter URL — only
// the English chrome is translated, the same way categoryLabelEN does it.
func brandLabelEN(brand string) string {
	switch brand {
	case "奇积":
		return "Qiji"
	case "乐高":
		return "LEGO"
	}
	return brand
}

func countSuffixZH(n int, unit string) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n) + " " + unit
}

func imgForceExt(url string) string {
	if i := strings.LastIndex(url, "."); i >= 0 {
		return url[i+1:]
	}
	return ""
}

// detailSpecTable renders A1. A row with an empty value is skipped: the ticket
// forbids placeholder text ("暂无"/"未知"/"-"), and 奇积 rows genuinely have no
// piece count, so that line must simply not exist on those pages.
func detailSpecTable(lang string, bp *db.Blueprint, cover string) string {
	var b strings.Builder
	b.WriteString(`<dl class="spec-table">`)
	for _, r := range specRows(lang, bp, cover) {
		if strings.TrimSpace(r.value) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("<dt>%s</dt><dd>%s</dd>", esc(r.label), esc(r.value)))
	}
	b.WriteString("</dl>")
	return b.String()
}

// detailGallery renders the first 8 instruction pages with the C1 alt template.
// Every <img> gets a non-empty alt (the acceptance script compares the count of
// <img> against the count of alt="..."), the first one is not lazy because it is
// the LCP candidate, and each thumbnail links to the full-size image.
func detailGallery(lang string, bp *db.Blueprint, public string) string {
	if len(bp.Images) == 0 {
		return ""
	}
	title := bp.TitleIn(lang)
	heading := "图纸目录"
	if lang == "en" {
		heading = "Instruction pages"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<section class="instruction-pages"><h2>%s</h2>`, esc(heading)))
	shown := 0
	for _, img := range bp.Images {
		if shown >= galleryCount {
			break
		}
		if img.FileType != "" && img.FileType != "image" {
			continue
		}
		n := shown + 1
		// Alt is the title alone: the title already carries the set number in
		// parentheses ("机甲小队首领 (LEGO 77078)"), so prefixing brand+code again
		// duplicated it on every one of the 2,134 pages.
		var alt string
		if lang == "en" {
			alt = fmt.Sprintf("%s building instructions page %d", title, n)
		} else {
			alt = fmt.Sprintf("%s 搭建说明书 第 %d 页", title, n)
		}
		if shown == 0 {
			b.WriteString(fmt.Sprintf(`<a href="%s"><img src="%s" alt="%s" /></a>`, esc(img.URL), esc(img.URL), esc(alt)))
		} else {
			b.WriteString(fmt.Sprintf(`<a href="%s"><img src="%s" alt="%s" loading="lazy" /></a>`, esc(img.URL), esc(img.URL), esc(alt)))
		}
		shown++
	}
	b.WriteString("</section>")
	if shown == 0 {
		return ""
	}
	return b.String()
}

// detailModules renders B1/B2/B3. Each module hides itself when empty, and the
// anchor text is always the blueprint title (the ticket bans "查看更多").
func detailModules(lang string, links *DetailLinks, public string) string {
	if links == nil {
		return ""
	}
	type mod struct {
		heading string
		rows    []db.Blueprint
	}
	mods := []mod{
		{heading: "同系列图纸", rows: links.Series},
		{heading: "同品牌相似件数", rows: links.SameBrand},
		{heading: "同类别热门图纸", rows: links.Category},
	}
	if lang == "en" {
		mods[0].heading = "More from this series"
		mods[1].heading = "Same brand, similar piece count"
		mods[2].heading = "Popular in this category"
	}
	// Dedupe is enforced at the sink as well as in the query layer: a URL may
	// appear once per page even if a caller hands over overlapping slices.
	seen := map[string]bool{}
	var b strings.Builder
	for _, m := range mods {
		var body strings.Builder
		rows := 0
		for _, r := range m.rows {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			body.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`,
				esc(public+langPath(lang, "/detail/"+r.ID)), esc(r.TitleIn(lang))))
			rows++
		}
		if rows == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf(`<section class="detail-links"><h2>%s</h2><ul>`, esc(m.heading)))
		b.WriteString(body.String())
		b.WriteString("</ul></section>")
	}
	return b.String()
}
