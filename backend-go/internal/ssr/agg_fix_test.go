package ssr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/gin-gonic/gin"

	"brickplans/internal/db"
)

// Follow-up to SEO-20261024, defect 1: the English aggregate pages printed the
// stored Chinese category next to every list item -- 72 "(奇幻)" on
// /en/category/fantasy, ~1000 across the 13 English landing pages. The listing
// renderer now goes through categoryLabelEN on the English tree, the same table
// the /en spec table and breadcrumbs already used.

// TestListItemsCategoryLabels pins both trees: Chinese pages keep the stored
// value, English pages translate it, and a value with no translation passes
// through unchanged (dropping it would leave an empty "( )").
func TestListItemsCategoryLabels(t *testing.T) {
	cases := []struct {
		lang     string
		category string
		want     string
		absent   string
	}{
		{"zh", "奇幻", "奇幻", "(Fantasy)"},
		{"en", "奇幻", "Fantasy", "奇幻"},
		{"en", "星战", "Star Wars", "星战"},   // second-tier translation
		{"zh", "星战", "星战", "Star Wars"},   // Chinese tree keeps stored value
		{"en", "未收录", "未收录", "(Fantasy)"}, // no translation: pass through
	}
	for _, tc := range cases {
		bp := bpFixture()
		cat := tc.category
		bp.Category = &cat
		html := string(listNoscriptFor(tc.lang, []db.Blueprint{*bp}, "https://brickplan.test"))
		if !strings.Contains(html, tc.want) {
			t.Errorf("lang=%s category=%q: %q missing from %s", tc.lang, tc.category, tc.want, html)
		}
		if strings.Contains(html, tc.absent) {
			t.Errorf("lang=%s category=%q: %q must not appear in %s", tc.lang, tc.category, tc.absent, html)
		}
	}
}

// An English aggregate page body must carry no Chinese at all: the list items,
// the spec-less chrome and the cross-link blocks are the only text on it.
func TestEnglishAggregateBodyHasNoCJK(t *testing.T) {
	cat := "奇幻"
	bps := make([]db.Blueprint, 0, 3)
	for i := 0; i < 3; i++ {
		bp := bpFixture()
		bp.ID = "aaaaaaaa-0000-0000-0000-00000000000" + string(rune('1'+i))
		bp.TitleEN = strPtr("Fantasy Castle")
		bp.Category = &cat
		bps = append(bps, *bp)
	}
	html := string(listNoscriptFor("en", bps, "https://brickplan.test"))
	if hasCJK(html) {
		t.Fatalf("Chinese leaked into the English listing: %s", html)
	}
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// Defect 2: the parameter URLs still answered 200 with a self-canonicalised
// duplicate of the landing page while being absent from the sitemap and from
// detail breadcrumbs. They now 301 onto the landing page, carrying ?page=N.

func redirectFor(t *testing.T, lang, target string) (bool, int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	h := &Handler{}
	done := h.redirectExploreFilter(c, lang)
	return done, w.Code, w.Header().Get("Location")
}

func TestExploreFilterRedirectsToLandingPage(t *testing.T) {
	cases := []struct {
		name   string
		lang   string
		target string
		want   string
	}{
		{"category zh", "zh", "/explore?category=%E5%A5%87%E5%B9%BB", "/category/fantasy"},
		{"category en", "en", "/en/explore?category=%E5%A5%87%E5%B9%BB", "/en/category/fantasy"},
		{"brand zh", "zh", "/explore?brand=%E5%A5%87%E7%A7%AF", "/brand/qiji"},
		{"brand lego", "zh", "/explore?brand=%E4%B9%90%E9%AB%98", "/brand/lego"},
		{"difficulty", "zh", "/explore?difficulty=3", "/difficulty/3"},
		{"page carried", "zh", "/explore?category=%E5%9C%BA%E6%99%AF&page=3", "/category/scene?page=3"},
		{"page 1 dropped", "zh", "/explore?category=%E5%9C%BA%E6%99%AF&page=1", "/category/scene"},
	}
	for _, tc := range cases {
		done, code, loc := redirectFor(t, tc.lang, tc.target)
		if !done {
			t.Errorf("%s: no redirect for %s", tc.name, tc.target)
			continue
		}
		if code != http.StatusMovedPermanently {
			t.Errorf("%s: status = %d, want 301", tc.name, code)
		}
		if loc != tc.want {
			t.Errorf("%s: Location = %q, want %q", tc.name, loc, tc.want)
		}
	}
}

// Values with no landing page, and filter combinations (which have no landing
// page either), keep the old behaviour: 200, no redirect.
func TestExploreFilterLeavesUnmappedFiltersAlone(t *testing.T) {
	cases := []struct{ name, lang, target string }{
		{"no landing page", "zh", "/explore?category=%E6%98%9F%E6%88%98"}, // 星战
		{"other bucket", "zh", "/explore?category=%E5%85%B6%E4%BB%96"},    // 其他
		{"brand without page", "zh", "/explore?brand=%E6%A3%AE%E5%AE%9D"}, // 森宝
		{"two filters", "zh", "/explore?category=%E5%A5%87%E5%B9%BB&brand=%E4%B9%90%E9%AB%98"},
		{"no filter", "zh", "/explore"},
		{"unknown difficulty", "zh", "/explore?difficulty=9"},
	}
	for _, tc := range cases {
		done, _, loc := redirectFor(t, tc.lang, tc.target)
		if done {
			t.Errorf("%s: unexpected redirect for %s -> %q", tc.name, tc.target, loc)
		}
	}
}
