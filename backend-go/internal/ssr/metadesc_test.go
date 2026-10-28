package ssr

import (
	"strings"
	"testing"
	"unicode/utf8"

	"brickplans/internal/db"
)

// B1: the detail meta description is derived from the editorial body and must
// stay inside the length Google shows, cut at a sentence end.
func TestDetailMetaDescriptionFromLongBody(t *testing.T) {
	zh := "这款奇积 3717 经典舰艇套装属于海底小纵队系列，盒面标注启蒙，定位是简易拼装积木，难度分 2/5，上手门槛不高。" +
		"配色以蓝绿为主，搭配橙和灰，整体是海底场景的基调。成品包含潜艇、章鱼堡、宝箱和海底场景，并附带人仔。" +
		"拼装时建议先按颜色分堆，再依次完成潜艇与章鱼堡的主体结构。"
	en := "This KEEPPLEY 3717 set is aimed at younger builders, ships with two hulls and a sticker sheet, and the finished model reads as one small underwater scene. " +
		"The build also includes a submarine, an octopus fortress and a chest, plus two minifigures. " +
		"Sort the parts by colour before you start, then build the two hulls in parallel."
	cases := []struct {
		name  string
		bp    db.Blueprint
		lang  string
		min   int
		max   int
		ends  string
		notIn []string
	}{
		{name: "zh long", bp: db.Blueprint{DescriptionLong: strPtr(zh)}, lang: "zh", min: 75, max: 160, ends: "。"},
		{name: "en long", bp: db.Blueprint{DescriptionLongEN: strPtr(en)}, lang: "en", min: 150, max: 160, ends: "."},
		{name: "zh markdown heading skipped",
			bp:   db.Blueprint{DescriptionLong: strPtr("## 这款套装值得搭\n\n" + zh)},
			lang: "zh", min: 75, max: 160, ends: "。", notIn: []string{"##"}},
		{name: "en no punctuation hard cut",
			bp:   db.Blueprint{DescriptionLongEN: strPtr(strings.Repeat("brick building ", 40))},
			lang: "en", min: 150, max: 160, ends: "…"},
		{name: "zh no punctuation hard cut",
			bp:   db.Blueprint{DescriptionLong: strPtr(strings.Repeat("积木", 200))},
			lang: "zh", min: 75, max: 160, ends: "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detailMetaDescription(&c.bp, c.lang)
			n := utf8.RuneCountInString(got)
			if n < c.min || n > c.max {
				t.Fatalf("length %d outside [%d,%d]: %q", n, c.min, c.max, got)
			}
			if !strings.HasSuffix(got, c.ends) {
				t.Fatalf("want suffix %q, got %q", c.ends, got)
			}
			for _, bad := range c.notIn {
				if strings.Contains(got, bad) {
					t.Fatalf("meta description still contains %q: %q", bad, got)
				}
			}
			if n > 160 {
				t.Fatalf("hard cap 160 broken: %d", n)
			}
		})
	}
}

// A row with no editorial body keeps the pre-B1 short description.
func TestDetailMetaDescriptionFallsBackToShort(t *testing.T) {
	short := "奇积 3717 经典舰艇，海底小纵队系列，简易拼装。"
	bp := db.Blueprint{Title: "经典舰艇 (奇积 3717)", Description: strPtr(short)}
	if got := detailMetaDescription(&bp, "zh"); got != short {
		t.Fatalf("zh fallback = %q, want the short description", got)
	}
	bpEN := db.Blueprint{Title: "经典舰艇", TitleEN: strPtr("Classic Ship"), DescriptionEN: strPtr("A small KEEPPLEY ship.")}
	if got := detailMetaDescription(&bpEN, "en"); got != "A small KEEPPLEY ship." {
		t.Fatalf("en fallback = %q", got)
	}
	// Nothing at all: the generated sentence is still used, never an empty tag.
	empty := db.Blueprint{Title: "无名"}
	if got := detailMetaDescription(&empty, "zh"); got != "BrickPlan 积木图纸：无名" {
		t.Fatalf("empty fallback = %q", got)
	}
}

// English must not inherit the Chinese body: /en/ pages of untranslated rows
// fall back to the short (or generated) description instead of printing Chinese.
func TestDetailMetaDescriptionNeverCrossesLanguages(t *testing.T) {
	bp := db.Blueprint{Title: "机甲", TitleEN: strPtr("Mecha"), DescriptionLong: strPtr(strings.Repeat("中文正文。", 40))}
	got := detailMetaDescription(&bp, "en")
	for _, r := range got {
		if r >= 0x4e00 && r <= 0x9fff {
			t.Fatalf("Chinese leaked into the English meta description: %q", got)
		}
	}
}
