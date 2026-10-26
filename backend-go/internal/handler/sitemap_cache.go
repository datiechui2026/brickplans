package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// sitemapCache serves every sitemap from a rendered snapshot instead of
// re-assembling the XML inside the request.
//
// Why this exists (2026-10-25, P0-2): until now each sitemap request ran
// `SELECT * FROM blueprints` (6.4k rows), rebuilt ~1-5 MB of XML, and answered
// with no Last-Modified / ETag / Cache-Control at all. Search Console's
// sitemaps().get() showed nine English blog children stuck at 21 of 255 URLs
// (four of them never parsed at all) — a crawler with no validator re-reads a
// URL it cannot confirm is unchanged, or skips it. Serving a snapshot fixes
// both halves: the request path touches no database, and the response carries
// real validators.
//
// Two modes:
//   - dir == "": memory only. Tests and any process started without
//     SITEMAP_DIR use this; behaviour is the same minus the files.
//   - dir != "": each document is also written to <dir>/<name>, atomically.
//
// The invariant that makes lastmod honest: a rebuild that produces byte-identical
// XML does NOT rewrite the file, so the mtime of a child file is exactly the
// time its content last changed. The sitemap index advertises that mtime as the
// child's <lastmod>, which is why the index follows content changes instead of
// being a constant someone hand-edited (the old code advertised
// sitemap-en-blog-brand.xml as lastmod=2026-10-01 while its content had moved).
type sitemapCache struct {
	dir string

	mu   sync.RWMutex
	docs map[string]*sitemapDoc
}

// sitemapDoc is one rendered sitemap plus the validators derived from it.
type sitemapDoc struct {
	body []byte
	etag string // quoted strong validator, sha256 of the body
	mod  time.Time
}

func newSitemapCache(dir string) *sitemapCache {
	return &sitemapCache{dir: dir, docs: make(map[string]*sitemapDoc, 24)}
}

// makeSitemapDoc derives the validators once, at render time, so serving can
// never disagree with the bytes it sends.
func makeSitemapDoc(body []byte, mod time.Time) *sitemapDoc {
	sum := sha256.Sum256(body)
	return &sitemapDoc{
		body: body,
		etag: `"` + hex.EncodeToString(sum[:16]) + `"`,
		// HTTP dates have second granularity; truncating here keeps
		// Last-Modified and If-Modified-Since comparable without surprises.
		mod: mod.UTC().Truncate(time.Second),
	}
}

func (c *sitemapCache) get(name string) *sitemapDoc {
	c.mu.RLock()
	doc := c.docs[name]
	c.mu.RUnlock()
	return doc
}

// sitemapDirty is bumped by every write that changes what a sitemap should
// contain (blueprint create / update / delete / publish / unpublish). The
// snapshot compares it with the value it rendered with, so a freshly published
// blueprint is advertised on the very next sitemap request instead of waiting
// for the ticker — the freshness the per-request rendering used to give, kept
// without paying for a full table scan on every crawl.
//
// A package-level counter rather than a field handed to the write handlers:
// blueprints.go and admin.go have no reference to the SEO handler, and plumbing
// one through would touch every constructor for a single counter.
var sitemapDirty atomic.Int64

// InvalidateSitemaps marks the sitemap snapshot stale. Call it after a
// successful write that changes the public blueprint set.
func InvalidateSitemaps() { sitemapDirty.Add(1) }

// put installs body as the current snapshot for name and returns the document
// that will be served. mod is the time this content last *changed*, not the
// time of this call: identical bytes reuse the previous mtime.
func (c *sitemapCache) put(name string, body []byte) *sitemapDoc {
	if prev := c.get(name); prev != nil && bytes.Equal(prev.body, body) {
		return prev
	}

	mod := time.Now()
	if c.dir != "" {
		path := filepath.Join(c.dir, name)
		// A fresh process starts with an empty map while the files survive the
		// restart. Reuse their mtime when the bytes match, otherwise every
		// deploy would bump Last-Modified on 23 unchanged files and the index
		// would claim a full refresh that never happened.
		onDisk := false
		if fi, err := os.Stat(path); err == nil {
			if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, body) {
				mod, onDisk = fi.ModTime(), true
			}
		}
		if !onDisk {
			if err := writeFileAtomic(path, body); err != nil {
				// Serving from memory still works; the file on disk is only
				// what the index derives lastmod from, so log and keep going.
				log.Printf("[sitemap] write %s: %v", path, err)
			}
		}
	}

	doc := makeSitemapDoc(body, mod)
	c.mu.Lock()
	c.docs[name] = doc
	c.mu.Unlock()
	return doc
}

// serve writes one cached document with the three headers a crawler needs to
// decide whether it has to re-read the file, and answers 304 when the client
// already holds this exact version.
func (c *sitemapCache) serve(ctx *gin.Context, name string) {
	doc := c.get(name)
	if doc == nil {
		// Not a normal state: the snapshot is rendered before the listener
		// accepts traffic. 503 (retryable) is honest; a 404 would tell Google
		// the sitemap was removed.
		ctx.Status(http.StatusServiceUnavailable)
		return
	}

	h := ctx.Writer.Header()
	h.Set("ETag", doc.etag)
	h.Set("Last-Modified", doc.mod.Format(http.TimeFormat))
	h.Set("Cache-Control", "public, max-age=3600")

	// RFC 9110: If-None-Match wins over If-Modified-Since, so a present but
	// non-matching ETag must produce a full body, not a 304.
	if inm := ctx.GetHeader("If-None-Match"); inm != "" {
		if etagMatches(inm, doc.etag) {
			ctx.Status(http.StatusNotModified)
			return
		}
	} else if ims := ctx.GetHeader("If-Modified-Since"); ims != "" {
		if t, err := http.ParseTime(ims); err == nil && !doc.mod.After(t) {
			ctx.Status(http.StatusNotModified)
			return
		}
	}

	ctx.Data(http.StatusOK, "application/xml; charset=utf-8", doc.body)
}

// etagMatches implements the If-None-Match comparison: a comma-separated list,
// "*" for any, and the weak prefix tolerated (our ETags are strong, but a proxy
// in front may weaken them).
func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}

// writeFileAtomic writes body to path via a temp file in the same directory, so
// a crawler never reads a half-written sitemap and a crash mid-write leaves the
// previous version in place.
func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sitemap-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
