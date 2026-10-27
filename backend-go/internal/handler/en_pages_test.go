package handler_test

import (
	"net/url"
	"strings"
	"testing"
)

// TestEnglishUGCPagesConsolidateToChinese pins the indexing policy for the new
// English UI pages: detail/user/tags serve the same Chinese content as the
// Chinese originals, so they must canonicalise back and must NOT declare a
// language alternate. Getting this wrong creates two indexable copies of one
// document and can pull the canonical away from the Chinese page.
func TestEnglishUGCPagesConsolidateToChinese(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	token, userID := registerUser(t, r, "builder", "builder@example.com")
	bpID := createBlueprint(t, r, token, "城镇街角", true)

	t.Run("detail", func(t *testing.T) {
		code, body := getHTML(t, r, "/en/detail/"+bpID)
		if code != 200 {
			t.Fatalf("/en/detail: %d", code)
		}
		if !strings.Contains(body, `<html lang="en">`) {
			t.Error("missing <html lang=\"en\">")
		}
		if !strings.Contains(body, `<link rel="canonical" href="https://brickplans.test/detail/`+bpID+`"`) {
			t.Errorf("canonical must point at the Chinese original:\n%s", headLines(body, "canonical"))
		}
		if !strings.Contains(body, `<meta property="og:url" content="https://brickplans.test/en/detail/`+bpID+`"`) {
			t.Errorf("og:url must stay the English URL:\n%s", headLines(body, "og:url"))
		}
		if strings.Contains(body, `<link rel="alternate" hreflang=`) {
			t.Errorf("a UI variant must not claim hreflang alternates:\n%s", headLines(body, "hreflang"))
		}
		if !strings.Contains(body, "Author: ") {
			t.Error("detail body is not in English (missing Author: label)")
		}
	})

	t.Run("user", func(t *testing.T) {
		code, body := getHTML(t, r, "/en/user/"+userID)
		if code != 200 {
			t.Fatalf("/en/user: %d", code)
		}
		if !strings.Contains(body, `<link rel="canonical" href="https://brickplans.test/user/`+userID+`"`) {
			t.Errorf("canonical must point at the Chinese original:\n%s", headLines(body, "canonical"))
		}
		if strings.Contains(body, `<link rel="alternate" hreflang=`) {
			t.Errorf("a UI variant must not claim hreflang alternates:\n%s", headLines(body, "hreflang"))
		}
	})

	t.Run("utility pages are noindex", func(t *testing.T) {
		for _, p := range []string{"/en/upload", "/en/edit/" + bpID, "/en/notifications", "/en/admin"} {
			code, body := getHTML(t, r, p)
			if code != 200 {
				t.Errorf("%s: %d", p, code)
				continue
			}
			// Spacing between the directives is cosmetic; both spellings are
			// valid robots directives, so assert the directive, not the comma.
			if !strings.Contains(body, `<meta name="robots" content="noindex`) {
				t.Errorf("%s: missing noindex meta", p)
			}
			if strings.Contains(body, `<link rel="alternate" hreflang=`) {
				t.Errorf("%s: utility page must not emit hreflang", p)
			}
		}
	})
}

// TestEnglishHomeAndExploreListBuildings pins the new EN bodies: they must render
// the same crawlable blueprint list as CN (English chrome, original titles), and
// the home must keep linking every English article.
func TestEnglishHomeAndExploreListBuildings(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	token, _ := registerUser(t, r, "builder", "builder@example.com")
	bpID := createBlueprint(t, r, token, "城镇街角", true)

	for _, p := range []string{"/en/", "/en/explore"} {
		code, body := getHTML(t, r, p)
		if code != 200 {
			t.Fatalf("%s: %d", p, code)
		}
		if !strings.Contains(body, "/en/detail/"+bpID) {
			t.Errorf("%s does not link the published blueprint into the EN tree", p)
		}
	}

	// Every English article must stay reachable from /en/ — the homepage is the
	// strongest internal-link source and the English tree is small.
	_, home := getHTML(t, r, "/en/")
	for _, slug := range []string{"sm-parts", "sm-en-only"} {
		if !strings.Contains(home, "/en/blog/"+slug) {
			t.Errorf("/en/ no longer links /en/blog/%s", slug)
		}
	}
}

// TestEnglishCategoryLabelsAreTranslated pins that a stored Chinese category
// value never leaks into English copy. The VALUE must survive into the API/URL
// (filters match on it), the LABEL must be English in <title>, description and
// the breadcrumb trail — otherwise English pages read "建筑 blueprints".
func TestEnglishCategoryLabelsAreTranslated(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	token, _ := registerUser(t, r, "builder", "builder@example.com")
	w := doJSON(t, r, "POST", "/api/blueprints", map[string]any{"title": "城镇街角", "category": "建筑"}, token)
	if w.Code != 201 {
		t.Fatalf("create bp: %d %s", w.Code, w.Body.String())
	}
	bpID := parseJSON(t, w)["id"].(string)

	// The filter query carries the stored value, exactly what the SPA sends.
	code, body := getHTML(t, r, "/en/explore?category="+url.QueryEscape("建筑"))
	if code != 200 {
		t.Fatalf("/en/explore?category=建筑: %d", code)
	}
	if !strings.Contains(body, "<title>Architecture blueprints — BrickPlan</title>") {
		t.Errorf("EN category title is not translated:\n%s", headLines(body, "<title>"))
	}
	desc := headLines(body, `name="description"`)
	if !strings.Contains(desc, "Architecture") || strings.Contains(desc, "建筑") {
		t.Errorf("EN category description is not translated: %s", desc)
	}
	// The listing must still be filtered by the Chinese value.
	if !strings.Contains(body, "/en/detail/"+bpID) {
		t.Error("category filter no longer matches the stored Chinese value")
	}

	// The EN detail trail is Home › Explore › Architecture › title (no brand or
	// series on this fixture). SEO-20261024 made the category a real crumb level:
	// it must point at the /en/category/{slug} landing page inside the /en tree,
	// carry the English label, and never fall back to the old ?category= form or
	// to the stored Chinese value.
	_, detail := getHTML(t, r, "/en/detail/"+bpID)
	crumbs := headLines(detail, "BreadcrumbList")
	if !strings.Contains(crumbs, `"name":"Home"`) || !strings.Contains(crumbs, `"name":"Explore"`) {
		t.Errorf("EN breadcrumb is not translated:\n%s", crumbs)
	}
	if !strings.Contains(crumbs, `"item":"https://brickplans.test/en/explore"`) {
		t.Errorf("explore crumb left the /en tree (broken trail):\n%s", crumbs)
	}
	// The category level now exists and must be the English label pointing at the
	// English landing page — not the stored Chinese value, not the parameter URL.
	if !strings.Contains(crumbs, `"name":"Architecture"`) {
		t.Errorf("category crumb label is not translated:\n%s", crumbs)
	}
	if !strings.Contains(crumbs, `"item":"https://brickplans.test/en/category/architecture"`) {
		t.Errorf("category crumb does not point at the /en/category landing page:\n%s", crumbs)
	}
	if strings.Contains(crumbs, `"name":"建筑"`) {
		t.Errorf("category crumb printed the stored Chinese value:\n%s", crumbs)
	}
	if strings.Contains(crumbs, "?category=") {
		t.Errorf("breadcrumb regressed to the parameter URL:\n%s", crumbs)
	}
}

// TestChinesePagesKeepTheirMarkup guards the "only optimize, never regress" rule:
// the bilingual work must not add the EN-only shell or change the Chinese
// canonical/hreflang contract.
func TestChinesePagesKeepTheirMarkup(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	token, _ := registerUser(t, r, "builder", "builder@example.com")
	bpID := createBlueprint(t, r, token, "城镇街角", true)

	for _, p := range []string{"/", "/explore", "/blog"} {
		_, body := getHTML(t, r, p)
		if strings.Contains(body, `class="page active"`) {
			t.Errorf("%s must not carry the SSR shell (the SPA hydrates it)", p)
		}
	}

	_, detail := getHTML(t, r, "/detail/"+bpID)
	if !strings.Contains(detail, `<link rel="canonical" href="https://brickplans.test/detail/`+bpID+`"`) {
		t.Errorf("Chinese detail lost its self canonical:\n%s", headLines(detail, "canonical"))
	}
	if !strings.Contains(detail, `<link rel="alternate" hreflang="zh-CN"`) || !strings.Contains(detail, `<link rel="alternate" hreflang="x-default"`) {
		t.Errorf("Chinese detail lost its hreflang contract:\n%s", headLines(detail, "hreflang"))
	}
}

// TestEnglishBlueprintTextUsesEnglishFields pins the language split for
// user-generated blueprint text: title/description stay the Chinese document
// (and the Chinese page keeps showing them), while /en/detail uses
// title_en/description_en. Empty English fields fall back to the Chinese values
// rather than rendering a blank heading.
func TestEnglishBlueprintTextUsesEnglishFields(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	token, _ := registerUser(t, r, "builder", "builder@example.com")

	w := doJSON(t, r, "POST", "/api/blueprints", map[string]any{
		"title":          "城镇街角",
		"title_en":       "Town Corner",
		"description":    "中文描述",
		"description_en": "English description",
		"category":       "建筑",
		"brand":          "乐高",
		"is_published":   true,
	}, token)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	created := parseJSON(t, w)
	bpID := created["id"].(string)
	if created["title_en"] != "Town Corner" || created["description_en"] != "English description" {
		t.Fatalf("English fields not echoed: %v", created)
	}

	_, en := getHTML(t, r, "/en/detail/"+bpID)
	if !strings.Contains(en, "<title>Town Corner — BrickPlan MOC blueprint</title>") {
		t.Errorf("EN detail title did not use title_en:\n%s", headLines(en, "<title>"))
	}
	if !strings.Contains(en, "<h1>Town Corner</h1>") || !strings.Contains(en, "English description") {
		t.Error("EN detail body did not use the English title/description")
	}
	if !strings.Contains(en, `"name":"Town Corner"`) {
		t.Errorf("EN JSON-LD did not use title_en:\n%s", headLines(en, "CreativeWork"))
	}

	_, zh := getHTML(t, r, "/detail/"+bpID)
	if !strings.Contains(zh, "<h1>城镇街角</h1>") || !strings.Contains(zh, "中文描述") {
		t.Error("CN detail no longer renders the Chinese title/description")
	}
	if strings.Contains(zh, "Town Corner") {
		t.Error("Chinese detail leaked the English title")
	}

	// Fallback: a blueprint without translations still renders its Chinese
	// title on the English page.
	w = doJSON(t, r, "POST", "/api/blueprints", map[string]any{
		"title": "无译文作品", "is_published": true,
	}, token)
	plainID := parseJSON(t, w)["id"].(string)
	_, fallback := getHTML(t, r, "/en/detail/"+plainID)
	if !strings.Contains(fallback, "<h1>无译文作品</h1>") {
		t.Error("EN detail lost the Chinese fallback title")
	}
}

// TestExploreBrandFilter pins the brand filter on both trees: the API, the
// server-rendered listing all match the stored Chinese value (what the dropdown
// sends), not the English display label.
func TestExploreBrandFilter(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	token, _ := registerUser(t, r, "builder", "builder@example.com")

	create := func(fields map[string]any) string {
		t.Helper()
		fields["is_published"] = true
		w := doJSON(t, r, "POST", "/api/blueprints", fields, token)
		if w.Code != 201 {
			t.Fatalf("create blueprint: %d %s", w.Code, w.Body.String())
		}
		return parseJSON(t, w)["id"].(string)
	}
	legoID := create(map[string]any{"title": "乐高城堡", "brand": "乐高"})
	semboID := create(map[string]any{"title": "森宝街景", "brand": "森宝"})

	code, en := getHTML(t, r, "/en/explore?brand="+url.QueryEscape("乐高"))
	if code != 200 {
		t.Fatalf("/en/explore: %d", code)
	}
	if !strings.Contains(en, "/en/detail/"+legoID) {
		t.Error("EN explore?brand=乐高 does not list the LEGO build")
	}
	if strings.Contains(en, "/en/detail/"+semboID) {
		t.Error("EN explore?brand=乐高 leaked another brand")
	}

	_, zh := getHTML(t, r, "/explore?brand="+url.QueryEscape("森宝"))
	if !strings.Contains(zh, "/detail/"+semboID) || strings.Contains(zh, "/detail/"+legoID) {
		t.Error("CN explore?brand=森宝 is not filtering by the stored brand")
	}

	w := doJSON(t, r, "GET", "/api/blueprints?brand="+url.QueryEscape("乐高"), nil, token)
	if w.Code != 200 {
		t.Fatalf("API list: %d", w.Code)
	}
	if total := parseJSON(t, w)["total"].(float64); total != 1 {
		t.Errorf("API brand filter total = %v, want 1", total)
	}
}

// TestNavShowsThreeTabs pins requirement 1: the server-rendered header shows
// only Explore / Blog / About (the same three links the SPA renders), while the
// footer keeps Home / FAQ / Privacy so those pages do not lose their internal
// links.
func TestNavShowsThreeTabs(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	_, body := getHTML(t, r, "/en/about")
	start := strings.Index(body, `<nav class="site-nav navbar"`)
	if start < 0 {
		t.Fatal("no server-rendered <nav>")
	}
	end := strings.Index(body[start:], "</nav>")
	if end < 0 {
		t.Fatal("unterminated <nav>")
	}
	nav := body[start : start+end]
	for _, want := range []string{`href="/en/explore"`, `href="/en/blog"`, `href="/en/about"`} {
		if !strings.Contains(nav, want) {
			t.Errorf("nav missing %s", want)
		}
	}
	for _, unwanted := range []string{`href="/en/faq"`, `href="/en/privacy"`} {
		if strings.Contains(nav, unwanted) {
			t.Errorf("nav still links %s", unwanted)
		}
	}
	if got := strings.Count(nav, "<li><a href="); got != 3 {
		t.Errorf("nav has %d menu links, want 3:\n%s", got, nav)
	}
	// The two static pages stay reachable from the site footer (the English
	// home's footer links the canonical Chinese utility paths).
	_, home := getHTML(t, r, "/en/")
	if !strings.Contains(home, `href="/faq"`) || !strings.Contains(home, `href="/privacy"`) {
		t.Error("footer lost the FAQ/Privacy links")
	}
}
