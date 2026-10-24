package blog

import (
	"os"
	"strings"
	"sync"
	"time"
)

// ExposureLedger gates which English articles are advertised in the English
// article sitemaps (`sitemap-en-blog-*.xml`).
//
// Why it exists: 1,537 English translations landed in a single batch. Putting
// every one of them into <urlset> on the same day is exactly the shape Google's
// scaled-content-abuse policy looks for — a thousand never-before-seen URLs, no
// inbound links, one publication date. The ledger advertises a growing subset
// (100/day) instead, while every article stays live and crawlable. Gating the
// *sitemap* rather than the *page* is deliberate: an already-linked article must
// keep answering 200 for readers and crawlers that arrive from elsewhere.
//
// File format (content/en/sitemap-exposure.txt): one slug per line, UTF-8,
// exactly as it appears in the URL (/en/blog/<slug>). Blank lines and lines
// starting with '#' are ignored, so the file can carry a dated header.
//
// Semantics, in order of precedence:
//   - file missing / unreadable → no gate, every post is advertised. This is
//     the behaviour before 2026-10-24 and keeps the binary safe to deploy
//     before the seed file exists.
//   - file present → only slugs listed in it are advertised.
//
// The file is re-read whenever its mtime or size changes, so appending slugs
// with scripts/brickplan_en_sitemap_release.py takes effect on the next sitemap
// request — no restart, no cache to bust.
type ExposureLedger struct {
	path string

	mu      sync.Mutex
	loaded  bool
	exists  bool
	slugs   map[string]struct{}
	modTime time.Time
	size    int64
}

// NewExposureLedger returns a ledger reading path. A nil ledger, an empty path
// and a missing file all mean "advertise everything" (see Allows).
func NewExposureLedger(path string) *ExposureLedger {
	return &ExposureLedger{path: path}
}

// Allows reports whether slug may be advertised in the English sitemaps.
// Callers never need a nil check.
func (l *ExposureLedger) Allows(slug string) bool {
	if l == nil || l.path == "" {
		return true
	}
	l.refresh()
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.exists {
		return true
	}
	_, ok := l.slugs[slug]
	return ok
}

// Active reports whether a ledger file is present and therefore gating.
func (l *ExposureLedger) Active() bool {
	if l == nil || l.path == "" {
		return false
	}
	l.refresh()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.exists
}

// Len is the number of slugs currently exposed (0 when the gate is inactive).
func (l *ExposureLedger) Len() int {
	if l == nil || l.path == "" {
		return 0
	}
	l.refresh()
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.slugs)
}

// Visible reports whether the article page at slug may be indexed. Chinese
// posts are never gated; English posts are gated by the same ledger that drives
// the sitemaps, so "in sitemap" and "indexable" cannot drift apart.
func (l *ExposureLedger) Visible(lang, slug string) bool {
	if lang != LangEN {
		return true
	}
	return l.Allows(slug)
}

// FilterVisible drops English posts that are not released yet.
//
// Why a listing has to be gated at all: the sitemap gate only controls what we
// *advertise*. The English listing (/en/blog, its pagination and the category
// filter behind /api/blog?lang=en) links every article, so a crawler that reads
// the listing discovers the whole unreleased corpus no matter what the sitemap
// says. The listing therefore shows only released posts; unreleased pages stay
// reachable (see Visible / the noindex they carry) but stop being advertised by
// navigation.
//
// A nil ledger, an empty path or a missing file returns posts untouched, so the
// gate cannot accidentally empty the site before the ledger is deployed.
func FilterVisible(posts []Post, lang string, l *ExposureLedger) []Post {
	if lang != LangEN || !l.Active() {
		return posts
	}
	out := make([]Post, 0, len(posts))
	for _, p := range posts {
		if l.Allows(p.Slug) {
			out = append(out, p)
		}
	}
	return out
}

// CategoriesOf returns the distinct non-empty categories of posts, in the same
// first-seen order the store uses, so a gated listing's filter bar matches the
// posts the listing actually shows.
func CategoriesOf(posts []Post) []string {
	seen := make(map[string]bool, 16)
	var cats []string
	for _, p := range posts {
		if p.Category != "" && !seen[p.Category] {
			seen[p.Category] = true
			cats = append(cats, p.Category)
		}
	}
	return cats
}

// refresh reloads the file only when the on-disk state moved. Stat is cheap
// (one syscall per sitemap request) and a handful of these files are served per
// crawl, so this stays well inside any budget worth measuring.
func (l *ExposureLedger) refresh() {
	fi, err := os.Stat(l.path)
	if err != nil {
		l.mu.Lock()
		l.loaded, l.exists, l.slugs = true, false, nil
		l.modTime, l.size = time.Time{}, -1
		l.mu.Unlock()
		return
	}

	l.mu.Lock()
	upToDate := l.loaded && l.exists && fi.Size() == l.size && fi.ModTime().Equal(l.modTime)
	l.mu.Unlock()
	if upToDate {
		return
	}

	data, err := os.ReadFile(l.path)
	if err != nil {
		// Raced with a rewrite (e.g. the release script's atomic rename) or the
		// file lost permissions: keep the last good view rather than flipping to
		// "advertise everything", which would leak the whole corpus.
		l.mu.Lock()
		if !l.loaded {
			l.loaded, l.exists, l.slugs = true, false, nil
		}
		l.mu.Unlock()
		return
	}

	slugs := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		slugs[line] = struct{}{}
	}

	l.mu.Lock()
	l.loaded, l.exists, l.slugs = true, true, slugs
	l.modTime, l.size = fi.ModTime(), fi.Size()
	l.mu.Unlock()
}
