package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func ptr(s string) *string { return &s }

func TestSelectPending(t *testing.T) {
	rows := []row{
		{ID: "both-missing", Title: "城堡", Description: ptr("描述")},
		{ID: "title-only", Title: "街景", Description: nil, TitleEN: ptr("Street")},
		{ID: "done", Title: "汽车", Description: ptr("描述"), TitleEN: ptr("Car"), DescriptionEN: ptr("A car")},
		{ID: "blank-fields", Title: "飞机", Description: ptr("描述"), TitleEN: ptr("  "), DescriptionEN: ptr("")},
		{ID: "no-chinese-desc", Title: "火车", Description: ptr("   "), TitleEN: ptr("Train")},
	}

	got := selectPending(rows, false)
	want := map[string]bool{"both-missing": true, "blank-fields": true}
	if len(got) != len(want) {
		t.Fatalf("pending = %d rows, want %d: %+v", len(got), len(want), got)
	}
	for _, r := range got {
		if !want[r.ID] {
			t.Errorf("unexpected pending row %s", r.ID)
		}
	}

	// --force must include rows that are already complete, but still skip rows
	// with no Chinese description (there is nothing to translate there).
	forced := selectPending(rows, true)
	if len(forced) != len(rows) {
		t.Fatalf("forced = %d rows, want %d", len(forced), len(rows))
	}
	if needsDescription(rows[4], true) {
		t.Error("a blank Chinese description must never request description_en")
	}
}

func TestNormalizeTitle(t *testing.T) {
	if got, ok := normalizeTitle("  Town   Corner  "); !ok || got != "Town Corner" {
		t.Errorf("normalizeTitle = %q, %v", got, ok)
	}
	if _, ok := normalizeTitle("   "); ok {
		t.Error("blank title must be rejected")
	}

	long := strings.Repeat("brick ", 40) // 240 chars, well over the limit
	got, ok := normalizeTitle(long)
	if !ok {
		t.Fatal("long title should be truncated, not rejected")
	}
	if n := utf8.RuneCountInString(got); n > titleLimit {
		t.Errorf("truncated title has %d runes, limit %d", n, titleLimit)
	}
	if strings.HasSuffix(got, "bric") {
		t.Errorf("title was cut mid-word: %q", got)
	}
}

func TestNormalizeDescription(t *testing.T) {
	got, ok := normalizeDescription("\n# 标题\n\n- 项目\n")
	if !ok || got != "# 标题\n\n- 项目" {
		t.Errorf("normalizeDescription = %q, %v", got, ok)
	}
	if _, ok := normalizeDescription(" \n "); ok {
		t.Error("blank description must be rejected")
	}

	huge := strings.Repeat("积木", descBytesLimit) // 2x the byte cap
	got, ok = normalizeDescription(huge)
	if !ok {
		t.Fatal("oversized description should be cut, not rejected")
	}
	if len(got) > descBytesLimit+len("…") {
		t.Errorf("description kept %d bytes, cap %d", len(got), descBytesLimit)
	}
	if !utf8.ValidString(got) {
		t.Error("cut description is not valid UTF-8")
	}
}

func TestParseTranslationResponse(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"plain", `{"items":[{"id":"a","title_en":"Castle","description_en":"# Castle"}]}`},
		{"fenced", "```json\n{\"items\":[{\"id\":\"a\",\"title_en\":\"Castle\",\"description_en\":\"\"}]}\n```"},
		{"surrounded", `Here you go: {"items":[{"id":"a","title_en":"Castle","description_en":""}]} thanks`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := parseTranslationResponse(tc.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(resp.Items) != 1 || resp.Items[0].ID != "a" || resp.Items[0].TitleEN != "Castle" {
				t.Fatalf("unexpected items: %+v", resp.Items)
			}
		})
	}

	if _, err := parseTranslationResponse("no json here"); err == nil {
		t.Error("expected an error for a response without JSON")
	}
	if _, err := parseTranslationResponse(`{"items":[]}`); err == nil {
		t.Error("expected an error for an empty item list")
	}
}

func TestBuildUserPromptCarriesSourceText(t *testing.T) {
	prompt := buildUserPrompt([]row{{ID: "id-1", Title: "乐高城堡", Description: ptr("这座城堡有 3200 个零件。")}})
	for _, want := range []string{"id-1", "乐高城堡", "3200", "title_en", "LEGO"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
