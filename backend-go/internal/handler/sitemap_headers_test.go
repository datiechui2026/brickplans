package handler_test

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"brickplans/internal/blog"
	"brickplans/internal/config"
	"brickplans/internal/router"
	"brickplans/internal/ssr"
)

// P0-2 regression guards (2026-10-25).
//
// Before the static snapshot, every sitemap was assembled per request and
// answered with nothing a crawler could use to decide whether it had to re-read
// the file: no Last-Modified, no ETag, no Cache-Control. Search Console then
// showed nine English blog children stuck at 21 of 255 URLs. These tests exist
// so that "no validators" cannot come back silently — an assertion is the only
// thing that stops the next refactor from dropping the headers again.

// sitemapGet issues a GET with optional request headers (case-insensitive keys).
func sitemapGet(t *testing.T, r http.Handler, path string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// validETag reports whether v looks like the quoted strong validator we emit
// (16-byte sha256 prefix, hex) rather than a weak or empty value.
func validETag(v string) bool {
	if len(v) != 34 || v[0] != '"' || v[len(v)-1] != '"' {
		return false
	}
	for _, c := range v[1 : len(v)-1] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func TestSitemapResponsesCarryFreshnessHeaders(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)

	paths := []string{
		"/sitemap.xml",
		"/sitemap-zh.xml",
		"/sitemap-en.xml",
		"/sitemap-zh-blog-brand.xml",
		"/sitemap-en-blog-parts.xml",
		// The child Search Console reported as a 404 before the snapshot
		// existed; it must carry validators like every other one.
		"/sitemap-en-blog-building.xml",
	}

	for _, p := range paths {
		w := sitemapGet(t, r, p, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", p, w.Code)
		}
		if w.Body.Len() == 0 {
			t.Fatalf("%s: empty body", p)
		}
		etag := w.Header().Get("ETag")
		if !validETag(etag) {
			t.Fatalf("%s: ETag = %q, want a quoted 32-hex strong validator", p, etag)
		}
		lm := w.Header().Get("Last-Modified")
		mod, err := http.ParseTime(lm)
		if err != nil {
			t.Fatalf("%s: Last-Modified = %q is not an HTTP date: %v", p, lm, err)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
			t.Fatalf("%s: Cache-Control = %q", p, cc)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
			t.Fatalf("%s: Content-Type = %q", p, ct)
		}

		// A crawler that already holds this version must be told so.
		if got := sitemapGet(t, r, p, map[string]string{"If-None-Match": etag}); got.Code != http.StatusNotModified {
			t.Fatalf("%s: If-None-Match with the current ETag = %d, want 304", p, got.Code)
		}
		// If-None-Match wins over If-Modified-Since (RFC 9110): a present but
		// non-matching ETag must produce the full body even when the date says
		// "not modified".
		stale := sitemapGet(t, r, p, map[string]string{
			"If-None-Match":     `"deadbeefdeadbeefdeadbeefdeadbeef"`,
			"If-Modified-Since": lm,
		})
		if stale.Code != http.StatusOK || stale.Body.Len() == 0 {
			t.Fatalf("%s: stale ETag + fresh date = %d (body %d bytes), want 200 with body", p, stale.Code, stale.Body.Len())
		}
		// Date-only clients (no ETag) are still served.
		if got := sitemapGet(t, r, p, map[string]string{"If-Modified-Since": mod.UTC().Format(http.TimeFormat)}); got.Code != http.StatusNotModified {
			t.Fatalf("%s: If-Modified-Since at the file mtime = %d, want 304", p, got.Code)
		}
	}
}

// TestSitemapSnapshotValidatorsFollowContent pins the two properties the
// on-disk snapshot buys us: the index advertises each child's real
// content-change time, and a restart (or a rebuild that changes nothing) does
// not make 23 files look freshly edited.
func TestSitemapSnapshotValidatorsFollowContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sitemap-static")
	r1, gdb, enParent, cfg := setupSitemapFixtureFull(t, 0, 0, nil, nil, func(c *config.Config) {
		c.SitemapDir = dir
		c.SitemapRefreshMin = 0 // no background ticker inside a test
	})

	bucket := blog.BucketFor("Parts Guide")
	path := "/sitemap-en-blog-" + bucket + ".xml"

	first := sitemapGet(t, r1, path, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200", path, first.Code)
	}
	etag1 := first.Header().Get("ETag")
	lm1 := first.Header().Get("Last-Modified")
	if !strings.Contains(first.Body.String(), "/en/blog/sm-parts") {
		t.Fatalf("%s: fixture post not advertised:\n%s", path, first.Body.String())
	}

	// Every index entry must advertise the mtime of the file it points at —
	// that is the "lastmod follows content, not a hand-maintained date" rule.
	idx := sitemapGet(t, r1, "/sitemap.xml", nil)
	var parsed xmlSitemapIndex
	if err := xml.Unmarshal(idx.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("/sitemap.xml is not well-formed: %v", err)
	}
	if len(parsed.Sitemaps) == 0 {
		t.Fatal("/sitemap.xml lists no children")
	}
	for _, ref := range parsed.Sitemaps {
		name := strings.TrimPrefix(ref.Loc, cfg.PublicURL+"/")
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: index advertises %s but no file exists: %v", name, ref.Loc, err)
		}
		want := fi.ModTime().UTC().Truncate(time.Second).Format("2006-01-02T15:04:05-07:00")
		if ref.Lastmod != want {
			t.Fatalf("%s: index lastmod = %q, want file mtime %q", name, ref.Lastmod, want)
		}
	}

	// A restart is a new engine over the same config and the same directory:
	// identical bytes must keep their mtime, ETag and Last-Modified.
	r2 := router.New(cfg, gdb, ssr.NewRenderer(cfg.FrontendDist, cfg.PublicURL))
	second := sitemapGet(t, r2, path, nil)
	if got := second.Header().Get("ETag"); got != etag1 {
		t.Fatalf("restart changed the ETag: %q -> %q", etag1, got)
	}
	if got := second.Header().Get("Last-Modified"); got != lm1 {
		t.Fatalf("restart changed Last-Modified: %q -> %q", lm1, got)
	}

	// Now change what the file should contain: release a different English slug,
	// which empties this bucket. HTTP dates have 1s granularity, so cross a
	// second boundary before the change or the two Last-Modified values can
	// legitimately be equal.
	time.Sleep(1100 * time.Millisecond)
	ledger := filepath.Join(enParent, "sitemap-exposure.txt")
	if err := os.WriteFile(ledger, []byte("# test ledger\nsm-en-only\n"), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	third := sitemapGet(t, r2, path, nil)
	if third.Header().Get("ETag") == etag1 {
		t.Fatal("content changed but the ETag did not: a crawler would keep the stale copy forever")
	}
	if third.Header().Get("Last-Modified") == lm1 {
		t.Fatalf("content changed but Last-Modified stayed at %q", lm1)
	}
	if strings.Contains(third.Body.String(), "/en/blog/sm-parts") {
		t.Fatalf("unreleased post still advertised after the ledger change:\n%s", third.Body.String())
	}
	// The validator a crawler cached before the change must no longer 304.
	old := sitemapGet(t, r2, path, map[string]string{"If-None-Match": etag1})
	if old.Code != http.StatusOK || old.Body.Len() == 0 {
		t.Fatalf("stale ETag after a content change = %d (body %d bytes), want 200 with body", old.Code, old.Body.Len())
	}
}
