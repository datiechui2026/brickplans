package handler_test

import (
	"strings"
	"testing"

	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/handler"
)

// fakeWeChatClient stands in for the real jscode2session call so tests never
// hit WeChat's API. It returns a fixed openid/unionid (or an error).
type fakeWeChatClient struct {
	openid  string
	unionid string
	err     error
	calls   int
}

func (f *fakeWeChatClient) Code2Session(code string) (string, string, error) {
	f.calls++
	if f.err != nil {
		return "", "", f.err
	}
	return f.openid, f.unionid, nil
}

// withWeChatClient swaps in a fake WeChat client for the duration of the test,
// restoring the real factory on cleanup.
func withWeChatClient(t *testing.T, c handler.WeChatClient) {
	t.Helper()
	orig := handler.NewWeChatClient
	t.Cleanup(func() { handler.NewWeChatClient = orig })
	handler.NewWeChatClient = func(cfg *config.Config) handler.WeChatClient { return c }
}

// TestWeChatLogin covers the find-or-create flow: the first call creates a
// WeChat-only account and returns both tokens in the body (mini programs can't
// read the httpOnly cookie); the second call with the same openid reuses it.
func TestWeChatLogin(t *testing.T) {
	withWeChatClient(t, &fakeWeChatClient{openid: "wx_openid_123", unionid: "union_abc"})
	r, gdb := setupTest(t)

	// First login -> creates a user.
	w := doJSON(t, r, "POST", "/api/auth/wechat-login", map[string]string{"code": "anycode"}, "")
	if w.Code != 200 {
		t.Fatalf("wechat-login: %d %s", w.Code, w.Body.String())
	}
	m := parseJSON(t, w)
	at1, _ := m["access_token"].(string)
	rt1, _ := m["refresh_token"].(string)
	if at1 == "" || rt1 == "" {
		t.Fatalf("expected both access+refresh tokens in body, got %v", m)
	}
	user := m["user"].(map[string]any)
	userID := user["id"].(string)
	if !strings.HasPrefix(user["username"].(string), "微信用户_") {
		t.Fatalf("expected generated 微信用户_ username, got %v", user["username"])
	}
	// The body refresh token must equal the cookie so either path works.
	if c := extractCookie(w, "bp_refresh"); c == "" || c != rt1 {
		t.Fatalf("bp_refresh cookie should match body refresh token: cookie=%q body=%q", c, rt1)
	}

	// User persisted with openid/unionid, empty password hash, verified email.
	var u db.User
	if err := gdb.First(&u, "id = ?", userID).Error; err != nil {
		t.Fatalf("user not persisted: %v", err)
	}
	if u.WeChatOpenID == nil || *u.WeChatOpenID != "wx_openid_123" || u.WeChatUnionID != "union_abc" {
		t.Fatalf("openid/unionid not stored: %+v", u)
	}
	if u.PasswordHash != "" {
		t.Fatalf("WeChat user should have empty password hash, got %q", u.PasswordHash)
	}
	if !u.EmailVerified {
		t.Fatal("WeChat user should be email-verified (skip unverified cleanup)")
	}

	// Second login with the same openid -> reuses the account, no duplicate.
	w2 := doJSON(t, r, "POST", "/api/auth/wechat-login", map[string]string{"code": "anycode2"}, "")
	if w2.Code != 200 {
		t.Fatalf("second wechat-login: %d %s", w2.Code, w2.Body.String())
	}
	m2 := parseJSON(t, w2)
	if m2["user"].(map[string]any)["id"] != userID {
		t.Fatalf("second login should reuse the same user id, got %v", m2["user"])
	}
	var count int64
	gdb.Model(&db.User{}).Where("wechat_open_id = ?", "wx_openid_123").Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 user with this openid, got %d", count)
	}
}

// TestWeChatLoginNotConfigured verifies the endpoint 503s when the server has no
// WECHAT_APPID/AppSecret (the client factory returns nil).
func TestWeChatLoginNotConfigured(t *testing.T) {
	withWeChatClient(t, nil) // simulates "WeChat login disabled"
	r, _ := setupTest(t)

	w := doJSON(t, r, "POST", "/api/auth/wechat-login", map[string]string{"code": "x"}, "")
	if w.Code != 503 {
		t.Fatalf("expected 503 when WeChat not configured, got %d %s", w.Code, w.Body.String())
	}
}

// TestRefreshViaBody verifies cookieless clients (mini program) can rotate tokens
// by sending the refresh token in the JSON body, and that the new refresh token
// is returned in the body (since they can't read the cookie).
func TestRefreshViaBody(t *testing.T) {
	r, _ := setupTest(t)

	// Register a normal user; the refresh token arrives via the bp_refresh cookie.
	w := doJSON(t, r, "POST", "/api/auth/register",
		map[string]string{"username": "alice", "email": "alice@example.com", "password": "password123"}, "")
	if w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	rt := extractCookie(w, "bp_refresh")
	if rt == "" {
		t.Fatal("register did not set bp_refresh cookie")
	}

	// Refresh using the body only (doJSON sends no cookies) -> cookieless path.
	w2 := doJSON(t, r, "POST", "/api/auth/refresh", map[string]string{"refresh_token": rt}, "")
	if w2.Code != 200 {
		t.Fatalf("refresh via body: %d %s", w2.Code, w2.Body.String())
	}
	m := parseJSON(t, w2)
	if m["access_token"] == nil || m["access_token"] == "" {
		t.Fatal("expected a new access token")
	}
	rt2, _ := m["refresh_token"].(string)
	if rt2 == "" {
		t.Fatal("cookieless refresh should return the rotated refresh token in the body")
	}
	if rt2 == rt {
		t.Fatal("refresh token should rotate on each refresh")
	}

	// The rotated refresh token should work for a subsequent body refresh.
	w3 := doJSON(t, r, "POST", "/api/auth/refresh", map[string]string{"refresh_token": rt2}, "")
	if w3.Code != 200 {
		t.Fatalf("second refresh via body: %d %s", w3.Code, w3.Body.String())
	}
}

// TestWeChatUserCannotPasswordLogin verifies the login() guard: a WeChat-only
// account (empty PasswordHash) is rejected with a clear message rather than the
// generic "Invalid email or password".
func TestWeChatUserCannotPasswordLogin(t *testing.T) {
	withWeChatClient(t, &fakeWeChatClient{openid: "wx_only_user"})
	r, _ := setupTest(t)

	// Create the WeChat user.
	w := doJSON(t, r, "POST", "/api/auth/wechat-login", map[string]string{"code": "c"}, "")
	if w.Code != 200 {
		t.Fatalf("wechat-login: %d %s", w.Code, w.Body.String())
	}
	email := parseJSON(t, w)["user"].(map[string]any)["email"].(string)

	// Attempting password login with that account's email -> rejected.
	w2 := doJSON(t, r, "POST", "/api/auth/login",
		map[string]string{"email": email, "password": "anything"}, "")
	if w2.Code != 401 {
		t.Fatalf("expected 401 for WeChat-only account password login, got %d %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "微信登录") {
		t.Fatalf("expected WeChat-login hint, got %s", w2.Body.String())
	}
}
