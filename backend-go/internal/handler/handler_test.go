package handler_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/router"
	"brickplans/internal/ssr"
	"brickplans/internal/storage"
)

const testSecret = "test-secret-key-must-be-at-least-32-characters-long"

func setupTest(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
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
		FrontendDist:   t.TempDir(), // no vite manifest → SSR HTML omits SPA script (fine for API tests)
		PublicURL:      "https://brickplans.test",
	}
	storage.Reset()
	renderer := ssr.NewRenderer(cfg.FrontendDist, cfg.PublicURL)
	// Close the DB connection so Windows can remove the temp dir on cleanup.
	sqlDB, _ := gdb.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	return router.New(cfg, gdb, renderer), gdb
}

// ── HTTP helpers ──

func doJSON(t *testing.T, r http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("parse json: %v (body=%s)", err, w.Body.String())
	}
	return m
}

func extractCookie(w *httptest.ResponseRecorder, name string) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func registerUser(t *testing.T, r http.Handler, username, email string) (token, userID string) {
	w := doJSON(t, r, "POST", "/api/auth/register",
		map[string]string{"username": username, "email": email, "password": "password123"}, "")
	if w.Code != 201 {
		t.Fatalf("register %s: %d %s", email, w.Code, w.Body.String())
	}
	m := parseJSON(t, w)
	token = m["access_token"].(string)
	userID = m["user"].(map[string]any)["id"].(string)
	return
}

func createBlueprint(t *testing.T, r http.Handler, token, title string, published bool) string {
	body := map[string]any{"title": title}
	if !published {
		body["is_published"] = false
	}
	w := doJSON(t, r, "POST", "/api/blueprints", body, token)
	if w.Code != 201 {
		t.Fatalf("create bp: %d %s", w.Code, w.Body.String())
	}
	return parseJSON(t, w)["id"].(string)
}

// ── Tests ──

func TestRegisterLoginMe(t *testing.T) {
	r, _ := setupTest(t)
	tok, uid := registerUser(t, r, "alice", "alice@example.com")
	if tok == "" || uid == "" {
		t.Fatal("missing token/id")
	}
	// /me returns email + is_admin (own projection).
	w := doJSON(t, r, "GET", "/api/auth/me", nil, tok)
	if w.Code != 200 {
		t.Fatalf("me: %d", w.Code)
	}
	me := parseJSON(t, w)
	if me["email"] != "alice@example.com" {
		t.Fatalf("email = %v", me["email"])
	}
	if me["is_admin"] != false {
		t.Fatalf("is_admin = %v", me["is_admin"])
	}
}

func TestPasswordPolicy(t *testing.T) {
	r, _ := setupTest(t)
	cases := []struct{ pw string }{
		{"123"},        // too short
		{"allletters"}, // no digit
		{"12345678"},   // no letter
	}
	for _, c := range cases {
		w := doJSON(t, r, "POST", "/api/auth/register",
			map[string]string{"username": "u" + c.pw, "email": c.pw + "@x.com", "password": c.pw}, "")
		if w.Code != 400 {
			t.Fatalf("password %q expected 400, got %d: %s", c.pw, w.Code, w.Body.String())
		}
	}
	// Valid password succeeds.
	w := doJSON(t, r, "POST", "/api/auth/register",
		map[string]string{"username": "validuser", "email": "valid@x.com", "password": "password123"}, "")
	if w.Code != 201 {
		t.Fatalf("valid password: %d %s", w.Code, w.Body.String())
	}
}

func TestTokenRevocationOnChangePassword(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "bob", "bob@example.com")
	// Old token works.
	if w := doJSON(t, r, "GET", "/api/auth/me", nil, tok); w.Code != 200 {
		t.Fatalf("pre-change me: %d", w.Code)
	}
	// Change password.
	w := doJSON(t, r, "PUT", "/api/auth/password",
		map[string]string{"current_password": "password123", "new_password": "newpass456"}, tok)
	if w.Code != 200 {
		t.Fatalf("change password: %d %s", w.Code, w.Body.String())
	}
	// Old token is now revoked.
	if w := doJSON(t, r, "GET", "/api/auth/me", nil, tok); w.Code != 401 {
		t.Fatalf("post-change old token expected 401, got %d", w.Code)
	}
	// New password works.
	w = doJSON(t, r, "POST", "/api/auth/login",
		map[string]string{"email": "bob@example.com", "password": "newpass456"}, "")
	if w.Code != 200 {
		t.Fatalf("login new password: %d", w.Code)
	}
}

func TestRefreshEndpointWorks(t *testing.T) {
	r, _ := setupTest(t)
	// Register sets the bp_refresh httpOnly cookie in the response.
	w := doJSON(t, r, "POST", "/api/auth/register",
		map[string]string{"username": "carol", "email": "carol@example.com", "password": "password123"}, "")
	refreshCookie := extractCookie(w, "bp_refresh")
	if refreshCookie == "" {
		t.Fatal("register did not set bp_refresh cookie")
	}
	// Refresh reads the cookie (no body). Regression: Python version crashed with NameError here.
	req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "bp_refresh", Value: refreshCookie})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("refresh: %d %s", rr.Code, rr.Body.String())
	}
	newTok := parseJSON(t, rr)["access_token"].(string)
	if w := doJSON(t, r, "GET", "/api/auth/me", nil, newTok); w.Code != 200 {
		t.Fatalf("me with refreshed token: %d", w.Code)
	}
}

func TestUnpublishedBlueprintNotVisibleToOthers(t *testing.T) {
	r, _ := setupTest(t)
	tokA, uidA := registerUser(t, r, "alice2", "a2@x.com")
	tokB, _ := registerUser(t, r, "bob2", "b2@x.com")
	bpID := createBlueprint(t, r, tokA, "secret bp", false)

	// Author sees it.
	if w := doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, tokA); w.Code != 200 {
		t.Fatalf("author detail: %d", w.Code)
	}
	// Other user gets 404.
	if w := doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, tokB); w.Code != 404 {
		t.Fatalf("other user detail expected 404, got %d", w.Code)
	}
	// Anonymous gets 404.
	if w := doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, ""); w.Code != 404 {
		t.Fatalf("anon detail expected 404, got %d", w.Code)
	}
	// Author's own listing includes it.
	w := doJSON(t, r, "GET", "/api/users/"+uidA+"/blueprints", nil, tokA)
	items := parseJSON(t, w)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("author listing expected 1, got %d", len(items))
	}
	// Other user's listing excludes unpublished.
	w = doJSON(t, r, "GET", "/api/users/"+uidA+"/blueprints", nil, tokB)
	items = parseJSON(t, w)["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("other user listing expected 0, got %d", len(items))
	}
}

func TestFavoritesArePrivate(t *testing.T) {
	r, _ := setupTest(t)
	tokA, _ := registerUser(t, r, "favA", "fava@x.com")
	tokB, uidB := registerUser(t, r, "favB", "favb@x.com")
	bpID := createBlueprint(t, r, tokA, "published bp", true)

	// B favorites A's blueprint.
	if w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/favorite", nil, tokB); w.Code != 201 {
		t.Fatalf("favorite: %d %s", w.Code, w.Body.String())
	}
	// B sees own favorites.
	if w := doJSON(t, r, "GET", "/api/users/"+uidB+"/favorites", nil, tokB); w.Code != 200 {
		t.Fatalf("own favorites: %d", w.Code)
	}
	// A cannot see B's favorites (privacy fix).
	if w := doJSON(t, r, "GET", "/api/users/"+uidB+"/favorites", nil, tokA); w.Code != 403 {
		t.Fatalf("other favorites expected 403, got %d", w.Code)
	}
	// Anonymous cannot either.
	if w := doJSON(t, r, "GET", "/api/users/"+uidB+"/favorites", nil, ""); w.Code != 403 {
		t.Fatalf("anon favorites expected 403, got %d", w.Code)
	}
}

func TestReportDoesNotAutoUnpublish(t *testing.T) {
	r, gdb := setupTest(t)
	tokA, _ := registerUser(t, r, "repA", "repa@x.com")
	bpID := createBlueprint(t, r, tokA, "reported bp", true)
	// Three different users report it.
	for i, suffix := range []string{"x", "y", "z"} {
		tok, _ := registerUser(t, r, "rep"+suffix, "rep"+suffix+"@x.com")
		if w := doJSON(t, r, "POST", "/api/reports",
			map[string]string{"blueprint_id": bpID, "reason": "spam"}, tok); w.Code != 201 {
			t.Fatalf("report %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	// Blueprint must still be published (no auto-unpublish).
	var bp db.Blueprint
	gdb.First(&bp, "id = ?", bpID)
	if !bp.IsPublished {
		t.Fatal("blueprint was auto-unpublished by 3 reports (griefing vulnerability)")
	}
}

func TestAvatarUploadRejectsHtml(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "ava1", "ava1@x.com")
	// Upload an HTML file disguised as image/jpeg. Magic bytes won't match → 422.
	w := uploadAvatarRequest(t, r, tok, "evil.html", "image/jpeg", []byte("<script>alert(1)</script>"))
	if w.Code != 422 {
		t.Fatalf("html avatar expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAvatarPngStoredAsJpg(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "ava2", "ava2@x.com")
	pngBytes := minPNG(t)
	w := uploadAvatarRequest(t, r, tok, "photo.png", "image/png", pngBytes)
	if w.Code != 200 {
		t.Fatalf("png avatar expected 200, got %d: %s", w.Code, w.Body.String())
	}
	url := parseJSON(t, w)["avatar_url"].(string)
	if !strings.HasSuffix(url, ".jpg") {
		t.Fatalf("avatar url should end with .jpg, got %q", url)
	}
}

func TestPublicProfileHasNoEmail(t *testing.T) {
	r, _ := setupTest(t)
	_, uidA := registerUser(t, r, "pub1", "pub1@x.com")
	// Public profile must not expose email or is_admin.
	w := doJSON(t, r, "GET", "/api/users/"+uidA, nil, "")
	if w.Code != 200 {
		t.Fatalf("profile: %d", w.Code)
	}
	m := parseJSON(t, w)
	if _, ok := m["email"]; ok {
		t.Fatal("public profile leaks email")
	}
	if _, ok := m["is_admin"]; ok {
		t.Fatal("public profile leaks is_admin")
	}
}

func TestCommentSerializationHasNoEmail(t *testing.T) {
	r, _ := setupTest(t)
	tokA, _ := registerUser(t, r, "cmtA", "cmtA@x.com")
	bpID := createBlueprint(t, r, tokA, "cmt bp", true)
	// A comments on own blueprint.
	w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/comments",
		map[string]string{"content": "nice"}, tokA)
	if w.Code != 201 {
		t.Fatalf("comment: %d %s", w.Code, w.Body.String())
	}
	// List comments (public) — user object must not contain email.
	w = doJSON(t, r, "GET", "/api/blueprints/"+bpID+"/comments", nil, "")
	var arr []any
	json.Unmarshal(w.Body.Bytes(), &arr)
	if len(arr) == 0 {
		t.Fatal("no comments")
	}
	user := arr[0].(map[string]any)["user"].(map[string]any)
	if _, ok := user["email"]; ok {
		t.Fatal("comment user leaks email")
	}
}

// ── upload helpers ──

func uploadAvatarRequest(t *testing.T, r http.Handler, token, filename, contentType string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("_", "_") // ensure non-empty
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	// Override Content-Type for the file part — CreateFormFile sets application/octet-stream;
	// gin's FormFile preserves fh.ContentType which comes from the multipart header. To mimic a
	// client lie, we rebuild the part header manually is complex; the server ignores Content-Type
	// anyway (it trusts magic bytes), so this is sufficient for the test.
	writer.Close()
	req := httptest.NewRequest("POST", "/api/auth/avatar", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func minPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSSRDetailHTML(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "ssruser", "ssr@x.com")
	bpID := createBlueprint(t, r, tok, "测试图纸SSR", true)

	req := httptest.NewRequest("GET", "/detail/"+bpID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("SSR detail: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"测试图纸SSR", "application/ld+json", "CreativeWork", "<article", "og:image"} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSR HTML missing %q\nbody: %s", want, body)
		}
	}
}

// TestBrandAndProductCodeRoundTrip covers the 品牌 / 编号(型号) attributes added to
// blueprint attributes: they must survive create → read → update → clear, appear in
// the public API payload, and reach the SSR noscript + Product JSON-LD.
func TestBrandAndProductCodeRoundTrip(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "branduser", "brand@x.com")

	// Create with both attributes set.
	w := doJSON(t, r, "POST", "/api/blueprints", map[string]any{
		"title":        "品牌型号测试图纸",
		"brand":        "乐高",
		"product_code": "42115",
	}, tok)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	bpID := parseJSON(t, w)["id"].(string)

	got := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, ""))
	if got["brand"] != "乐高" || got["product_code"] != "42115" {
		t.Fatalf("brand/product_code not persisted: %v / %v", got["brand"], got["product_code"])
	}

	// Update replaces both values.
	w = doJSON(t, r, "PUT", "/api/blueprints/"+bpID, map[string]any{
		"brand":        "森宝",
		"product_code": " 10236 ",
	}, tok)
	if w.Code != 200 {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	got = parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, ""))
	if got["brand"] != "森宝" {
		t.Fatalf("brand not updated: %v", got["brand"])
	}
	if got["product_code"] != "10236" {
		t.Fatalf("product_code not trimmed: %q", got["product_code"])
	}

	// SSR must expose the attributes and the CreativeWork identifier for SEO
	// （G-2：主实体不再是 Product，mpn 是 Product 专属字段，已换成 identifier）。
	req := httptest.NewRequest("GET", "/detail/"+bpID, nil)
	sw := httptest.NewRecorder()
	r.ServeHTTP(sw, req)
	body := sw.Body.String()
	for _, want := range []string{"品牌：森宝", "编号：10236", `"identifier":"10236"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSR body missing %q", want)
		}
	}

	// Over-long brand is rejected.
	if w := doJSON(t, r, "PUT", "/api/blueprints/"+bpID, map[string]any{"brand": strings.Repeat("品", 31)}, tok); w.Code != 400 {
		t.Fatalf("expected 400 for over-long brand, got %d", w.Code)
	}

	// Blank string clears the field (stored as NULL, not "").
	if w := doJSON(t, r, "PUT", "/api/blueprints/"+bpID, map[string]any{"brand": "  ", "product_code": ""}, tok); w.Code != 200 {
		t.Fatalf("clear update: %d %s", w.Code, w.Body.String())
	}
	got = parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, ""))
	if got["brand"] != nil || got["product_code"] != nil {
		t.Fatalf("blank input should clear the fields, got %v / %v", got["brand"], got["product_code"])
	}
}

// TestListFiltersByBrandAndProductCode covers the brand / product-code lookup on
// the public list endpoint: AND-combined filters, LEGO -> 乐高 alias, case-insensitive
// code match, multiple hits per code, and unpublished rows staying hidden.
func TestListFiltersByBrandAndProductCode(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "filteruser", "filter@x.com")

	mk := func(title, brand, code string, published bool) string {
		body := map[string]any{"title": title}
		if brand != "" {
			body["brand"] = brand
		}
		if code != "" {
			body["product_code"] = code
		}
		if !published {
			body["is_published"] = false
		}
		w := doJSON(t, r, "POST", "/api/blueprints", body, tok)
		if w.Code != 201 {
			t.Fatalf("create %s: %d %s", title, w.Code, w.Body.String())
		}
		return parseJSON(t, w)["id"].(string)
	}

	// Two authors' uploads of the same set: a code lookup must return both.
	a1 := mk("乐高款A (LEGO 76344)", "乐高", "76344", true)
	a2 := mk("乐高款A副本 (LEGO 76344)", "乐高", "76344", true)
	mk("奇积款B (奇积 23025)", "奇积", "23025", true)
	mk("乐高未发布款", "乐高", "88888", false)

	total := func(qs string) (int, []any) {
		m := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints?"+qs, nil, ""))
		items, _ := m["items"].([]any)
		return int(m["total"].(float64)), items
	}

	// brand only (Chinese + LEGO alias resolve to the same rows)
	lego := url.QueryEscape("乐高")
	if n, _ := total("brand=" + lego); n != 2 {
		t.Fatalf("brand=乐高 total = %d, want 2", n)
	}
	if n, _ := total("brand=LEGO"); n != 2 {
		t.Fatalf("brand=LEGO alias total = %d, want 2", n)
	}

	// brand + code -> both uploads of that set, with the attributes exposed
	n, items := total("brand=" + lego + "&product_code=76344")
	if n != 2 || len(items) != 2 {
		t.Fatalf("brand+code total = %d (items %d), want 2", n, len(items))
	}
	first, _ := items[0].(map[string]any)
	if first["brand"] != "乐高" || first["product_code"] != "76344" {
		t.Fatalf("published payload missing brand/product_code: %v", first)
	}
	ids := map[string]bool{}
	for _, it := range items {
		m, _ := it.(map[string]any)
		ids[m["id"].(string)] = true
	}
	if !ids[a1] || !ids[a2] {
		t.Fatalf("both uploads of the same code must be returned, got %v", ids)
	}

	// code only, lowercase -> case-insensitive single hit
	if n, _ := total("product_code=23025"); n != 1 {
		t.Fatalf("code lookup total = %d, want 1", n)
	}
	if n, _ := total("product_code=-1"); n != 0 {
		t.Fatalf("unknown code total = %d, want 0", n)
	}

	// mismatched brand+code pair returns nothing, and unpublished rows stay hidden
	qiji := url.QueryEscape("奇积")
	if n, _ := total("brand=" + qiji + "&product_code=76344"); n != 0 {
		t.Fatalf("mismatched pair total = %d, want 0", n)
	}
	if n, _ := total("product_code=88888"); n != 0 {
		t.Fatalf("unpublished code total = %d, want 0", n)
	}

	// free-text search also matches the code
	if n, _ := total("q=76344"); n != 2 {
		t.Fatalf("q=76344 total = %d, want 2", n)
	}
}

// TestLookupRequiresBothAndMatchesExactly covers GET /api/blueprints/lookup: both
// parameters are mandatory (400 otherwise), matching is strict equality on brand and
// product code (no prefix / fuzzy hits), several uploads of one set are all returned,
// and unpublished rows stay hidden.
func TestLookupRequiresBothAndMatchesExactly(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "lookupuser", "lookup@x.com")

	mk := func(title, brand, code string, published bool) string {
		body := map[string]any{"title": title}
		if brand != "" {
			body["brand"] = brand
		}
		if code != "" {
			body["product_code"] = code
		}
		if !published {
			body["is_published"] = false
		}
		w := doJSON(t, r, "POST", "/api/blueprints", body, tok)
		if w.Code != 201 {
			t.Fatalf("create %s: %d %s", title, w.Code, w.Body.String())
		}
		// product_code must be stored upper-cased (write-side normalisation).
		got := parseJSON(t, w)
		if code != "" {
			if want := strings.ToUpper(code); got["product_code"] != want {
				t.Fatalf("product_code stored as %v, want %q", got["product_code"], want)
			}
		}
		return got["id"].(string)
	}

	a1 := mk("乐高款A (LEGO 76344)", "乐高", "76344", true)
	a2 := mk("乐高款A副本 (LEGO 76344)", "乐高", "76344", true)
	mk("前缀诱饵 (LEGO 7634)", "乐高", "7634", true) // must NOT match 76344
	mk("奇积款B", "奇积", "23025", true)
	mk("乐高未发布款", "乐高", "76344", false) // same pair but unpublished
	mk("小写编码款", "小米", "abc12", true)   // stored as ABC12

	lookup := func(brand, code string) (int, int, string) {
		qs := ""
		if brand != "" {
			qs += "brand=" + url.QueryEscape(brand)
		}
		if code != "" {
			if qs != "" {
				qs += "&"
			}
			qs += "product_code=" + url.QueryEscape(code)
		}
		w := doJSON(t, r, "GET", "/api/blueprints/lookup?"+qs, nil, "")
		if w.Code != 200 {
			if w.Code == 400 {
				return -1, -1, parseJSON(t, w)["detail"].(string)
			}
			t.Fatalf("lookup %q: %d %s", qs, w.Code, w.Body.String())
		}
		m := parseJSON(t, w)
		items, _ := m["items"].([]any)
		return int(m["total"].(float64)), len(items), ""
	}

	// both parameters required
	for _, tc := range []struct{ brand, code string }{{"", ""}, {"乐高", ""}, {"", "76344"}} {
		n, _, detail := lookup(tc.brand, tc.code)
		if n != -1 {
			t.Fatalf("lookup(brand=%q, code=%q) should be 400, got total=%d", tc.brand, tc.code, n)
		}
		if detail == "" {
			t.Fatalf("400 response must carry a detail message")
		}
	}

	// exact hit -> every upload of that set (unpublished one excluded)
	n, items, _ := lookup("乐高", "76344")
	if n != 2 || items != 2 {
		t.Fatalf("lookup(乐高, 76344) total=%d items=%d, want 2/2", n, items)
	}
	w := doJSON(t, r, "GET", "/api/blueprints/lookup?brand="+url.QueryEscape("乐高")+"&product_code=76344", nil, "")
	found := map[string]bool{}
	for _, it := range parseJSON(t, w)["items"].([]any) {
		found[it.(map[string]any)["id"].(string)] = true
	}
	if !found[a1] || !found[a2] {
		t.Fatalf("all uploads of the set must be returned, got %v", found)
	}
	// LEGO alias resolves to the same row set
	if n, _, _ := lookup("LEGO", "76344"); n != 2 {
		t.Fatalf("lookup(LEGO, 76344) total=%d, want 2", n)
	}
	// strict: no prefix match (7634 must hit only its own row, never the 76344 ones;
	// "763" has no exact row at all), no fuzzy brand, no mismatched pair
	if n, _, _ := lookup("乐高", "7634"); n != 1 {
		t.Fatalf("lookup(乐高, 7634) must return only the exact 7634 row, total=%d", n)
	}
	if n, _, _ := lookup("乐高", "763"); n != 0 {
		t.Fatalf("prefix-only code must not match, total=%d", n)
	}
	if n, _, _ := lookup("乐", "76344"); n != 0 {
		t.Fatalf("partial brand must not match, total=%d", n)
	}
	if n, _, _ := lookup("奇积", "76344"); n != 0 {
		t.Fatalf("mismatched pair must not match, total=%d", n)
	}
	if n, _, _ := lookup("乐高", "76344 "); n != 2 {
		t.Fatalf("surrounding whitespace should be tolerated, total=%d", n)
	}
	// write-side upper-casing + case-insensitive compare
	if n, _, _ := lookup("小米", "abc12"); n != 1 {
		t.Fatalf("lowercase code input must match the stored uppercase code, total=%d", n)
	}
	if n, _, _ := lookup("小米", "ABC12"); n != 1 {
		t.Fatalf("uppercase code input must match, total=%d", n)
	}
}

// TestBackfillBrandsFromTitles covers the admin backfill endpoint: titles that end
// with "(<brand> <code>)" are parsed into brand / product_code, rows that already
// carry BOTH values are left alone, dry_run writes nothing, and the call is
// idempotent.
func TestBackfillBrandsFromTitles(t *testing.T) {
	r, gdb := setupTest(t)
	tok, userID := registerUser(t, r, "bfadmin", "bf@x.com")
	if err := gdb.Model(&db.User{}).Where("id = ?", userID).Update("is_admin", true).Error; err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	a := createBlueprint(t, r, tok, "钢铁侠 Mark 3 收藏版 (LEGO 76344)", true)
	b := createBlueprint(t, r, tok, "林间蘑菇 (LEGO 11505)", true)
	c := createBlueprint(t, r, tok, "纯中文标题没有编码", true)
	// b already carries both attributes -> must never be re-extracted.
	if w := doJSON(t, r, "PUT", "/api/blueprints/"+b, map[string]any{"brand": "乐高", "product_code": "11505"}, tok); w.Code != 200 {
		t.Fatalf("seed b: %d %s", w.Code, w.Body.String())
	}

	// Default (no body) is a dry run: preview only, nothing written.
	w := doJSON(t, r, "POST", "/api/admin/blueprints/backfill-brands", nil, tok)
	if w.Code != 200 {
		t.Fatalf("dry run: %d %s", w.Code, w.Body.String())
	}
	m := parseJSON(t, w)
	if m["dry_run"] != true {
		t.Fatalf("dry_run must default to true, got %v", m["dry_run"])
	}
	// scanned 3 rows: a parses, b is already complete, c has no code in the title.
	if m["scanned"].(float64) != 3 || m["parsed"].(float64) != 1 || m["changed"].(float64) != 1 {
		t.Fatalf("dry run counters: %v", m)
	}
	if m["already_set"].(float64) != 1 {
		t.Fatalf("row b must be skipped as already set, got already_set=%v", m["already_set"])
	}
	if got := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+a, nil, "")); got["brand"] != nil {
		t.Fatalf("dry run wrote data: %v", got["brand"])
	}

	// Real run writes only the parsed row and touches nothing else.
	w = doJSON(t, r, "POST", "/api/admin/blueprints/backfill-brands", map[string]any{"dry_run": false}, tok)
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	m = parseJSON(t, w)
	if m["changed"].(float64) != 1 || m["already_set"].(float64) != 1 {
		t.Fatalf("apply counters: %v", m)
	}
	got := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+a, nil, ""))
	if got["brand"] != "乐高" || got["product_code"] != "76344" {
		t.Fatalf("LEGO not normalised or code missing: %v / %v", got["brand"], got["product_code"])
	}
	if gotB := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+b, nil, "")); gotB["product_code"] != "11505" {
		t.Fatalf("already-set row changed: %v", gotB["product_code"])
	}
	if gotC := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+c, nil, "")); gotC["brand"] != nil || gotC["product_code"] != nil {
		t.Fatalf("unparseable title must stay empty: %v / %v", gotC["brand"], gotC["product_code"])
	}

	// Idempotent: a second pass finds nothing new to change.
	m = parseJSON(t, doJSON(t, r, "POST", "/api/admin/blueprints/backfill-brands", map[string]any{"dry_run": false}, tok))
	if m["changed"].(float64) != 0 {
		t.Fatalf("backfill not idempotent: %v", m["changed"])
	}

	// Non-admins are rejected before the handler runs.
	tok2, _ := registerUser(t, r, "bfuser", "bf2@x.com")
	if w := doJSON(t, r, "POST", "/api/admin/blueprints/backfill-brands", nil, tok2); w.Code != 403 {
		t.Fatalf("non-admin expected 403, got %d", w.Code)
	}
}

func TestSSRUnpublishedIs404(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "ssrpriv", "ssrpriv@x.com")
	bpID := createBlueprint(t, r, tok, "未发布SSR", false)
	req := httptest.NewRequest("GET", "/detail/"+bpID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("SSR unpublished expected 404, got %d", w.Code)
	}
}

// TestSitemapIndexAndPagesChild verifies the root sitemap is now an index and
// that the pages child still carries the site's real (non-hash) URLs.
func TestSitemapIndexAndPagesChild(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "smuser", "sm@x.com")
	createBlueprint(t, r, tok, "站点地图作品", true)

	// Root must be a sitemap index pointing at the children.
	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("sitemap index: %d", w.Code)
	}
	index := w.Body.String()
	if !strings.Contains(index, "<sitemapindex") {
		t.Fatalf("root sitemap is not a sitemap index: %s", index[:200])
	}
	if strings.Contains(index, "<urlset") {
		t.Fatal("root sitemap index must not contain a urlset")
	}
	for _, child := range []string{"/sitemap-zh.xml", "/sitemap-en.xml"} {
		if !strings.Contains(index, child) {
			t.Fatalf("index missing child %s", child)
		}
	}

	// The pages child keeps the old assertions.
	req = httptest.NewRequest("GET", "/sitemap-zh.xml", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("sitemap pages: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<urlset") {
		t.Fatal("urlset missing")
	}
	if strings.Contains(body, "#/detail") {
		t.Fatal("sitemap still uses hash URLs")
	}
	if !strings.Contains(body, "/explore") {
		t.Fatal("explore missing from sitemap")
	}
}

// TestImageOrderingBySortOrder verifies images are ordered by sort_order, NOT by
// the DB id (a random UUID). Images are inserted with sort_order out of insertion
// order; the API must return them sorted by sort_order.
func TestImageOrderingBySortOrder(t *testing.T) {
	r, gdb := setupTest(t)
	tok, _ := registerUser(t, r, "orduser", "ord@x.com")
	bpID := createBlueprint(t, r, tok, "顺序测试", true)

	created := []db.BlueprintImage{
		{BlueprintID: bpID, URL: "u1", SortOrder: 2, FileType: "image"},
		{BlueprintID: bpID, URL: "u2", SortOrder: 0, FileType: "image"},
		{BlueprintID: bpID, URL: "u3", SortOrder: 1, FileType: "image"},
	}
	for i := range created {
		if err := gdb.Create(&created[i]).Error; err != nil {
			t.Fatalf("create image: %v", err)
		}
	}

	w := doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, "")
	if w.Code != 200 {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	images := parseJSON(t, w)["images"].([]any)
	if len(images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(images))
	}
	want := []string{"u2", "u3", "u1"} // sort_order 0,1,2 - NOT insertion order
	for i, im := range images {
		got := im.(map[string]any)["url"].(string)
		if got != want[i] {
			t.Fatalf("image[%d] = %q, want %q (must sort by sort_order, not id)", i, got, want[i])
		}
	}
}

// ── Comment moderation ──

func makeAdmin(t *testing.T, gdb *gorm.DB, userID string) {
	t.Helper()
	if err := gdb.Model(&db.User{}).Where("id = ?", userID).Update("is_admin", true).Error; err != nil {
		t.Fatalf("make admin: %v", err)
	}
}

func listPublicComments(t *testing.T, r http.Handler, bpID string) []any {
	t.Helper()
	w := doJSON(t, r, "GET", "/api/blueprints/"+bpID+"/comments", nil, "")
	if w.Code != 200 {
		t.Fatalf("list comments: %d %s", w.Code, w.Body.String())
	}
	var arr []any
	json.Unmarshal(w.Body.Bytes(), &arr)
	return arr
}

// New comments are approved by default (publicly visible) and an admin can pin
// one to the top of the blueprint's comment list.
func TestCommentModerationDefaultsAndPin(t *testing.T) {
	r, gdb := setupTest(t)
	tokA, uidA := registerUser(t, r, "modA", "moda@x.com")
	bpID := createBlueprint(t, r, tokA, "mod bp", true)
	for _, c := range []string{"first", "second"} {
		if w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/comments",
			map[string]string{"content": c}, tokA); w.Code != 201 {
			t.Fatalf("comment: %d %s", w.Code, w.Body.String())
		}
	}
	arr := listPublicComments(t, r, bpID)
	if len(arr) != 2 {
		t.Fatalf("default-approved comments expected 2, got %d", len(arr))
	}
	secondID := arr[1].(map[string]any)["id"].(string)

	makeAdmin(t, gdb, uidA)
	if w := doJSON(t, r, "PUT", "/api/admin/comments/"+secondID+"/pin",
		map[string]bool{"pinned": true}, tokA); w.Code != 200 {
		t.Fatalf("pin: %d %s", w.Code, w.Body.String())
	}
	arr = listPublicComments(t, r, bpID)
	first := arr[0].(map[string]any)
	if first["id"] != secondID {
		t.Fatalf("pinned comment not first: %v", first["id"])
	}
	if first["is_pinned"] != true {
		t.Fatal("pinned comment missing is_pinned=true")
	}
}

func TestCommentHideRemovesFromPublicList(t *testing.T) {
	r, gdb := setupTest(t)
	tokA, uidA := registerUser(t, r, "hideA", "hidea@x.com")
	bpID := createBlueprint(t, r, tokA, "hide bp", true)
	w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/comments",
		map[string]string{"content": "bad"}, tokA)
	cid := parseJSON(t, w)["id"].(string)
	// A reply to the offending comment must also disappear when the parent is hidden.
	if w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/comments",
		map[string]any{"content": "reply", "parent_id": cid}, tokA); w.Code != 201 {
		t.Fatalf("reply: %d %s", w.Code, w.Body.String())
	}

	makeAdmin(t, gdb, uidA)
	if w := doJSON(t, r, "PUT", "/api/admin/comments/"+cid+"/status",
		map[string]string{"status": "hidden"}, tokA); w.Code != 200 {
		t.Fatalf("hide: %d %s", w.Code, w.Body.String())
	}
	if arr := listPublicComments(t, r, bpID); len(arr) != 0 {
		t.Fatalf("hidden comment (or its reply) still public, got %d", len(arr))
	}
	// Admin listing still shows both, flagged hidden.
	w = doJSON(t, r, "GET", "/api/admin/comments", nil, tokA)
	items := parseJSON(t, w)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("admin list expected 2, got %d", len(items))
	}
	if items[0].(map[string]any)["status"] != "hidden" {
		t.Fatal("admin list status != hidden")
	}
	if items[0].(map[string]any)["blueprint_title"] != "hide bp" {
		t.Fatalf("admin list blueprint_title = %v", items[0].(map[string]any)["blueprint_title"])
	}
	// Invalid status is rejected.
	if w := doJSON(t, r, "PUT", "/api/admin/comments/"+cid+"/status",
		map[string]string{"status": "bogus"}, tokA); w.Code != 400 {
		t.Fatalf("invalid status expected 400, got %d", w.Code)
	}
}

// Regression: the admin guard must reject BEFORE the handler runs. Previously
// AdminRequired delegated to AuthRequired's closure, whose c.Next() executed the
// admin handler first — so a non-admin's mutation committed and only then got a
// 403 appended.
func TestAdminEndpointsRejectNonAdminBeforeHandler(t *testing.T) {
	r, gdb := setupTest(t)
	tokA, _ := registerUser(t, r, "victim", "victim@x.com")
	bpID := createBlueprint(t, r, tokA, "protected bp", true)
	tokB, _ := registerUser(t, r, "attacker", "attacker@x.com")

	w := doJSON(t, r, "DELETE", "/api/admin/blueprints/"+bpID, nil, tokB)
	if w.Code != 403 {
		t.Fatalf("non-admin admin delete expected 403, got %d: %s", w.Code, w.Body.String())
	}
	var count int64
	gdb.Model(&db.Blueprint{}).Where("id = ?", bpID).Count(&count)
	if count != 1 {
		t.Fatal("non-admin executed an admin delete (privilege escalation)")
	}
}

func TestAdminCommentEndpointsRequireAdmin(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "nope", "nope@x.com")
	if w := doJSON(t, r, "GET", "/api/admin/comments", nil, tok); w.Code != 403 {
		t.Fatalf("non-admin comment list expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteCommentRemovesReplies(t *testing.T) {
	r, gdb := setupTest(t)
	tokA, uidA := registerUser(t, r, "delA", "dela@x.com")
	bpID := createBlueprint(t, r, tokA, "del bp", true)
	w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/comments",
		map[string]string{"content": "parent"}, tokA)
	pid := parseJSON(t, w)["id"].(string)
	if w := doJSON(t, r, "POST", "/api/blueprints/"+bpID+"/comments",
		map[string]any{"content": "reply", "parent_id": pid}, tokA); w.Code != 201 {
		t.Fatalf("reply: %d %s", w.Code, w.Body.String())
	}

	makeAdmin(t, gdb, uidA)
	if w := doJSON(t, r, "DELETE", "/api/admin/comments/"+pid, nil, tokA); w.Code != 204 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	var count int64
	gdb.Model(&db.Comment{}).Where("blueprint_id = ?", bpID).Count(&count)
	if count != 0 {
		t.Fatalf("expected 0 comments after cascade delete, got %d", count)
	}
}
