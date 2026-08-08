package handler_test

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"brickplans/internal/db"
)

// promoteAdmin flips is_admin on the given user. The register-issued token
// stays valid (token_version is unchanged), so it can then call /api/admin/*.
func promoteAdmin(t *testing.T, gdb *gorm.DB, userID string) {
	t.Helper()
	if err := gdb.Model(&db.User{}).Where("id = ?", userID).Update("is_admin", true).Error; err != nil {
		t.Fatalf("promote admin: %v", err)
	}
}

// TestFeaturedFallbackToLatest verifies that with no admin curation the home
// "热门推荐" endpoint falls back to the latest published blueprints, NOT by
// view_count (the old behavior the user asked to replace).
func TestFeaturedFallbackToLatest(t *testing.T) {
	r, gdb := setupTest(t)
	token, _ := registerUser(t, r, "feeder", "feed@x.com")

	a := createBlueprint(t, r, token, "A", true)
	b := createBlueprint(t, r, token, "B", true)
	c := createBlueprint(t, r, token, "C", true)
	unpub := createBlueprint(t, r, token, "hidden", false)

	// Force distinct view_counts and created_ats so latest-order != view-order.
	// view_count order would be A(100) > B(10) > C(1); latest order is C > B > A.
	gdb.Model(&db.Blueprint{}).Where("id = ?", a).Updates(map[string]any{"view_count": 100, "created_at": time.Now().Add(-2 * time.Hour)})
	gdb.Model(&db.Blueprint{}).Where("id = ?", b).Updates(map[string]any{"view_count": 10, "created_at": time.Now().Add(-1 * time.Hour)})
	gdb.Model(&db.Blueprint{}).Where("id = ?", c).Updates(map[string]any{"view_count": 1, "created_at": time.Now().Add(-1 * time.Minute)})
	_ = unpub

	w := doJSON(t, r, "GET", "/api/blueprints/featured?size=8", nil, "")
	if w.Code != 200 {
		t.Fatalf("featured: %d %s", w.Code, w.Body.String())
	}
	m := parseJSON(t, w)
	items := m["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 published blueprints, got %d", len(items))
	}
	got := []string{
		items[0].(map[string]any)["title"].(string),
		items[1].(map[string]any)["title"].(string),
		items[2].(map[string]any)["title"].(string),
	}
	want := []string{"C", "B", "A"} // latest first, NOT view_count (which would be A, B, C)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fallback order = %v, want %v (latest, not view_count)", got, want)
		}
	}
}

// TestFeaturedCuratedOrder verifies admins curate the list and the public
// endpoint returns exactly those, in admin-set order.
func TestFeaturedCuratedOrder(t *testing.T) {
	r, gdb := setupTest(t)
	token, userID := registerUser(t, r, "curator", "cur@x.com")
	promoteAdmin(t, gdb, userID)

	a := createBlueprint(t, r, token, "Alpha", true)
	b := createBlueprint(t, r, token, "Beta", true)
	c := createBlueprint(t, r, token, "Gamma", true) // not featured

	// Add in order Alpha, Beta.
	w := doJSON(t, r, "POST", "/api/admin/featured", map[string]string{"blueprint_id": a}, token)
	if w.Code != 201 {
		t.Fatalf("add a: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, r, "POST", "/api/admin/featured", map[string]string{"blueprint_id": b}, token)
	if w.Code != 201 {
		t.Fatalf("add b: %d %s", w.Code, w.Body.String())
	}

	// Duplicate add -> 409.
	w = doJSON(t, r, "POST", "/api/admin/featured", map[string]string{"blueprint_id": a}, token)
	if w.Code != 409 {
		t.Fatalf("duplicate add: expected 409, got %d", w.Code)
	}

	// Admin list -> [Alpha, Beta].
	w = doJSON(t, r, "GET", "/api/admin/featured", nil, token)
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	adminItems := parseJSON(t, w)["items"].([]any)
	if len(adminItems) != 2 {
		t.Fatalf("admin list: expected 2, got %d", len(adminItems))
	}

	// Public featured -> [Alpha, Beta] (Gamma excluded).
	w = doJSON(t, r, "GET", "/api/blueprints/featured?size=8", nil, "")
	if w.Code != 200 {
		t.Fatalf("public featured: %d %s", w.Code, w.Body.String())
	}
	items := parseJSON(t, w)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("public featured: expected 2, got %d", len(items))
	}
	got := []string{
		items[0].(map[string]any)["title"].(string),
		items[1].(map[string]any)["title"].(string),
	}
	if got[0] != "Alpha" || got[1] != "Beta" {
		t.Fatalf("curated order = %v, want [Alpha Beta]", got)
	}
	_ = c
}

// TestFeaturedReorderAndRemove verifies reorder + remove via the featured record id.
func TestFeaturedReorderAndRemove(t *testing.T) {
	r, gdb := setupTest(t)
	token, userID := registerUser(t, r, "editor", "ed@x.com")
	promoteAdmin(t, gdb, userID)

	a := createBlueprint(t, r, token, "A", true)
	b := createBlueprint(t, r, token, "B", true)

	doJSON(t, r, "POST", "/api/admin/featured", map[string]string{"blueprint_id": a}, token)
	w := doJSON(t, r, "POST", "/api/admin/featured", map[string]string{"blueprint_id": b}, token)
	bID := parseJSON(t, w)["id"].(string)

	// Reorder: swap so B (sort_order 0) comes before A (sort_order 1).
	listW := doJSON(t, r, "GET", "/api/admin/featured", nil, token)
	items := parseJSON(t, listW)["items"].([]any)
	aID := ""
	for _, it := range items {
		m := it.(map[string]any)
		if m["blueprint_id"].(string) == a {
			aID = m["id"].(string)
		}
	}
	w = doJSON(t, r, "PUT", "/api/admin/featured/reorder",
		map[string]any{"items": []map[string]any{
			{"id": bID, "sort_order": 0},
			{"id": aID, "sort_order": 1},
		}}, token)
	if w.Code != 200 {
		t.Fatalf("reorder: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, r, "GET", "/api/blueprints/featured?size=8", nil, "")
	titles := []string{}
	for _, it := range parseJSON(t, w)["items"].([]any) {
		titles = append(titles, it.(map[string]any)["title"].(string))
	}
	if len(titles) != 2 || titles[0] != "B" || titles[1] != "A" {
		t.Fatalf("after reorder = %v, want [B A]", titles)
	}

	// Remove B -> only A remains.
	w = doJSON(t, r, "DELETE", "/api/admin/featured/"+bID, nil, token)
	if w.Code != 204 {
		t.Fatalf("remove: expected 204, got %d", w.Code)
	}
	w = doJSON(t, r, "GET", "/api/blueprints/featured?size=8", nil, "")
	remaining := parseJSON(t, w)["items"].([]any)
	if len(remaining) != 1 || remaining[0].(map[string]any)["title"].(string) != "A" {
		t.Fatalf("after remove = %v, want [A]", remaining)
	}
}
