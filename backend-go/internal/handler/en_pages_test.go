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

	_, detail := getHTML(t, r, "/en/detail/"+bpID)
	crumbs := headLines(detail, "BreadcrumbList")
	if !strings.Contains(crumbs, `"name":"Home"`) || !strings.Contains(crumbs, `"name":"Architecture"`) {
		t.Errorf("EN breadcrumb is not translated:\n%s", crumbs)
	}
	if strings.Contains(crumbs, `"name":"建筑"`) {
		t.Errorf("stored Chinese value leaked into the EN breadcrumb label:\n%s", crumbs)
	}
	if !strings.Contains(crumbs, `/en/explore?category=%E5%BB%BA%E7%AD%91`) {
		t.Errorf("category crumb left the /en tree (broken trail):\n%s", crumbs)
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
