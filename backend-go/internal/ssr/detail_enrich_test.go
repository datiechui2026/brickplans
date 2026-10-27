package ssr

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"brickplans/internal/db"
)

// Ticket SEO-20261026-02 batch 1. These pin the four things the live acceptance
// script measures, because a regression here is invisible in the API tests: the
// spec table, the 「颗粒数」 omission rule, the alt non-empty rule, and link
// dedupe across the three modules.

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func bpFixture() *db.Blueprint {
	bp := &db.Blueprint{
		ID:          "aaaaaaaa-0000-0000-0000-000000000001",
		Title:       "钢铁侠 Mark 3 收藏版",
		TitleEN:     strPtr("Iron Man Mark 3 Collectors' Edition"),
		Brand:       strPtr("奇积"),
		ProductCode: strPtr("S0101"),
		Category:    strPtr("场景"),
		Difficulty:  intPtr(2),
		ViewCount:   13,
		Description: strPtr("奇积 官方套装 S0101 钢铁侠 搭建说明书。系列: 宝可梦系列。共 1 份说明书。"),
		Images: []db.BlueprintImage{
			{URL: "https://cdn.test/1.jpg", FileType: "image", SortOrder: 1},
			{URL: "https://cdn.test/2.jpg", FileType: "image", SortOrder: 2},
		},
	}
	return bp
}

func TestSeriesOfReadsDescriptionToken(t *testing.T) {
	bp := bpFixture()
	if got := seriesOf(bp); got != "宝可梦系列" {
		t.Fatalf("seriesOf = %q, want 宝可梦系列", got)
	}
	bp.Description = strPtr("没有系列标记的描述。")
	if got := seriesOf(bp); got != "" {
		t.Fatalf("seriesOf without token = %q, want empty", got)
	}
}

// A1: a <dl> must exist and carry 编号.
func TestDetailBodyCarriesSpecTable(t *testing.T) {
	bp := bpFixture()
	html := string(detailNoscriptFor("zh", bp, "https://cdn.test/1.jpg", "https://brickplan.test", nil))
	if !strings.Contains(html, "<dl") {
		t.Fatal("no <dl> spec table in detail body")
	}
	if !strings.Contains(html, "编号") {
		t.Fatal("spec table has no 编号 row")
	}
	if !strings.Contains(html, "宝可梦系列") {
		t.Fatal("spec table has no 系列 row")
	}
}

// A2: 奇积 rows have piece_count = 0, and the word 颗粒数 must not appear at all.
func TestDetailBodyOmitsPieceRowWithoutPieceCount(t *testing.T) {
	bp := bpFixture()
	bp.PieceCount = intPtr(0)
	html := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if strings.Contains(html, "颗粒数") {
		t.Fatalf("detail body renders 颗粒数 for a row without piece_count")
	}
	// The same row with a real count must render it.
	bp.PieceCount = intPtr(1234)
	html = string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(html, "颗粒数") || !strings.Contains(html, "1234") {
		t.Fatal("detail body drops 颗粒数 for a row that has one")
	}
}

// C1/C1b: every <img> carries a non-empty alt.
func TestDetailImagesAllHaveAlt(t *testing.T) {
	bp := bpFixture()
	html := string(detailNoscriptFor("zh", bp, "https://cdn.test/1.jpg", "https://brickplan.test", nil))
	imgs := strings.Count(html, "<img")
	alts := len(regexp.MustCompile(`alt="[^"]{1,}"`).FindAllString(html, -1))
	if imgs < 2 {
		t.Fatalf("<img> count = %d, want >= 2", imgs)
	}
	if alts != imgs {
		t.Fatalf("alt count %d != <img> count %d", alts, imgs)
	}
}

// A7/A7b: the three modules must never repeat a URL, must not link back to the
// page itself, and must clear the 10-link floor when the data allows.
func TestDetailModulesDedupeAndReachFloor(t *testing.T) {
	self := bpFixture()
	var series, brand, category []db.Blueprint
	for i := 0; i < 6; i++ {
		series = append(series, db.Blueprint{ID: "series-" + string(rune('a'+i)), Title: "S"})
	}
	for i := 0; i < 4; i++ {
		brand = append(brand, db.Blueprint{ID: "brand-" + string(rune('a'+i)), Title: "B"})
	}
	// B3 intentionally repeats one B2 row: dedupe is per page, not per module.
	category = append(category, brand[0])
	for i := 0; i < 6; i++ {
		category = append(category, db.Blueprint{ID: "cat-" + string(rune('a'+i)), Title: "C"})
	}
	links := &DetailLinks{Series: series, SameBrand: brand, Category: category}
	html := string(detailNoscriptFor("zh", self, "", "https://brickplan.test", links))

	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`href="[^"]*/detail/([^"]+)"`).FindAllStringSubmatch(html, -1) {
		id := m[1]
		if id == self.ID {
			t.Fatalf("module links back to the current page: %s", id)
		}
		if seen[id] {
			t.Fatalf("duplicate /detail/ link on one page: %s", id)
		}
		seen[id] = true
	}
	if len(seen) < 10 {
		t.Fatalf("unique /detail/ links = %d, want >= 10", len(seen))
	}
}

// B4 (defect 1, 2026-10-27): the BreadcrumbList must mirror the visible trail
// level by level — 5 items, contiguous positions, and every item URL identical
// to a visible href.
func TestDetailBreadcrumbJSONLDMatchesVisibleTrail(t *testing.T) {
	bp := bpFixture()
	public := "https://brickplan.test"
	visible := string(detailBreadcrumb("zh", bp, public))
	raw := string(detailBreadcrumbJSONLD("zh", bp, public))

	var doc struct {
		Type  string `json:"@type"`
		Items []struct {
			Position int    `json:"position"`
			Name     string `json:"name"`
			Item     string `json:"item"`
		} `json:"itemListElement"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("breadcrumb JSON-LD is not valid JSON: %v\n%s", err, raw)
	}
	if doc.Type != "BreadcrumbList" {
		t.Fatalf("@type = %q", doc.Type)
	}
	want := []string{"首页", "探索", "奇积", "宝可梦系列", bp.Title}
	if len(doc.Items) != len(want) {
		t.Fatalf("itemListElement = %d items, visible trail has %d (%s)", len(doc.Items), len(want), raw)
	}
	for i, it := range doc.Items {
		if it.Position != i+1 {
			t.Fatalf("position %d at index %d, positions must be contiguous", it.Position, i)
		}
		if it.Name != want[i] {
			t.Fatalf("item[%d].name = %q, want %q", i, it.Name, want[i])
		}
		if it.Item != "" && !strings.Contains(visible, `href="`+it.Item+`"`) {
			t.Fatalf("item[%d] URL %q has no matching visible href\n%s", i, it.Item, visible)
		}
	}
	// Every visible href must be in the JSON-LD too (no extra steps).
	for _, m := range regexp.MustCompile(`href="([^"]+)"`).FindAllStringSubmatch(visible, -1) {
		if !strings.Contains(raw, `"item":"`+m[1]+`"`) {
			t.Fatalf("visible href %q missing from BreadcrumbList\n%s", m[1], raw)
		}
	}
	// The old explore?category= step must not come back.
	if strings.Contains(raw, "?category=") {
		t.Fatalf("breadcrumb JSON-LD regressed to the parameter URL: %s", raw)
	}
}

// B4 in English: same trail, English labels, /en tree.
func TestDetailBreadcrumbJSONLDENMatchesVisibleTrail(t *testing.T) {
	bp := bpFixture()
	public := "https://brickplan.test"
	raw := string(detailBreadcrumbJSONLD("en", bp, public))
	for _, want := range []string{`"name":"Home"`, `"name":"Explore"`, `"name":"Qiji"`, `"position":5`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("EN breadcrumb JSON-LD missing %s: %s", want, raw)
		}
	}
	if strings.Contains(raw, "?category=") {
		t.Fatalf("EN breadcrumb regressed to the parameter URL: %s", raw)
	}
}

// Defect 2 (2026-10-27): alt text was 品牌+编号+标题, and the title already ends
// with "(LEGO 77078)" — so the set number printed twice on all 2,134 pages.
func TestDetailImageAltsAreTitleOnly(t *testing.T) {
	bp := bpFixture()
	bp.Title = "机甲小队首领 (LEGO 77078)"
	html := string(detailNoscriptFor("zh", bp, "https://cdn.test/1.jpg", "https://brickplan.test", nil))
	if !strings.Contains(html, `alt="`+bp.Title+` 封面"`) {
		t.Fatalf("cover alt is not \"{title} 封面\"")
	}
	if !strings.Contains(html, `alt="`+bp.Title+` 搭建说明书 第 1 页"`) {
		t.Fatalf("page alt is not \"{title} 搭建说明书 第 N 页\"")
	}
	if strings.Contains(html, "奇积S0101") {
		t.Fatalf("alt still prefixes brand+code")
	}

	en := string(detailNoscriptFor("en", bp, "https://cdn.test/1.jpg", "https://brickplan.test", nil))
	if !strings.Contains(en, `alt="`+bp.Title[:len(bp.Title)]+`"`) && !strings.Contains(en, "cover") {
		t.Fatalf("EN cover alt missing")
	}
	if !strings.Contains(en, "building instructions page 1") {
		t.Fatalf("EN page alt missing")
	}
}

// Defect 3 (2026-10-27): the gallery must render cover + first 10 pages = 11
// <img>, first one eager, the rest lazy.
func TestDetailGalleryRendersElevenImages(t *testing.T) {
	bp := bpFixture()
	bp.Images = nil
	for i := 0; i < 25; i++ {
		bp.Images = append(bp.Images, db.BlueprintImage{
			URL:       "https://cdn.test/p" + strconv.Itoa(i+1) + ".jpg",
			FileType:  "image",
			SortOrder: i,
		})
	}
	html := string(detailNoscriptFor("zh", bp, "https://cdn.test/cover.jpg", "https://brickplan.test", nil))
	if got := strings.Count(html, "<img"); got != 11 {
		t.Fatalf("<img> count = %d, want 11 (cover + first 10 pages)", got)
	}
	if got := strings.Count(html, `loading="lazy"`); got != 9 {
		t.Fatalf("lazy images = %d, want 9 (all but the first page image; the cover is a separate <img> without loading)", got)
	}
	if got := strings.Count(html, `alt="`+bp.Title); got != 11 {
		t.Fatalf("alt count = %d, want 11", got)
	}
}

// B4: the visible crumb lives in HTML (not only in JSON-LD).
func TestDetailBreadcrumbVisible(t *testing.T) {
	bp := bpFixture()
	html := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(html, `<nav class="breadcrumb"`) {
		t.Fatal("no visible breadcrumb nav")
	}
	for _, want := range []string{"首页", "探索", "奇积", "宝可梦系列"} {
		if !strings.Contains(html, want) {
			t.Fatalf("breadcrumb missing %q", want)
		}
	}
}
