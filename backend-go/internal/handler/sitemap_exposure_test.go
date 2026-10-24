package handler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The exposure ledger releases the 1,537 English translations 100/day. These
// tests pin the three properties the rollout depends on:
//   - no ledger file  → every English post is advertised (backwards compatible,
//     and the documented rollback);
//   - ledger present  → only the listed slugs appear in sitemap-en-blog-*.xml,
//     and buckets with nothing released are absent from the index;
//   - editing the file takes effect on the NEXT request, no restart (the file is
//     re-read when mtime/size moves).
func TestEnglishSitemapHonoursExposureLedger(t *testing.T) {
	extraEN := []fixturePost{
		{"sm-en-second", "Beginner Guide"},
		{"sm-en-third", "Parts Guide"},
	}
	r, _, enParent := setupSitemapFixture(t, 0, 0, nil, extraEN)
	ledger := filepath.Join(enParent, "sitemap-exposure.txt")

	// Baseline: no ledger on disk → pre-rollout behaviour, everything listed.
	_, body := getSitemap(t, r, "/sitemap-en-blog-beginner.xml")
	if !strings.Contains(body, "/en/blog/sm-en-only") {
		t.Fatalf("without a ledger every EN post must be advertised; got:\n%s", body)
	}
	if !strings.Contains(body, "/en/blog/sm-en-second") {
		t.Fatalf("without a ledger every EN post must be advertised; got:\n%s", body)
	}

	// Seed the ledger with a single slug from the beginner bucket.
	if err := os.WriteFile(ledger, []byte("# exposure ledger\nsm-en-only\n"), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	_, body = getSitemap(t, r, "/sitemap-en-blog-beginner.xml")
	if !strings.Contains(body, "/en/blog/sm-en-only") {
		t.Fatalf("seed slug must stay exposed; got:\n%s", body)
	}
	if strings.Contains(body, "/en/blog/sm-en-second") {
		t.Fatalf("unlisted slug leaked into the gated sitemap; got:\n%s", body)
	}

	// A bucket with zero exposed slugs must not be advertised by the index:
	// an empty <urlset> reads as "0 discovered pages" in Search Console.
	_, idx := getSitemap(t, r, "/sitemap.xml")
	if strings.Contains(idx, "sitemap-en-blog-parts") {
		t.Fatalf("bucket with no exposed post must be absent from the index; got:\n%s", idx)
	}
	if !strings.Contains(idx, "sitemap-en-blog-beginner") {
		t.Fatalf("bucket with an exposed post must stay in the index; got:\n%s", idx)
	}

	// Chinese tree is never gated by an English knob.
	_, zh := getSitemap(t, r, "/sitemap-zh-blog-parts.xml")
	if !strings.Contains(zh, "/blog/sm-parts") {
		t.Fatalf("Chinese sitemap must be unaffected by the ledger; got:\n%s", zh)
	}

	// Appending a slug must be visible on the next request (no restart).
	appended := "# exposure ledger\nsm-en-only\nsm-en-second\n"
	if err := os.WriteFile(ledger, []byte(appended), 0o644); err != nil {
		t.Fatalf("rewrite ledger: %v", err)
	}
	// Some filesystems have 1s mtime granularity; a different size also forces
	// the reload, which is exactly what the append path does in production.
	_, body = getSitemap(t, r, "/sitemap-en-blog-beginner.xml")
	if !strings.Contains(body, "/en/blog/sm-en-second") {
		t.Fatalf("appended slug must appear without a restart; got:\n%s", body)
	}
	if strings.Contains(body, "/en/blog/sm-en-third") {
		t.Fatalf("still-unlisted slug leaked; got:\n%s", body)
	}

	// Deleting the ledger restores the full corpus (rollback path).
	if err := os.Remove(ledger); err != nil {
		t.Fatalf("remove ledger: %v", err)
	}
	_, body = getSitemap(t, r, "/sitemap-en-blog-parts.xml")
	if !strings.Contains(body, "/en/blog/sm-en-third") {
		t.Fatalf("removing the ledger must re-advertise every EN post; got:\n%s", body)
	}
}
