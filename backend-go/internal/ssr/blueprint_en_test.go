package ssr

import (
	"strings"
	"testing"

	"brickplans/internal/db"
)

func bpStrPtr(s string) *string { return &s }

// The blueprint detail pages are a hreflang cluster now that every row has a
// real English title (cmd/translate-blueprints, 2026-10-24). The cluster has to
// be byte-identical to what sitemap-en advertises, otherwise Search Console
// reports "no return tags" for the whole group.
func TestBlueprintDetailAlternates(t *testing.T) {
	got := blueprintDetailAlternates("3945fa8f", "https://brickplan.test")
	want := []AltLink{
		{"en", "https://brickplan.test/en/detail/3945fa8f"},
		{"zh-CN", "https://brickplan.test/detail/3945fa8f"},
		{"x-default", "https://brickplan.test/en/detail/3945fa8f"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d alternates, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("alternate[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A row without title_en still renders the Chinese title on both trees, so the
// English URL must keep consolidating into the Chinese document. Flipping it to
// self-canonical too early is what creates a duplicate pair.
func TestBlueprintHasEnglish(t *testing.T) {
	t.Run("null title_en", func(t *testing.T) {
		if blueprintHasEnglish(&db.Blueprint{Title: "四不相"}) {
			t.Fatal("nil TitleEN reported as translated")
		}
	})
	t.Run("blank title_en", func(t *testing.T) {
		if blueprintHasEnglish(&db.Blueprint{Title: "四不相", TitleEN: bpStrPtr("   ")}) {
			t.Fatal("whitespace TitleEN reported as translated")
		}
	})
	t.Run("real title_en", func(t *testing.T) {
		if !blueprintHasEnglish(&db.Blueprint{Title: "四不相", TitleEN: bpStrPtr("Sibuxiang")}) {
			t.Fatal("real TitleEN reported as untranslated")
		}
	})
}

// Category is stored in Chinese and shipped back to the API verbatim; only the
// English chrome may translate it. The /en/detail noscript body used to print
// "Category: 其他" on 1,801 of 2,135 rows.
func TestDetailNoscriptLocalisesCategory(t *testing.T) {
	bp := &db.Blueprint{
		Title:       "X-Wing",
		TitleEN:     bpStrPtr("X-Wing"),
		Category:    bpStrPtr("星战"),
		IsPublished: true,
	}
	en := string(detailNoscriptFor("en", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(en, "Star Wars") {
		t.Fatalf("EN noscript missing English category label: %s", en)
	}
	if strings.Contains(en, "星战") {
		t.Fatalf("EN noscript leaked the stored Chinese category: %s", en)
	}
	zh := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(zh, "星战") {
		t.Fatalf("ZH noscript lost its category: %s", zh)
	}
}

// Census of SELECT DISTINCT category on production (2026-10-24). Every stored
// value needs an English label, otherwise the English tree prints Chinese.
func TestCategoryLabelENCoversStoredValues(t *testing.T) {
	stored := []string{"其他", "星战", "城市", "朋友", "图标", "机械", "创意", "赛车", "场景", "建筑"}
	for _, cat := range stored {
		if got := categoryLabelEN(cat); got == cat {
			t.Errorf("categoryLabelEN(%q) returned the stored value — missing English label", cat)
		}
	}
	if got := categoryLabelEN("未收录"); got != "未收录" {
		t.Errorf("unknown category should pass through, got %q", got)
	}
}
