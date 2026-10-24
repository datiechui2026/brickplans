package handler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The exposure ledger gates three surfaces, and they must move together:
//   - sitemap-en-blog-*.xml  → what we advertise;
//   - the article page       → noindex + X-Robots-Tag until released;
//   - hreflang annotations   → the subject of this file.
//
// The third one was missed in the first cut (400675b / aa9cf57): the Chinese
// article page and the Chinese sitemaps kept emitting hreflang="en" and
// x-default for every /en/blog/<slug>, i.e. roughly 1,381 pointers at pages
// that answer noindex. Google requires both sides of an hreflang pair to be
// indexable — the targets are reported as "Alternate page with noindex" and the
// annotation can be discarded for the whole cluster. Hence: same ledger, same
// answer, no drift.

const enPair = `hreflang="en" href="https://brickplans.test/en/blog/sm-parts"`

// TestHreflangSkipsUnreleasedEnglish covers the Chinese page and the Chinese
// sitemap, before and after the slug is released.
func TestHreflangSkipsUnreleasedEnglish(t *testing.T) {
	r, _, enParent := setupSitemapFixture(t, 0, 0, nil, []fixturePost{{"sm-en-second", "Beginner Guide"}})
	ledger := filepath.Join(enParent, "sitemap-exposure.txt")

	// Baseline: no ledger file → gate inactive → the pair is advertised exactly
	// as before the rollout (and as after `rm sitemap-exposure.txt`).
	code, body := getHTML(t, r, "/blog/sm-parts")
	if code != 200 {
		t.Fatalf("/blog/sm-parts: %d", code)
	}
	if !strings.Contains(body, enPair) {
		t.Fatalf("without a ledger the zh page must advertise the EN pair; got:\n%s", truncateForLog(body))
	}

	// Ledger present, sm-parts NOT released → the English side disappears from
	// the page, and x-default must not point into /en/ either.
	if err := os.WriteFile(ledger, []byte("# exposure ledger\nsm-en-only\n"), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}
	_, body = getHTML(t, r, "/blog/sm-parts")
	if strings.Contains(body, enPair) {
		t.Fatalf("unreleased EN translation must not be offered as an alternate (target is noindex); got:\n%s",
			truncateForLog(body))
	}
	if strings.Contains(body, `hreflang="x-default" href="https://brickplans.test/en/blog/sm-parts"`) {
		t.Fatalf("x-default must not point at the noindex English page; got:\n%s", truncateForLog(body))
	}
	if !strings.Contains(body, `hreflang="zh-CN" href="https://brickplans.test/blog/sm-parts"`) {
		t.Fatalf("the Chinese page must keep its own hreflang; got:\n%s", truncateForLog(body))
	}

	// The Chinese sitemap entry must not carry the noindex target either.
	_, sm := getSitemap(t, r, "/sitemap-zh-blog-parts.xml")
	if strings.Contains(sm, enPair) {
		t.Fatalf("zh sitemap entry must not point at a noindex EN page; got:\n%s", truncateForLog(sm))
	}

	// Releasing the slug restores the full cluster on the NEXT request — no
	// restart, no cache to bust (the same mtime/size reload as the sitemaps).
	if err := os.WriteFile(ledger, []byte("# exposure ledger\nsm-en-only\nsm-parts\n"), 0o644); err != nil {
		t.Fatalf("rewrite ledger: %v", err)
	}
	_, body = getHTML(t, r, "/blog/sm-parts")
	if !strings.Contains(body, enPair) {
		t.Fatalf("released translation must be advertised again; got:\n%s", truncateForLog(body))
	}
	_, sm = getSitemap(t, r, "/sitemap-zh-blog-parts.xml")
	if !strings.Contains(sm, enPair) {
		t.Fatalf("released translation must be back in the zh sitemap entry; got:\n%s", truncateForLog(sm))
	}
}

// TestEnglishHomeLinksOnlyReleasedArticles pins the secondary leak: /en/ listed
// the first 60 English posts by date without consulting the ledger, so the
// strongest internal link source on the site still handed crawlers unreleased
// (noindex) URLs. Filtered now, which also keeps the homepage and /en/blog in
// agreement.
func TestEnglishHomeLinksOnlyReleasedArticles(t *testing.T) {
	r, _, enParent := setupSitemapFixture(t, 0, 0, nil, []fixturePost{{"sm-en-second", "Beginner Guide"}})
	ledger := filepath.Join(enParent, "sitemap-exposure.txt")
	if err := os.WriteFile(ledger, []byte("# exposure ledger\nsm-en-only\n"), 0o644); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	code, body := getHTML(t, r, "/en/")
	if code != 200 {
		t.Fatalf("/en/: %d", code)
	}
	if strings.Contains(body, "https://brickplans.test/en/blog/sm-en-second") {
		t.Fatalf("the English home must not link an unreleased article; got:\n%s", truncateForLog(body))
	}
	if !strings.Contains(body, "https://brickplans.test/en/blog/sm-en-only") {
		t.Fatalf("the English home must keep linking released articles; got:\n%s", truncateForLog(body))
	}
}
