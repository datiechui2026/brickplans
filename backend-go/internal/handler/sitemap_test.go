package handler_test

import (
	"encoding/json"
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

type xmlAltLink struct {
	Rel      string `xml:"rel,attr"`
	Hreflang string `xml:"hreflang,attr"`
	Href     string `xml:"href,attr"`
}

type xmlURL struct {
	Loc        string       `xml:"loc"`
	Lastmod    string       `xml:"lastmod"`
	Changefreq string       `xml:"changefreq"`
	Priority   string       `xml:"priority"`
	Links      []xmlAltLink `xml:"http://www.w3.org/1999/xhtml link"`
}

type xmlURLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []xmlURL `xml:"url"`
}

// ── Fixtures ──

// fixture posts. zhSlug/zhCat and enSlug/enCat are intentionally the same slug
// for the "paired" entries — that is the contract the design doc fixes (one
// slug, two language trees).
type fixturePost struct {
	slug string
	cat  string
}

var zhFixtures = []fixturePost{
	{"sm-parts", "零件知识"}, // also has an English translation
	{"sm-brand-canonical", "品牌推荐"},
	{"sm-brand-legacy", "品牌评测"},
	{"sm-beginner-legacy", "积木教程"},
	{"sm-moc-spaced", "MOC 教程"},
	{"sm-unknown", "一个不在映射表里的分类"},
	{"sm-empty", ""},
}

var enFixtures = []fixturePost{
	{"sm-parts", "Parts Guide"},      // pair with the zh post above
	{"sm-en-only", "Beginner Guide"}, // English only
}

func fixtureBucket(p fixturePost) string { return blog.BucketFor(p.cat) }

// setupSitemapTest builds an engine whose blog store is loaded from zh + en
// fixture markdown, so the language × category split can be asserted end to end.
// bulkZH/bulkEN are extra single-language posts (all in one bucket) used to
// cross the SSR page-size boundary.
func setupSitemapTest(t *testing.T, bulkZH, bulkEN int) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	zhDir := filepath.Join(t.TempDir(), "blog")
	enDir := filepath.Join(t.TempDir(), "en-blog")
	for _, d := range []string{zhDir, enDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	write := func(dir, slug, cat, title string) {
		body := "---\n" +
			"title: \"" + title + "\"\n" +
			"date: 2026-01-02\n" +
			"author: \"tester\"\n" +
			"category: \"" + cat + "\"\n" +
			"tags: [a, b]\n" +
			"---\n\nBody of " + slug + "\n"
		if err := os.WriteFile(filepath.Join(dir, slug+".md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	for _, p := range zhFixtures {
		write(zhDir, p.slug, p.cat, "ZH "+p.slug)
	}
	for _, p := range enFixtures {
		write(enDir, p.slug, p.cat, "EN "+p.slug)
	}
	for i := 0; i < bulkZH; i++ {
		write(zhDir, fmt.Sprintf("sm-bulk-zh-%03d", i), "品牌推荐", fmt.Sprintf("ZH bulk %d", i))
	}
	for i := 0; i < bulkEN; i++ {
		write(enDir, fmt.Sprintf("sm-bulk-en-%03d", i), "Beginner Guide", fmt.Sprintf("EN bulk %d", i))
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
		AppEnv:           "test",
		SecretKey:        testSecret,
		JWTAccessMin:     30,
		JWTRefreshDays:   7,
		CORSOrigins:      []string{"http://localhost:5173"},
		StorageBackend:   "local",
		UploadDir:        filepath.Join(t.TempDir(), "uploads"),
		FrontendDist:     t.TempDir(),
		PublicURL:        "https://brickplans.test",
		BlogContentDir:   zhDir,
		BlogContentDirEN: enDir,
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

func getHTML(t *testing.T, r *gin.Engine, path string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// ── Category mapping semantics ──

// TestBucketForMapsLegacyCategories pins the mapping: legacy Chinese
// near-duplicates AND the English categories M2 will author must land in the
// documented buckets, and anything unknown must degrade to the catch-all rather
// than vanish.
func TestBucketForMapsLegacyCategories(t *testing.T) {
	cases := map[string]string{
		"品牌推荐":           "brand",
		"品牌评测":           "brand",
		"选购指南":           "brand",
		"Brand Guide":    "brand",
		"Buying Guide":   "brand",
		"积木教程":           "beginner",
		"新手入门":           "beginner",
		"Beginner Guide": "beginner",
		"零件知识":           "parts",
		"Parts Guide":    "parts",
		"搭建技巧":           "building",
		"MOC搭建":          "building",
		"积木评测":           "set-review",
		"评测":             "set-review",
		"Set Review":     "set-review",
		"MOC图纸评测":        "moc-review",
		"MOC评测":          "moc-review",
		"MOC Review":     "moc-review",
		"MOC教程":          "moc-tutorial",
		"MOC 教程":         "moc-tutorial",
		"MOC Tutorial":   "moc-tutorial",
		"知识科普":           "knowledge",
		"资源推荐":           "resources",
		"":               blog.OtherBucketSlug,
		"没有这个分类":         blog.OtherBucketSlug,
		" 品牌推荐":          blog.OtherBucketSlug, // whitespace is NOT trimmed: be explicit, not clever
	}
	for raw, want := range cases {
		if got := blog.BucketFor(raw); got != want {
			t.Errorf("BucketFor(%q) = %q, want %q", raw, got, want)
		}
	}
}

// ── Sitemap structure ──

// TestSitemapIndexListsEveryChildSitemap checks the index is complete (both
// languages, all zh buckets, only non-empty en buckets) and every child it
// advertises answers with a parseable urlset.
func TestSitemapIndexListsEveryChildSitemap(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)

	code, body := getSitemap(t, r, "/sitemap.xml")
	if code != 200 {
		t.Fatalf("/sitemap.xml: %d", code)
	}
	var index xmlSitemapIndex
	if err := xml.Unmarshal([]byte(body), &index); err != nil {
		t.Fatalf("sitemap index is not well-formed XML: %v", err)
	}

	wantPaths := []string{"/sitemap-zh.xml", "/sitemap-en.xml"}
	for _, slug := range blog.BucketSlugs() {
		wantPaths = append(wantPaths, "/sitemap-zh-blog-"+slug+".xml")
	}
	// Only the buckets that actually hold English posts.
	enBuckets := map[string]bool{}
	for _, p := range enFixtures {
		enBuckets[fixtureBucket(p)] = true
	}
	for bucket := range enBuckets {
		wantPaths = append(wantPaths, "/sitemap-en-blog-"+bucket+".xml")
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
	// An empty bucket sitemap is not advertised: Search Console reports it as
	// "0 pages discovered", which reads like a bug rather than "not translated".
	// parts has an English post, moc-review does not: the empty one must not be
	// advertised, the filled one must be.
	if strings.Contains(body, "/sitemap-en-blog-moc-review.xml") {
		t.Error("index advertises an empty English bucket")
	}
	if !strings.Contains(body, "/sitemap-en-blog-parts.xml") {
		t.Error("index omits the English bucket that has posts")
	}

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
// appears in exactly one child sitemap, in the child of its own language and
// bucket, and no URL is duplicated across children. A post missing from every
// sitemap is an invisible de-indexing risk (and the English tree has no
// historical baseline, so an en-side regression would otherwise go unnoticed).
func TestSitemapPartitionIsTotal(t *testing.T) {
	const bulkZH, bulkEN = 12, 4
	r, _ := setupSitemapTest(t, bulkZH, bulkEN)

	langs := []string{blog.LangZH, blog.LangEN}
	seen := map[string]string{} // loc -> first child that carried it

	for _, lang := range langs {
		for _, slug := range blog.BucketSlugs() {
			path := "/sitemap-" + lang + "-blog-" + slug + ".xml"
			code, body := getSitemap(t, r, path)
			if code != 200 {
				t.Fatalf("%s: %d", path, code)
			}
			set := parseURLSet(t, path, body)

			want := 0
			fixtures := zhFixtures
			prefix := "https://brickplans.test/blog/"
			if lang == blog.LangEN {
				fixtures = enFixtures
				prefix = "https://brickplans.test/en/blog/"
			}
			for _, p := range fixtures {
				if fixtureBucket(p) == slug {
					want++
				}
			}
			if lang == blog.LangZH && slug == "brand" {
				want += bulkZH
			}
			if lang == blog.LangEN && slug == "beginner" {
				want += bulkEN
			}
			if len(set.URLs) != want {
				t.Errorf("bucket %s/%s has %d urls, want %d", lang, slug, len(set.URLs), want)
			}
			for _, u := range set.URLs {
				if !strings.HasPrefix(u.Loc, prefix) {
					t.Errorf("bucket %s/%s emitted %q, want prefix %s", lang, slug, u.Loc, prefix)
				}
				if prev, dup := seen[u.Loc]; dup {
					t.Errorf("url %s listed in both %s and %s", u.Loc, prev, path)
				}
				seen[u.Loc] = path
			}
		}
	}

	for _, p := range zhFixtures {
		loc := "https://brickplans.test/blog/" + p.slug
		gotPath, ok := seen[loc]
		if !ok {
			t.Errorf("zh post %s is in NO child sitemap — it would be de-indexed", p.slug)
			continue
		}
		if want := "/sitemap-zh-blog-" + fixtureBucket(p) + ".xml"; gotPath != want {
			t.Errorf("zh post %s served from %s, want %s", p.slug, gotPath, want)
		}
	}
	for _, p := range enFixtures {
		loc := "https://brickplans.test/en/blog/" + p.slug
		gotPath, ok := seen[loc]
		if !ok {
			t.Errorf("en post %s is in NO child sitemap — it would be de-indexed", p.slug)
			continue
		}
		if want := "/sitemap-en-blog-" + fixtureBucket(p) + ".xml"; gotPath != want {
			t.Errorf("en post %s served from %s, want %s", p.slug, gotPath, want)
		}
	}
	if want := len(zhFixtures) + len(enFixtures) + bulkZH + bulkEN; len(seen) != want {
		t.Errorf("blog children carried %d urls, want %d", len(seen), want)
	}
}

// TestSitemapHreflangAlternates asserts the xhtml:link annotations are emitted
// for a translated pair and omitted for a one-sided article.
func TestSitemapHreflangAlternates(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)

	// Paired article (both trees) → three alternates on each side.
	for _, path := range []string{"/sitemap-zh-blog-parts.xml", "/sitemap-en-blog-parts.xml"} {
		code, body := getSitemap(t, r, path)
		if code != 200 {
			t.Fatalf("%s: %d", path, code)
		}
		set := parseURLSet(t, path, body)
		if len(set.URLs) != 1 {
			t.Fatalf("%s has %d urls, want 1", path, len(set.URLs))
		}
		links := set.URLs[0].Links
		if len(links) != 3 {
			t.Errorf("%s: got %d xhtml:link, want 3 (%+v)", path, len(links), links)
			continue
		}
		got := map[string]string{}
		for _, l := range links {
			if l.Rel != "alternate" {
				t.Errorf("%s: rel = %q, want alternate", path, l.Rel)
			}
			got[l.Hreflang] = l.Href
		}
		if got["zh-CN"] != "https://brickplans.test/blog/sm-parts" {
			t.Errorf("%s: zh-CN alternate = %q", path, got["zh-CN"])
		}
		if got["en"] != "https://brickplans.test/en/blog/sm-parts" {
			t.Errorf("%s: en alternate = %q", path, got["en"])
		}
		if got["x-default"] != "https://brickplans.test/en/blog/sm-parts" {
			t.Errorf("%s: x-default = %q (design: x-default points at the English page)", path, got["x-default"])
		}
	}

	// Untranslated article → no alternates at all (a one-sided hreflang makes
	// Google drop the annotation for the whole cluster).
	code, body := getSitemap(t, r, "/sitemap-zh-blog-brand.xml")
	if code != 200 {
		t.Fatalf("brand child: %d", code)
	}
	set := parseURLSet(t, "/sitemap-zh-blog-brand.xml", body)
	if len(set.URLs) == 0 {
		t.Fatal("brand child is empty")
	}
	for _, u := range set.URLs {
		if len(u.Links) != 0 {
			t.Errorf("%s has %d hreflang links but has no translation", u.Loc, len(u.Links))
		}
	}
}

// TestSitemapSiteHubHoldsBlueprintsAndPagination checks the per-language hubs
// carry the non-article resources for their tree.
func TestSitemapSiteHubHoldsBlueprintsAndPagination(t *testing.T) {
	// 7 zh fixtures + 50 bulk = 57 zh posts → 2 zh listing pages.
	// 2 en fixtures + 50 bulk = 52 en posts → 2 en listing pages.
	r, gdb := setupSitemapTest(t, 50, 50)
	if err := gdb.Create(&db.Tag{Name: "积木"}).Error; err != nil {
		t.Fatalf("seed tag: %v", err)
	}
	tok, _ := registerUser(t, r, "smhub", "smhub@x.com")
	pubID := createBlueprint(t, r, tok, "已发布作品", true)
	draftID := createBlueprint(t, r, tok, "未发布草稿", false)

	code, body := getSitemap(t, r, "/sitemap-zh.xml")
	if code != 200 {
		t.Fatalf("/sitemap-zh.xml: %d", code)
	}
	locs := map[string]bool{}
	for _, u := range parseURLSet(t, "/sitemap-zh.xml", body).URLs {
		locs[u.Loc] = true
	}
	for _, want := range []string{
		"https://brickplans.test/",
		"https://brickplans.test/explore",
		"https://brickplans.test/blog",
		"https://brickplans.test/blog?page=2",
		"https://brickplans.test/tags/" + url.PathEscape("积木"),
		"https://brickplans.test/detail/" + pubID,
	} {
		if !locs[want] {
			t.Errorf("zh hub missing %s", want)
		}
	}
	if locs["https://brickplans.test/blog?page=3"] {
		t.Error("zh hub advertised a pagination page past the end")
	}
	if strings.Contains(body, draftID) {
		t.Error("unpublished blueprint leaked into the zh hub")
	}
	// Articles never live in the hub — that is what keeps the sitemap partition
	// duplicate-free.
	if locs["https://brickplans.test/blog/sm-parts"] {
		t.Error("zh hub lists an article; articles belong to the bucket sitemaps")
	}

	code, body = getSitemap(t, r, "/sitemap-en.xml")
	if code != 200 {
		t.Fatalf("/sitemap-en.xml: %d", code)
	}
	locs = map[string]bool{}
	for _, u := range parseURLSet(t, "/sitemap-en.xml", body).URLs {
		locs[u.Loc] = true
	}
	for _, want := range []string{
		"https://brickplans.test/en/blog",
		"https://brickplans.test/en/blog?page=2",
	} {
		if !locs[want] {
			t.Errorf("en hub missing %s", want)
		}
	}
	if locs["https://brickplans.test/blog"] {
		t.Error("en hub leaked a Chinese URL")
	}
}

// TestSitemapUnknownBlogBucket404s ensures a bucket path that is not in the
// table cannot be served (a soft-404 HTML page here would be parsed as a broken
// sitemap by the crawler).
func TestSitemapUnknownBlogBucket404s(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)
	for _, path := range []string{
		"/sitemap-zh-blog-not-a-bucket.xml",
		"/sitemap-en-blog-not-a-bucket.xml",
		"/sitemap-xx-blog-parts.xml",
	} {
		code, body := getSitemap(t, r, path)
		if code != 404 {
			t.Errorf("%s: HTTP %d, want 404", path, code)
		}
		if strings.Contains(body, "<urlset") {
			t.Errorf("%s returned a urlset", path)
		}
	}
}

// ── Multilingual SSR / API behaviour ──

// TestEnglishBlogDetailHreflangPair asserts the translated article page: English
// head, self-canonical /en/ URL, bidirectional hreflang, English inLanguage.
func TestEnglishBlogDetailHreflangPair(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)

	code, body := getHTML(t, r, "/en/blog/sm-parts")
	if code != 200 {
		t.Fatalf("/en/blog/sm-parts: %d", code)
	}
	for _, want := range []string{
		`<html lang="en">`,
		`<link rel="canonical" href="https://brickplans.test/en/blog/sm-parts" />`,
		`hreflang="en" href="https://brickplans.test/en/blog/sm-parts"`,
		`hreflang="zh-CN" href="https://brickplans.test/blog/sm-parts"`,
		`hreflang="x-default" href="https://brickplans.test/en/blog/sm-parts"`,
		`"inLanguage":"en"`,
		"EN sm-parts", // the English title, not the Chinese one
	} {
		if !strings.Contains(body, want) {
			t.Errorf("en detail missing %q", want)
		}
	}
	if strings.Contains(body, "ZH sm-parts") {
		t.Error("en detail rendered the Chinese title")
	}
	// An English body must not send crawlers back into the Chinese tree.
	if strings.Contains(body, `href="https://brickplans.test/blog/`) &&
		!strings.Contains(body, `href="https://brickplans.test/blog/sm-parts"`) {
		t.Error("en body contains an unexpected /blog/ link")
	}

	// The Chinese page must declare the mirror pair.
	code, body = getHTML(t, r, "/blog/sm-parts")
	if code != 200 {
		t.Fatalf("/blog/sm-parts: %d", code)
	}
	for _, want := range []string{
		`<html lang="zh-CN">`,
		`<link rel="canonical" href="https://brickplans.test/blog/sm-parts" />`,
		`hreflang="en" href="https://brickplans.test/en/blog/sm-parts"`,
		`hreflang="zh-CN" href="https://brickplans.test/blog/sm-parts"`,
		`"inLanguage":"zh-CN"`,
		"ZH sm-parts",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("zh detail missing %q", want)
		}
	}
	// The bogus zh-TW/zh-Hans/zh-Hant annotations that pointed at the Chinese
	// page used to be hardcoded in the template; they must be gone.
	if strings.Contains(body, `hreflang="zh-TW"`) || strings.Contains(body, `hreflang="zh-Hans"`) {
		t.Error("stale zh-TW/zh-Hans hreflang still emitted")
	}
}

// TestEnglishFallbackServesChineseCanonical covers the missing-translation case:
// /en/blog/<slug> must render (no 404 / orphan link) but must not claim to be
// English or advertise an English alternate.
func TestEnglishFallbackServesChineseCanonical(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)

	code, body := getHTML(t, r, "/en/blog/sm-brand-canonical")
	if code != 200 {
		t.Fatalf("fallback: HTTP %d, want 200", code)
	}
	if !strings.Contains(body, "ZH sm-brand-canonical") {
		t.Error("fallback page did not render the Chinese article")
	}
	if !strings.Contains(body, `<link rel="canonical" href="https://brickplans.test/blog/sm-brand-canonical" />`) {
		t.Error("fallback must keep the Chinese canonical so the two URLs consolidate")
	}
	if strings.Contains(body, `hreflang="en" href="https://brickplans.test/en/blog/sm-brand-canonical"`) {
		t.Error("fallback must not claim an English version exists")
	}
	if !strings.Contains(body, `<html lang="zh-CN">`) {
		t.Error("fallback page should declare the language it actually renders")
	}

	// A slug in neither tree is still a real 404.
	if code, _ := getHTML(t, r, "/en/blog/does-not-exist-anywhere"); code != 404 {
		t.Errorf("unknown slug: HTTP %d, want 404", code)
	}
}

// TestEnglishBlogListIsCrawlable checks the English listing page: English
// chrome, /en/ links, hreflang pair, and pagination.
func TestEnglishBlogListIsCrawlable(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 50)

	code, body := getHTML(t, r, "/en/blog")
	if code != 200 {
		t.Fatalf("/en/blog: %d", code)
	}
	for _, want := range []string{
		"<h1>BrickPlan Blog</h1>",
		`href="https://brickplans.test/en/blog/sm-bulk-en-000"`,
		`<link rel="canonical" href="https://brickplans.test/en/blog" />`,
		`hreflang="zh-CN" href="https://brickplans.test/blog"`,
		`/en/blog?page=2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/en/blog missing %q", want)
		}
	}
	// 52 English posts at 50 per page: the two fixture posts sort last, so they
	// must be reachable on page 2 — that is the crawl chain working.
	code, page2 := getHTML(t, r, "/en/blog?page=2")
	if code != 200 {
		t.Fatalf("/en/blog?page=2: %d", code)
	}
	for _, want := range []string{
		`href="https://brickplans.test/en/blog/sm-en-only"`,
		`href="https://brickplans.test/en/blog/sm-parts"`,
		`<link rel="canonical" href="https://brickplans.test/en/blog?page=2" />`,
	} {
		if !strings.Contains(page2, want) {
			t.Errorf("/en/blog?page=2 missing %q", want)
		}
	}
	if strings.Contains(body, "BrickPlan 博客") {
		t.Error("/en/blog rendered the Chinese H1")
	}
}

// TestBlogAPILangSelector asserts ?lang=en switches the tree and stays
// backward-compatible when the parameter is absent.
func TestBlogAPILangSelector(t *testing.T) {
	r, _ := setupSitemapTest(t, 0, 0)

	req := httptest.NewRequest("GET", "/api/blog?lang=en&page_size=100", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("api lang=en: %d", w.Code)
	}
	var en struct {
		Lang  string `json:"lang"`
		Total int    `json:"total"`
		Items []struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &en); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if en.Lang != "en" {
		t.Errorf("lang = %q, want en", en.Lang)
	}
	if en.Total != len(enFixtures) {
		t.Errorf("en total = %d, want %d", en.Total, len(enFixtures))
	}
	for _, it := range en.Items {
		if !strings.HasPrefix(it.Title, "EN ") {
			t.Errorf("en listing leaked a zh post: %+v", it)
		}
	}

	// No ?lang → the Chinese tree, exactly as before the rollout.
	req = httptest.NewRequest("GET", "/api/blog?page_size=100", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var zh struct {
		Lang  string `json:"lang"`
		Total int    `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &zh); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if zh.Lang != "zh" || zh.Total != len(zhFixtures) {
		t.Errorf("default listing = lang %q total %d, want zh/%d", zh.Lang, zh.Total, len(zhFixtures))
	}

	// Detail: the English translation wins under ?lang=en, and its neighbours
	// stay in the English tree.
	req = httptest.NewRequest("GET", "/api/blog/sm-parts?lang=en", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("api detail lang=en: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "EN sm-parts") {
		t.Error("api detail lang=en served the Chinese body")
	}
}
