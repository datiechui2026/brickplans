package ssr

import (
	"strings"
	"testing"
)

// Ticket SEO-20261026-02 batch 2: the long editorial body. These pin the slot
// itself -- where it renders, that an empty field renders nothing at all, and
// that the English tree never inherits the Chinese body.

func TestDescriptionLongRendersBetweenDescriptionAndSpecTable(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLong = strPtr("## 这款钢铁侠为什么值得搭\n\n第一段正文内容。\n\n第二段正文内容。")
	html := string(detailNoscriptFor("zh", bp, "https://cdn.test/1.jpg", "https://brickplan.test", nil))
	if !strings.Contains(html, `<div class="desc-long">`) {
		t.Fatal("no <div class=\"desc-long\"> container for a row that has description_long")
	}
	if !strings.Contains(html, "这款钢铁侠为什么值得搭") {
		t.Fatal("long body text missing from the rendered page")
	}
	// ordering: after the short description, before the spec table
	iDesc := strings.Index(html, "奇积 官方套装 S0101 钢铁侠 搭建说明书。")
	iLong := strings.Index(html, `<div class="desc-long">`)
	iSpec := strings.Index(html, `<dl class="spec-table">`)
	if iDesc < 0 || iLong < 0 || iSpec < 0 {
		t.Fatalf("missing anchors: desc=%d long=%d spec=%d", iDesc, iLong, iSpec)
	}
	if !(iDesc < iLong && iLong < iSpec) {
		t.Fatalf("desc-long out of order: desc=%d long=%d spec=%d", iDesc, iLong, iSpec)
	}
}

// An absent or whitespace-only field must not emit the container at all -- the
// ticket bans 暂无 / 未知 / - placeholders.
func TestDescriptionLongEmptyRendersNothing(t *testing.T) {
	for _, v := range []*string{nil, strPtr(""), strPtr("   \n\t ")} {
		bp := bpFixture()
		bp.DescriptionLong = v
		bp.DescriptionLongEN = v
		html := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
		if strings.Contains(html, "desc-long") {
			t.Fatalf("empty long body still rendered a container (value=%v)", v)
		}
		if strings.Contains(html, "暂无") || strings.Contains(html, "未知") {
			t.Fatal("placeholder text leaked into the page")
		}
	}
}

// The English tree renders description_long_en only. Falling back to the Chinese
// body would put 300-500 characters of Chinese prose inside an English page.
func TestDescriptionLongNoCrossLanguageFallback(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLong = strPtr("中文长描述正文。")
	if html := string(detailNoscriptFor("en", bp, "", "https://brickplan.test", nil)); strings.Contains(html, "desc-long") {
		t.Fatal("English page rendered desc-long from the Chinese column")
	}
	bp.DescriptionLongEN = strPtr("English long body.")
	html := string(detailNoscriptFor("en", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(html, `<div class="desc-long">`) || !strings.Contains(html, "English long body.") {
		t.Fatal("English page did not render description_long_en")
	}
	if strings.Contains(html, "中文长描述正文") {
		t.Fatal("Chinese long body leaked into the English page")
	}
}

// DescriptionLongIn must trim-check, not nil-check, because empty strings are
// what the upload form writes when the author submits nothing.
func TestDescriptionLongInTrimsWhitespace(t *testing.T) {
	bp := bpFixture()
	bp.DescriptionLong = strPtr("  \t ")
	if got := bp.DescriptionLongIn("zh"); got != nil {
		t.Fatalf("DescriptionLongIn(zh) = %q, want nil", *got)
	}
	bp.DescriptionLong = strPtr("有内容")
	if got := bp.DescriptionLongIn("zh"); got == nil || *got != "有内容" {
		t.Fatalf("DescriptionLongIn(zh) = %v, want 有内容", got)
	}
}
