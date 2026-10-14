package handler_test

import (
	"encoding/xml"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"brickplans/internal/blog"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/router"
	"brickplans/internal/ssr"
	"brickplans/internal/storage"
)

// ── XML shapes (parsing also proves the documents are well-formed) ──

type xmlSitemapRef struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod"`
}

type xmlSitemapIndex struct {
	XMLName  xml.Name        `xml:"sitemapindex"`
	Sitemaps []xmlSitemapRef `xml:"sitemap"`
}

type xmlURL struct {
	Loc        string `xml:"loc"`
	Lastmod    string `xml:"lastmod"`
	Changefreq string `xml:"changefreq"`
	Priority   string `xml:"priority"`
}

type xmlURLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []xmlURL `xml:"url"`
}

// ── Fixture helpers ──

// setupSitemapTest builds an engine whose blog store is loaded from fixture
// markdown, so the per-category split can be asserted end to end. postCount is
// the number of synthetic posts (all in the 品牌推荐 bucket) appended after the
// hand-written fixtures; use >50 to cross the SSR page-size boundary.
func setupSitemapTest(t *testing.T, postCount int) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	contentDir := t.TempDir()
	writePost := func(slug, category string) {
		body := "---\n" +
			"title: \"" + slug + "\"\n" +
			"date: 2026-01-02\n" +
			"author: \"tester\"\n" +
			"category: \"" + category + "\"\n" +
			"tags: [a, b]\n" +
			"---\n\nbody\n"
		if err := os.WriteFile(filepath.Join(contentDir, slug+".md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	// Fixtures covering: canonical, legacy, spaced legacy, unknown, empty.
	writePost("sm-brand-canonical", "品牌推荐")
	writePost("sm-brand-legacy", "品牌评测")
	writePost("sm-beginner-legacy", "积木教程")
	writePost("sm-moc-spaced", "MOC 教程")
	writePost("sm-parts", "零件知识")
	writePost("sm-unknown", "一个不在映射表里的分类")
	writePost("sm-empty", "")
	for i := 0; i < postCount; i++ {
		writePost(fmt.Sprintf("sm-bulk-%03d", i), "品牌推荐")
	}

	dsn := filepath.Join(t.TempDir(), "test.db")
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: db.TestLogger()})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	gdb.Exec("PRAGMA foreign_keys = ON")
	if err := gdb.AutoMigrate(db.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{
		AppEnv:         "test",
		SecretKey:      testSecret,
		JWTAccessMin:   30,
		JWTRefreshDays: 7,
		CORSOrigins:    []string{"http://localhost:5173"},
		StorageBackend: "local",
		UploadDir:      filepath.Join(t.TempDir(), "uploads"),
		FrontendDist:   t.TempDir(),
		PublicURL:      "https://brickplans.test",
		BlogContentDir: contentDir,
	}
	storage.Reset()
	sqlDB, _ := gdb.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return router.New(cfg, gdb, ssr.NewRenderer(cfg.FrontendDist, cfg.PublicURL)), gdb
}

func getSitemap(t *testing.T, r *gin.Engine, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func parseURLSet(t *testing.T, path, body string) xmlURLSet {
	t.Helper()
	var set xmlURLSet
	if err := xml.Unmarshal([]byte(body), &set); err != nil {
		t.Fatalf("%s is not well-formed XML: %v", path, err)
	}
	return set
}

// fixtureBuckets mirrors the fixtures written by setupSitemapTest (excluding the
// bulk posts, which all live in the 品牌推荐 bucket).
var fixtureBuckets = map[string]string{
	"sm-brand-canonical": blog.BucketFor("品牌推荐"),
	"sm-brand-legacy":    blog.BucketFor("品牌评测"),
	"sm-beginner-legacy": blog.BucketFor("积木教程"),
	"sm-moc-spaced":      blog.BucketFor("MOC 教程"),
	"sm-parts":           blog.BucketFor("零件知识"),
	"sm-unknown":         blog.OtherBucketSlug,
	"sm-empty":           blog.OtherBucketSlug,
}

// TestBucketForMapsLegacyCategories pins the mapping semantics: the legacy
// near-duplicate category strings must land in the documented buckets, and
// anything unknown must degrade to the catch-all rather than vanish.
func TestBucketForMapsLegacyCategories(t *testing.T) {
	cases := map[string]string{
		"品牌推荐":    "brand",
		"品牌评测":    "brand",
		"选购指南":    "brand",
		"积木教程":    "beginner",
		"搭建教程":    "beginner",
		"新手入门":    "beginner",
		"零件知识":    "parts",
		"搭建技巧":    "building",
		"MOC搭建":   "building",
		"积木评测":    "set-review",
		"评测":      "set-review",
		"MOC图纸评测": "moc-review",
		"图纸评测":    "moc-review",
		"MOC评测":   "moc-review",
		"MOC教程":   "moc-tutorial",
		"MOC 教程":  "moc-tutorial", // legacy variant with a space
		"知识科普":    "knowledge",
		"资源推荐":    "resources",
		"":        blog.OtherBucketSlug,
		"没有这个分类":  blog.OtherBucketSlug,
		" 品牌推荐":   blog.OtherBucketSlug, // whitespace is NOT trimmed: be explicit, not clever
	}
	for raw, want := range cases {
		if got := blog.BucketFor(raw); got != want {
			t.Errorf("BucketFor(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestSitemapIndexListsEveryChildSitemap checks the index is complete and every
// child it advertises is actually served as a <urlset>.
func TestSitemapIndexListsEveryChildSitemap(t *testing.T) {
	r, _ := setupSitemapTest(t, 0)

	code, body := getSitemap(t, r, "/sitemap.xml")
	if code != 200 {
		t.Fatalf("/sitemap.xml: %d", code)
	}
	var index xmlSitemapIndex
	if err := xml.Unmarshal([]byte(body), &index); err != nil {
		t.Fatalf("sitemap index is not well-formed XML: %v", err)
	}

	wantPaths := []string{"/sitemap-pages.xml", "/sitemap-blueprints.xml"}
	for _, slug := range blog.BucketSlugs() {
		wantPaths = append(wantPaths, "/sitemap-blog-"+slug+".xml")
	}
	if len(index.Sitemaps) != len(wantPaths) {
		t.Fatalf("index has %d children, want %d", len(index.Sitemaps), len(wantPaths))
	}
	for _, want := range wantPaths {
		full := "https://brickplans.test" + want
		found := false
		for _, s := range index.Sitemaps {
			if s.Loc == full {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("index missing child %s", full)
		}
	}

	// Every advertised child must answer with a parseable urlset, never a
	// soft-404 HTML page.
	for _, s := range index.Sitemaps {
		path := strings.TrimPrefix(s.Loc, "https://brickplans.test")
		code, childBody := getSitemap(t, r, path)
		if code != 200 {
			t.Errorf("child %s: HTTP %d", path, code)
			continue
		}
		set := parseURLSet(t, path, childBody)
		for _, u := range set.URLs {
			if !strings.HasPrefix(u.Loc, "https://brickplans.test") {
				t.Errorf("child %s emitted non-absolute loc %q", path, u.Loc)
			}
		}
	}
}

// TestSitemapPartitionIsTotal is the load-bearing test: every fixture post URL
// appears in exactly one child sitemap, and no URL is duplicated across
// children. A post missing from every sitemap is an invisible de-indexing risk,
// and a duplicate wastes crawl budget and muddies the coverage report.
func TestSitemapPartitionIsTotal(t *testing.T) {
	const bulk = 12
	r, _ := setupSitemapTest(t, bulk)

	seen := map[string]string{} // loc -> first child that carried it
	for _, slug := range blog.BucketSlugs() {
		path := "/sitemap-blog-" + slug + ".xml"
		code, body := getSitemap(t, r, path)
		if code != 200 {
			t.Fatalf("%s: %d", path, code)
		}
		set := parseURLSet(t, path, body)

		// Expected count for this bucket, derived from the fixtures.
		want := 0
		for _, gotSlug := range fixtureBuckets {
			if gotSlug == slug {
				want++
			}
		}
		if slug == "brand" {
			want += bulk
		}
		if len(set.URLs) != want {
			t.Errorf("bucket %s has %d urls, want %d", slug, len(set.URLs), want)
		}
		for _, u := range set.URLs {
			if prev, dup := seen[u.Loc]; dup {
				t.Errorf("url %s listed in both %s and %s", u.Loc, prev, path)
			}
			seen[u.Loc] = path
		}
	}

	for slug, bucket := range fixtureBuckets {
		loc := "https://brickplans.test/blog/" + slug
		gotPath, ok := seen[loc]
		if !ok {
			t.Errorf("post %s (%s) is in NO child sitemap — it would be de-indexed", slug, bucket)
			continue
		}
		if want := "/sitemap-blog-" + bucket + ".xml"; gotPath != want {
			t.Errorf("post %s served from %s, want %s", slug, gotPath, want)
		}
	}
	if len(seen) != len(fixtureBuckets)+bulk {
		t.Errorf("blog children carried %d urls, want %d", len(seen), len(fixtureBuckets)+bulk)
	}
}

// TestSitemapBlueprintsChildHidesUnpublished guards the existing rule: only
// published blueprints are advertised.
func TestSitemapBlueprintsChildHidesUnpublished(t *testing.T) {
	r, _ := setupSitemapTest(t, 0)
	tok, _ := registerUser(t, r, "smpub", "smpub@x.com")
	pubID := createBlueprint(t, r, tok, "已发布作品", true)
	draftID := createBlueprint(t, r, tok, "未发布草稿", false)

	code, body := getSitemap(t, r, "/sitemap-blueprints.xml")
	if code != 200 {
		t.Fatalf("/sitemap-blueprints.xml: %d", code)
	}
	set := parseURLSet(t, "/sitemap-blueprints.xml", body)
	if len(set.URLs) != 1 {
		t.Fatalf("blueprints child has %d urls, want 1", len(set.URLs))
	}
	if !strings.HasSuffix(set.URLs[0].Loc, "/detail/"+pubID) {
		t.Errorf("published blueprint missing: %s", set.URLs[0].Loc)
	}
	if strings.Contains(body, draftID) {
		t.Error("unpublished blueprint leaked into the sitemap")
	}
}

// TestSitemapPagesChildListsBlogPagination checks the blog listing pagination
// pages are advertised — they are the crawl chain that reaches the archive.
func TestSitemapPagesChildListsBlogPagination(t *testing.T) {
	// 7 fixtures + 50 bulk = 57 posts → ceil(57/50) = 2 pages.
	r, gdb := setupSitemapTest(t, 50)
	if err := gdb.Create(&db.Tag{Name: "积木"}).Error; err != nil {
		t.Fatalf("seed tag: %v", err)
	}

	code, body := getSitemap(t, r, "/sitemap-pages.xml")
	if code != 200 {
		t.Fatalf("/sitemap-pages.xml: %d", code)
	}
	set := parseURLSet(t, "/sitemap-pages.xml", body)
	locs := map[string]bool{}
	for _, u := range set.URLs {
		locs[u.Loc] = true
	}
	for _, want := range []string{
		"https://brickplans.test/",
		"https://brickplans.test/explore",
		"https://brickplans.test/blog",
		"https://brickplans.test/blog?page=2",
		"https://brickplans.test/tags/" + url.PathEscape("积木"),
	} {
		if !locs[want] {
			t.Errorf("pages child missing %s", want)
		}
	}
	if locs["https://brickplans.test/blog?page=3"] {
		t.Error("pages child advertised a pagination page past the end")
	}
}

// TestSitemapUnknownBlogBucket404s ensures a bucket path that is not in the
// table cannot be served (a soft-404 HTML page here would be parsed as a broken
// sitemap by the crawler).
func TestSitemapUnknownBlogBucket404s(t *testing.T) {
	r, _ := setupSitemapTest(t, 0)
	code, body := getSitemap(t, r, "/sitemap-blog-not-a-bucket.xml")
	if code != 404 {
		t.Fatalf("/sitemap-blog-not-a-bucket.xml: HTTP %d, want 404", code)
	}
	if strings.Contains(body, "<urlset") {
		t.Fatal("unknown bucket returned a urlset")
	}
}
