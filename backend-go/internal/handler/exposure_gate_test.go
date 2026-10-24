package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The English rollout has TWO gates, and they must agree:
//
//	1. navigation — /en/blog and /api/blog?lang=en only link released slugs, so
//	   a crawler cannot walk the listing into the 1,382 unreleased articles;
//	2. indexation — an unreleased article page answers 200 (readers keep their
//	   links) but carries <meta name="robots" content="noindex, follow"> and the
//	   equivalent X-Robots-Tag header, so article-to-article links cannot
//	   launder it into the index either.
//
// Released articles must be byte-identical to the pre-rollout page: no header,
// no meta tag. These tests pin both directions plus the Chinese tree.

// writeExposureLedger seeds content/en/sitemap-exposure.txt next to the EN
// content dir — the same path the router derives at startup.
func writeExposureLedger(t *testing.T, enParent, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(enParent, "sitemap-exposure.txt"), []byte(body), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
}

func serveGet(t *testing.T, r http.Handler, path string) (int, http.Header, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Header(), rec.Body.String()
}

func TestEnglishListingHonoursExposureLedger(t *testing.T) {
	extraEN := []fixturePost{
		{"sm-en-second", "Beginner Guide"},
		{"sm-en-third", "Parts Guide"},
	}
	r, _, enParent := setupSitemapFixture(t, 0, 0, nil, extraEN)

	// No ledger → everything listed (pre-rollout behaviour, also the rollback).
	code, _, body := serveGet(t, r, "/en/blog?page=1")
	if code != http.StatusOK {
		t.Fatalf("/en/blog = %d, want 200", code)
	}
	if !strings.Contains(body, "/en/blog/sm-en-second") {
		t.Fatalf("without a ledger the listing must link every EN post; got:\n%s", truncateForLog(body))
	}

	writeExposureLedger(t, enParent, "# exposure ledger\nsm-en-only\n")

	// SSR listing: released in, unreleased out.
	code, _, body = serveGet(t, r, "/en/blog?page=1")
	if code != http.StatusOK {
		t.Fatalf("/en/blog = %d, want 200", code)
	}
	if !strings.Contains(body, "/en/blog/sm-en-only") {
		t.Fatalf("released slug must stay linked from the listing; got:\n%s", truncateForLog(body))
	}
	for _, hidden := range []string{"sm-en-second", "sm-en-third"} {
		if strings.Contains(body, "/en/blog/"+hidden) {
			t.Fatalf("unreleased slug %q leaked into the gated listing", hidden)
		}
	}
	// Pagination must shrink to the released set, not merely skip links.
	if strings.Contains(body, "page=2") || strings.Contains(body, "page 2") {
		t.Fatalf("gated listing must be a single page for 1 released post; got:\n%s", truncateForLog(body))
	}

	// SPA hydration path: same gate, because /en/blog hydrates from this API.
	code, _, apiBody := serveGet(t, r, "/api/blog?lang=en&page_size=100")
	if code != http.StatusOK {
		t.Fatalf("/api/blog?lang=en = %d, want 200", code)
	}
	var resp struct {
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
		Total      int      `json:"total"`
		Categories []string `json:"categories"`
	}
	if err := json.Unmarshal([]byte(apiBody), &resp); err != nil {
		t.Fatalf("decode /api/blog: %v", err)
	}
	var got []string
	for _, it := range resp.Items {
		got = append(got, it.Slug)
	}
	if len(got) != 1 || got[0] != "sm-en-only" {
		t.Fatalf("gated API listing = %v, want [sm-en-only]", got)
	}
	if resp.Total != 1 {
		t.Fatalf("API total = %d, want 1 (paginator must count released posts)", resp.Total)
	}
	for _, c := range resp.Categories {
		if c == "Parts Guide" {
			t.Fatalf("category %q has no released post and must not be offered; got %v", c, resp.Categories)
		}
	}

	// Chinese listing is untouched by an English knob.
	code, _, zh := serveGet(t, r, "/blog?page=1")
	if code != http.StatusOK || !strings.Contains(zh, "/blog/sm-parts") {
		t.Fatalf("Chinese listing must be unaffected; code=%d body:\n%s", code, truncateForLog(zh))
	}
}

func TestUnreleasedEnglishArticleIsNoindex(t *testing.T) {
	r, _, enParent := setupSitemapFixture(t, 0, 0, nil, []fixturePost{{"sm-en-second", "Beginner Guide"}})
	writeExposureLedger(t, enParent, "# exposure ledger\nsm-en-only\n")

	// Unreleased: still 200 (links keep working) but noindex in BOTH forms.
	code, hdr, body := serveGet(t, r, "/en/blog/sm-en-second")
	if code != http.StatusOK {
		t.Fatalf("unreleased EN article = %d, want 200 (noindex, not 404)", code)
	}
	if got := hdr.Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
		t.Fatalf("X-Robots-Tag = %q, want it to contain noindex", got)
	}
	if !strings.Contains(body, `<meta name="robots" content="noindex, follow"`) {
		t.Fatalf("unreleased article must carry the noindex meta; got:\n%s", truncateForLog(body))
	}

	// Released: no header at all, no robots meta — byte-compatible with the page
	// Google already had before the gate existed.
	code, hdr, body = serveGet(t, r, "/en/blog/sm-en-only")
	if code != http.StatusOK {
		t.Fatalf("released EN article = %d, want 200", code)
	}
	if got := hdr.Get("X-Robots-Tag"); got != "" {
		t.Fatalf("released article must not send X-Robots-Tag; got %q", got)
	}
	if strings.Contains(body, `name="robots"`) {
		t.Fatalf("released article must not carry a robots meta; got:\n%s", truncateForLog(body))
	}

	// HEAD must agree with GET (the pre-2026-10-24 bug was HEAD saying 200 while
	// GET said 404; a header-only gate has to survive the same rewrite path).
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("HEAD", "/en/blog/sm-en-second", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatalf("HEAD unreleased = %d, X-Robots-Tag=%q; want 200 + noindex",
			rec.Code, rec.Header().Get("X-Robots-Tag"))
	}

	// Chinese article pages never carry the English gate.
	code, hdr, _ = serveGet(t, r, "/blog/sm-parts")
	if code != http.StatusOK {
		t.Fatalf("Chinese article = %d, want 200", code)
	}
	if got := hdr.Get("X-Robots-Tag"); got != "" {
		t.Fatalf("Chinese article must not send X-Robots-Tag; got %q", got)
	}
}

func truncateForLog(s string) string {
	if len(s) > 600 {
		return s[:600] + "…"
	}
	return s
}
