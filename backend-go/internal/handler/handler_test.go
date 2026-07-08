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

func setupTest(t *testing.T) (*gin.Engine, *gorm.DB) {
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

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any, token string) *httptest.ResponseRecorder {
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

func registerUser(t *testing.T, r *gin.Engine, username, email string) (token, userID string) {
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

func createBlueprint(t *testing.T, r *gin.Engine, token, title string, published bool) string {
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
		{"123"},          // too short
		{"allletters"},   // no digit
		{"12345678"},     // no letter
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

func uploadAvatarRequest(t *testing.T, r *gin.Engine, token, filename, contentType string, content []byte) *httptest.ResponseRecorder {
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

func TestSitemapUsesRealURLs(t *testing.T) {
	r, _ := setupTest(t)
	tok, _ := registerUser(t, r, "smuser", "sm@x.com")
	createBlueprint(t, r, tok, "站点地图作品", true)

	req := httptest.NewRequest("GET", "/sitemap.xml", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("sitemap: %d", w.Code)
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
