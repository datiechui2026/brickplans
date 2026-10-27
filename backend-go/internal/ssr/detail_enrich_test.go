package ssr

import (
	"regexp"
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
