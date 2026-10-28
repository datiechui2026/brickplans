package handler_test

import (
	"testing"

	"brickplans/internal/db"
)

// TestDetailAPIExposesLongDescription covers the 2026-10-28 P0: the SPA detail
// page can only render 「套装介绍」 if /api/blueprints/{id} actually returns the
// long editorial body. The list projection must NOT carry it (long prose would
// bloat every card response), so both halves are asserted here.
func TestDetailAPIExposesLongDescription(t *testing.T) {
	r, gdb := setupTest(t)
	tok, _ := registerUser(t, r, "longuser", "long@x.com")
	bpID := createBlueprint(t, r, tok, "长描述图纸", true)

	const longZH = "宝可梦系列的大型皮卡丘，由 2018 颗粒组成，分上中下三册装订，适合进阶玩家。"
	const longEN = "A large-scale Pikachu from the Pokémon series, 2018 pieces, split into three volumes."

	if err := gdb.Model(&db.Blueprint{}).Where("id = ?", bpID).Updates(map[string]any{
		"description_long":    longZH,
		"description_long_en": longEN,
	}).Error; err != nil {
		t.Fatalf("seed long description: %v", err)
	}

	got := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints/"+bpID, nil, ""))
	if got["description_long"] != longZH {
		t.Fatalf("detail description_long = %v, want %q", got["description_long"], longZH)
	}
	if got["description_long_en"] != longEN {
		t.Fatalf("detail description_long_en = %v, want %q", got["description_long_en"], longEN)
	}

	// The short description must keep its own value (long body is an addition,
	// not a replacement) so meta description / og:description stay short.
	if _, ok := got["description"]; !ok {
		t.Fatalf("detail payload lost the short description field: %v", got)
	}

	lst := parseJSON(t, doJSON(t, r, "GET", "/api/blueprints?page=1&page_size=10", nil, ""))
	items, _ := lst["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("list returned no items: %v", lst)
	}
	for _, it := range items {
		item, _ := it.(map[string]any)
		if _, ok := item["description_long"]; ok {
			t.Fatalf("list item must not expose description_long, got %v", item["description_long"])
		}
		if _, ok := item["description_long_en"]; ok {
			t.Fatalf("list item must not expose description_long_en, got %v", item["description_long_en"])
		}
	}
}
