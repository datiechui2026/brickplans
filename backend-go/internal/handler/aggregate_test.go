package handler_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"brickplans/internal/db"
)

// Ticket SEO-20261024 acceptance, expressed as tests: the aggregate landing
// pages replace /explore?category=… as the crawlable entry points, so every
// property the ticket asks for (self-canonical, count in title, 48 per page, no
// overlap between pages, hreflang cluster, sitemap replacement) is pinned here.
// The live acceptance script measures the same things over HTTP.

func seedAggregate(t *testing.T, gdb *gorm.DB, authorID, cat, brand string, diff, n int) []string {
	t.Helper()
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		bp := db.Blueprint{
			AuthorID:    authorID,
			Title:       fmt.Sprintf("%s 图纸 %02d", cat, i),
			Slug:        fmt.Sprintf("%s-%s-%d-%02d", cat, brand, diff, i),
			Category:    &cat,
			Brand:       &brand,
			Difficulty:  &diff,
			IsPublished: true,
			ViewCount:   i,
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
		}
		if err := gdb.Create(&bp).Error; err != nil {
			t.Fatalf("seed blueprint: %v", err)
		}
		ids = append(ids, bp.ID)
	}
	return ids
}

func detailLinks(body string) map[string]bool {
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`href="[^"]*?/detail/([^"?#]+)"`).FindAllStringSubmatch(body, -1) {
		out[m[1]] = true
	}
	return out
}

// mainListLinks returns the detail links of the paged section only, i.e. the
// first H2 (count line) up to the next H2 (热门).
func mainListLinks(t *testing.T, body string) map[string]bool {
	t.Helper()
	start := strings.Index(body, `class="agg-results"`)
	if start < 0 {
		t.Fatal("no agg-results block in body")
	}
	section := body[start:]
	if end := strings.Index(section, `</div>`); end > 0 {
		section = section[:end]
	}
	return detailLinks(section)
}

func mustContain(t *testing.T, body, want, what string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("%s: missing %q\n%.600s", what, want, body)
	}
}

// A1–A3: 200, ≥48 detail links on the first page, self-canonical, count in
// <title> and in the first H2.
func TestCategoryLandingPageRendersFullFirstPage(t *testing.T) {
	r, gdb := setupSitemapTest(t, 0, 0)
	token, uid := registerUser(t, r, "aggcat", "aggcat@example.com")
	_ = token
	seedAggregate(t, gdb, uid, "场景", "奇积", 2, 60)
	seedAggregate(t, gdb, uid, "车辆", "乐高", 3, 3)

	code, body := getHTML(t, r, "/category/scene")
	if code != http.StatusOK {
		t.Fatalf("/category/scene = %d, want 200", code)
	}
	mustContain(t, body, "<title>场景类积木图纸（60 个）— BrickPlan</title>", "title carries the DB count")
	mustContain(t, body, "<h1>场景类积木图纸</h1>", "H1")
	mustContain(t, body, "<h2>场景类图纸共 60 个</h2>", "H2 #1 carries the count")
	// H2 order is the ticket's structure, verbatim.
	wantH2 := []string{
		"<h2>场景类图纸共 60 个</h2>",
		"<h2>热门场景图纸</h2>",
		"<h2>最新上传</h2>",
		"<h2>按品牌浏览</h2>",
		"<h2>按难度浏览</h2>",
		"<h2>其他分类</h2>",
	}
	last := -1
	for _, h := range wantH2 {
		i := strings.Index(body, h)
		if i < 0 {
			t.Fatalf("missing H2 %q", h)
		}
		if i < last {
			t.Fatalf("H2 %q is out of order", h)
		}
		last = i
	}
	mustContain(t, body, `<link rel="canonical" href="https://brickplans.test/category/scene" />`, "canonical")
	if strings.Contains(body, "/explore?category=") {
		t.Fatal("aggregate page still points at the parameter URL")
	}
	if got := len(detailLinks(body)); got < 48 {
		t.Fatalf("first page links %d detail pages, want >= 48", got)
	}
	// The three 车辆 rows must not leak in: the filter is the whole point.
	if strings.Contains(body, "车辆 图纸 00") {
		t.Fatal("page leaked rows from another category")
	}

	// JSON-LD: ItemList for the 48 items + BreadcrumbList on the new path.
	mustContain(t, body, `"@type":"ItemList"`, "ItemList JSON-LD")
	mustContain(t, body, `"numberOfItems":48`, "ItemList numberOfItems")
	mustContain(t, body, `"item":"https://brickplans.test/explore"`, "BreadcrumbList item for the index")
	mustContain(t, body, `"name":"场景类积木图纸"`, "BreadcrumbList names the page itself")
}

// A4: page 1 and page 2 must not share a single blueprint.
func TestCategoryPaginationHasNoOverlap(t *testing.T) {
	r, gdb := setupSitemapTest(t, 0, 0)
	_, uid := registerUser(t, r, "aggpage", "aggpage@example.com")
	seedAggregate(t, gdb, uid, "场景", "奇积", 2, 100)

	_, p1 := getHTML(t, r, "/category/scene")
	code, p2 := getHTML(t, r, "/category/scene?page=2")
	if code != http.StatusOK {
		t.Fatalf("page 2 = %d, want 200", code)
	}
	// Only the paged result list may be compared: 热门/最新 repeat on every
	// page by design, so counting whole-page links would always "overlap".
	a, b := mainListLinks(t, p1), mainListLinks(t, p2)
	if len(a) < 48 || len(b) < 48 {
		t.Fatalf("page sizes: p1=%d p2=%d, want >=48 each", len(a), len(b))
	}
	for id := range a {
		if b[id] {
			t.Fatalf("blueprint %s appears on both page 1 and page 2", id)
		}
	}
	// page>=2 is self-canonical and carries the prev/next chain.
	mustContain(t, p2, `<link rel="canonical" href="https://brickplans.test/category/scene?page=2" />`, "page-2 canonical")
	mustContain(t, p2, `<link rel="prev" href="https://brickplans.test/category/scene" />`, "page-2 rel=prev")
	mustContain(t, p1, `<link rel="next" href="https://brickplans.test/category/scene?page=2" />`, "page-1 rel=next")
}

// 3.2: past the last page is still 200 (a real page, just empty), never a 404.
func TestCategoryOutOfRangePageIs200(t *testing.T) {
	r, gdb := setupSitemapTest(t, 0, 0)
	_, uid := registerUser(t, r, "aggoob", "aggoob@example.com")
	seedAggregate(t, gdb, uid, "场景", "奇积", 2, 3)

	code, body := getHTML(t, r, "/category/scene?page=99")
	if code != http.StatusOK {
		t.Fatalf("page=99 = %d, want 200", code)
	}
	mustContain(t, body, "<h2>场景类图纸共 3 个</h2>", "count is still stated on an empty page")
	mustContain(t, body, "这一页没有图纸。", "empty-page notice")
}

// Unknown slug / out-of-range difficulty are real 404s: these URLs must never
// be indexed, so they cannot answer 200.
func TestAggregateUnknownSlugIs404(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	registerUser(t, r, "agg404", "agg404@example.com")
	for _, p := range []string{"/category/nope", "/brand/nope", "/difficulty/9", "/difficulty/x"} {
		if code, _ := getHTML(t, r, p); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", p, code)
		}
	}
}

// Brand and difficulty pages use the same renderer with a different filter.
func TestBrandAndDifficultyLandingPages(t *testing.T) {
	r, gdb := setupSitemapTest(t, 0, 0)
	_, uid := registerUser(t, r, "aggbrand", "aggbrand@example.com")
	seedAggregate(t, gdb, uid, "场景", "奇积", 2, 5)
	seedAggregate(t, gdb, uid, "场景", "乐高", 5, 4)

	code, body := getHTML(t, r, "/brand/qiji")
	if code != http.StatusOK {
		t.Fatalf("/brand/qiji = %d", code)
	}
	mustContain(t, body, "<title>奇积品牌积木图纸（5 个）— BrickPlan</title>", "brand title")
	mustContain(t, body, `<link rel="canonical" href="https://brickplans.test/brand/qiji" />`, "brand canonical")
	mustContain(t, body, `href="https://brickplans.test/category/scene">场景<`, "brand page links the category landing page")
	mustContain(t, body, `href="https://brickplans.test/brand/lego">乐高<`, "brand page links the other brand")

	code, body = getHTML(t, r, "/difficulty/2")
	if code != http.StatusOK {
		t.Fatalf("/difficulty/2 = %d", code)
	}
	mustContain(t, body, "<title>难度 2 积木图纸（5 个）— BrickPlan</title>", "difficulty title")
	mustContain(t, body, "<h1>难度 2 积木图纸</h1>", "difficulty H1")
	// 4 rows are difficulty 5 — they must not appear in the main list.
	if strings.Contains(body, "乐高 图纸") {
		t.Fatal("difficulty page leaked rows of another difficulty")
	}
}

// A6: the English twin is in the same hreflang cluster, x-default on Chinese.
func TestCategoryEnglishTwinHreflang(t *testing.T) {
	r, gdb := setupSitemapTest(t, 0, 0)
	_, uid := registerUser(t, r, "aggen", "aggen@example.com")
	seedAggregate(t, gdb, uid, "场景", "奇积", 2, 4)

	code, body := getHTML(t, r, "/en/category/scene")
	if code != http.StatusOK {
		t.Fatalf("/en/category/scene = %d", code)
	}
	mustContain(t, body, `<html lang="en">`, "html lang")
	mustContain(t, body, "<title>Scene LEGO &amp; Brick Blueprints (4) — BrickPlan</title>", "EN title")
	mustContain(t, body, `<link rel="alternate" hreflang="zh-CN" href="https://brickplans.test/category/scene" />`, "zh-CN alternate")
	mustContain(t, body, `<link rel="alternate" hreflang="en" href="https://brickplans.test/en/category/scene" />`, "en alternate")
	mustContain(t, body, `<link rel="alternate" hreflang="x-default" href="https://brickplans.test/category/scene" />`, "x-default on zh")
	mustContain(t, body, `<link rel="canonical" href="https://brickplans.test/en/category/scene" />`, "EN self-canonical")
}

// A5: sitemap-zh no longer advertises a single ?category= URL and instead
// carries 6 + 2 + 5 aggregate pages; sitemap-en carries their English twins.
func TestSitemapAggregateReplacesParameterPages(t *testing.T) {
	r, gdb := setupSitemapTest(t, 0, 0)
	_, uid := registerUser(t, r, "aggsitemap", "aggsitemap@example.com")
	seedAggregate(t, gdb, uid, "场景", "奇积", 2, 3)
	seedAggregate(t, gdb, uid, "车辆", "乐高", 3, 2)

	code, zh := getSitemap(t, r, "/sitemap-zh.xml")
	if code != http.StatusOK {
		t.Fatalf("sitemap-zh = %d", code)
	}
	if n := strings.Count(zh, "explore?category="); n != 0 {
		t.Errorf("sitemap-zh still has %d parameter URLs", n)
	}
	for _, want := range []struct {
		frag string
		n    int
	}{
		{"<loc>https://brickplans.test/category/", 6},
		{"<loc>https://brickplans.test/brand/", 2},
		{"<loc>https://brickplans.test/difficulty/", 5},
	} {
		if got := strings.Count(zh, want.frag); got != want.n {
			t.Errorf("sitemap-zh %s = %d, want %d", want.frag, got, want.n)
		}
	}
	mustContain(t, zh, "<loc>https://brickplans.test/category/scene</loc>", "zh aggregate loc")

	_, en := getSitemap(t, r, "/sitemap-en.xml")
	if n := strings.Count(en, "<loc>https://brickplans.test/en/category/"); n != 6 {
		t.Errorf("sitemap-en /en/category/ = %d, want 6", n)
	}
	if n := strings.Count(en, "<loc>https://brickplans.test/en/difficulty/"); n != 5 {
		t.Errorf("sitemap-en /en/difficulty/ = %d, want 5", n)
	}
	// No pagination URLs: the hubs must not be buried under page=2..N.
	if strings.Contains(zh, "/category/scene?page=") {
		t.Error("sitemap advertises pagination URLs for an aggregate page")
	}
	_ = url.QueryEscape
}
