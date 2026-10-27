package ssr

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Ticket 02 batch 2: the long editorial body. Position, heading, fallback and
// no-truncation are all contractual, so each one gets its own pin. Empty rows
// must not leave a heading hanging over nothing (the ticket bans blank blocks
// and bans 暂无/未知 placeholders).

func TestSetIntroRendersBelowSpecTableWithZHHeading(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLong = strPtr("## 这款钢铁侠为什么值得搭\n\n第一段正文内容，讲的是分袋与拼装顺序。\n\n第二段正文内容，讲的是公差与替代件。")
	html := string(detailNoscriptFor("zh", bp, "https://cdn.test/1.jpg", "https://brickplan.test", nil))
	iSpec := strings.Index(html, `<dl class="spec-table">`)
	iSec := strings.Index(html, `<section class="desc-long" id="set-intro">`)
	if iSpec < 0 || iSec < 0 {
		t.Fatalf("missing anchors: spec=%d section=%d", iSpec, iSec)
	}
	if iSec < iSpec {
		t.Fatalf("套装介绍 must render BELOW the spec table: spec=%d section=%d", iSpec, iSec)
	}
	if !strings.Contains(html[iSec:], "<h2>套装介绍</h2>") {
		t.Fatalf("missing <h2>套装介绍</h2> after the spec table:\n%s", html[iSec:])
	}
	for _, want := range []string{"这款钢铁侠为什么值得搭", "讲的是分袋与拼装顺序", "讲的是公差与替代件"} {
		if !strings.Contains(html, want) {
			t.Fatalf("long body fragment %q missing", want)
		}
	}
	// Paragraphs, not one blob: the two blank-line separated blocks become two
	// <p>, and the leading "## " line becomes the section's <h3>.
	sec := html[iSec:]
	sec = sec[:strings.Index(sec, "</section>")]
	if n := strings.Count(sec, "<p>"); n != 2 {
		t.Fatalf("body should render as 2 <p> paragraphs, got %d:\n%s", n, sec)
	}
}

func TestSetIntroEnglishHeading(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLongEN = strPtr("Why this Iron Man set is worth building.\n\nPart one.\n\nPart two.")
	html := string(detailNoscriptFor("en", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(html, "<h2>About this set</h2>") {
		t.Fatal("English page missing <h2>About this set</h2>")
	}
	if strings.Contains(html, "<h2>套装介绍</h2>") {
		t.Fatal("Chinese heading leaked into the English page")
	}
	if !strings.Contains(html, "Why this Iron Man set is worth building.") {
		t.Fatal("description_long_en body missing from the English page")
	}
}

// A NULL long column falls back to the row's own short description, exactly once:
// the lead paragraph at the top is suppressed in that case so the same sentence
// cannot appear twice on one page.
func TestSetIntroFallsBackToShortDescription(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLong = nil
	short := *bp.Description
	if short == "" {
		t.Fatal("fixture lost its description")
	}
	html := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(html, `<section class="desc-long" id="set-intro">`) {
		t.Fatal("no editorial block for a row whose description_long is NULL")
	}
	if n := strings.Count(html, short); n != 1 {
		t.Fatalf("short description rendered %d times, want exactly 1 (lead suppressed)", n)
	}
	if !strings.Contains(html, "<h2>套装介绍</h2>") {
		t.Fatal("fallback block is missing its heading")
	}
}

func TestSetIntroOmitsBlockWhenBothFieldsEmpty(t *testing.T) {
	for _, v := range []*string{nil, strPtr(""), strPtr("   \n\t ")} {
		bp := bpFixture()
		bp.DescriptionLong, bp.DescriptionLongEN = v, v
		bp.Description, bp.DescriptionEN = v, v
		for _, lang := range []string{"zh", "en"} {
			html := string(detailNoscriptFor(lang, bp, "", "https://brickplan.test", nil))
			if strings.Contains(html, "set-intro") || strings.Contains(html, "desc-long") {
				t.Fatalf("%s: empty row still emitted the block (value=%v)", lang, v)
			}
			if strings.Contains(html, "暂无") || strings.Contains(html, "未知") {
				t.Fatalf("%s: placeholder text leaked", lang)
			}
		}
	}
}

// 250-500 characters go out verbatim. A 480-character body must survive in full;
// truncation at any boundary would silently cut the tail of the prose.
func TestSetIntroDoesNotTruncate(t *testing.T) {
	body := strings.Repeat("积木搭建的过程里，分袋与公差决定了手感。", 20) // 400 runes
	if n := utf8.RuneCountInString(body); n != 400 {
		t.Fatalf("fixture length = %d, want 400", n)
	}
	bp := bpFixture()
	bp.DescriptionLong = strPtr(body)
	html := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(html, body) {
		t.Fatal("long body was truncated or re-encoded")
	}
	if strings.Contains(html, "…") || strings.Contains(html, "...") {
		t.Fatal("ellipsis found in the rendered long body")
	}
}

// The English tree never inherits Chinese prose: no description_en and no
// description_long_en means no block at all, rather than 400 characters of
// Chinese under an English heading.
func TestSetIntroNoCrossLanguageFallback(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLong, bp.DescriptionLongEN = nil, nil
	bp.Description = strPtr("中文短文描述。")
	bp.DescriptionEN = nil
	if html := string(detailNoscriptFor("en", bp, "", "https://brickplan.test", nil)); strings.Contains(html, "set-intro") {
		t.Fatal("English page rendered the block from the Chinese column")
	}
}
