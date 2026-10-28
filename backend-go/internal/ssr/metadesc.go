package ssr

import (
	"regexp"
	"strings"

	"brickplans/internal/db"
)

// B1 — the detail page's <meta name="description">.
//
// The stored short description is a one-liner (median 75 characters on a
// 40-page sample, 37 of them under 100), while 2,133 rows carry a 260–300
// character editorial body that the SERP never saw. That body is now the
// source, clipped to a whole sentence inside the length Google actually shows:
//
//	Chinese : cut at the last 。！？ inside 80 runes; if that lands short of 75
//	          look further (still under the 160 cap) so the snippet is never a
//	          stub; no sentence end at all → hard cut 80 + "…"
//	English : cut at the last sentence period inside 155 runes, ≥150 when one
//	          exists; no period → hard cut 155 + "…"
//
// A row without a long body keeps the pre-B1 output byte for byte (short
// description, truncate at 160), so untranslated / bodyless pages do not move.
const (
	metaZhTarget  = 80
	metaZhMin     = 75
	metaEnTarget  = 155
	metaEnMin     = 150
	metaHardLimit = 160
)

// detailMetaDescription returns the meta/OG description for one detail page in
// one site language.
func detailMetaDescription(bp *db.Blueprint, lang string) string {
	if long := bp.DescriptionLongIn(lang); long != nil {
		if body := metaPlainText(*long, lang); body != "" {
			if lang == "en" {
				return clipMetaEN(body)
			}
			return clipMetaZH(body)
		}
	}
	// Fallback: exactly what the handler used before B1.
	if lang == "en" {
		desc := "BrickPlan MOC blueprint: " + bp.TitleIn("en")
		if d := bp.DescriptionIn("en"); d != nil && *d != "" {
			desc = *d
		}
		return truncate(desc, metaHardLimit)
	}
	desc := "BrickPlan 积木图纸：" + bp.Title
	if bp.Description != nil && *bp.Description != "" {
		desc = *bp.Description
	}
	return truncate(desc, metaHardLimit)
}

var (
	mdLinkRE = regexp.MustCompile(`!?\[([^\]]*)\]\(([^)]*)\)`)
	mdTagRE  = regexp.MustCompile(`<[^>]*>`)
	mdRunsRE = regexp.MustCompile(`[ \t]{2,}`)
)

// metaPlainText flattens the markdown body into one run of prose: headings are
// dropped (they are fragments without a full stop and read badly in a SERP),
// list markers and inline markup are stripped, paragraphs are joined with a
// space in English and with nothing in Chinese.
func metaPlainText(body, lang string) string {
	body = strings.ReplaceAll(body, "\r", "")
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		line = mdLinkRE.ReplaceAllString(line, "$1")
		line = mdTagRE.ReplaceAllString(line, "")
		line = strings.NewReplacer("**", "", "__", "", "~~", "", "`", "").Replace(line)
		line = strings.TrimSpace(strings.TrimLeft(line, "-*+•> "))
		line = strings.TrimSpace(mdRunsRE.ReplaceAllString(line, " "))
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	sep := ""
	if lang == "en" {
		sep = " "
	}
	return strings.TrimSpace(strings.Join(kept, sep))
}

func clipMetaZH(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= metaZhMin {
		return string(r)
	}
	if i := lastSentenceEnd(r, metaZhTarget, ""); i >= 0 && i+1 >= metaZhMin {
		return string(r[:i+1])
	}
	if i := lastSentenceEnd(r, metaHardLimit, ""); i >= 0 && i+1 >= metaZhMin {
		return string(r[:i+1])
	}
	return string(r[:metaZhTarget]) + "…"
}

func clipMetaEN(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= metaEnMin {
		return string(r)
	}
	if i := lastSentenceEnd(r, metaEnTarget, "en"); i >= 0 && i+1 >= metaEnMin {
		return string(r[:i+1])
	}
	if i := lastSentenceEnd(r, metaHardLimit-2, "en"); i >= 0 && i+1 >= metaEnMin {
		return string(r[:i+1])
	}
	return hardCutEN(r)
}

// lastSentenceEnd returns the index of the last sentence-ending punctuation
// strictly before limit, or -1. In English a period only counts when it closes
// a sentence (followed by a space or the end of the text) and is not part of a
// number such as 10311 or 3.5, because cutting at "e.g." or "U.S." mid-clause
// reads like a truncated title.
func lastSentenceEnd(r []rune, limit int, lang string) int {
	if limit > len(r) {
		limit = len(r)
	}
	for i := limit - 1; i >= 0; i-- {
		c := r[i]
		if lang == "en" {
			if c != '.' && c != '!' && c != '?' {
				continue
			}
			if i+1 < len(r) && r[i+1] != ' ' && r[i+1] != '"' && r[i+1] != '”' {
				continue
			}
			if c == '.' && i > 0 && i+1 < len(r) && (isDigit(r[i-1]) || isDigit(r[i+1])) {
				continue
			}
			return i
		}
		if c == '。' || c == '！' || c == '？' || c == '!' || c == '?' {
			return i
		}
	}
	return -1
}

// hardCutEN is the last resort for a body with no usable sentence end: cut at
// 155 runes, back off to the previous word boundary when that keeps the snippet
// long enough, and mark the truncation with an ellipsis instead of a dangling
// comma or space.
func hardCutEN(r []rune) string {
	limit := metaEnTarget
	if limit > len(r) {
		limit = len(r)
	}
	cut := r[:limit]
	for i := len(cut) - 1; i >= metaEnMin; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " ,;:—-") + "…"
}

func isDigit(c rune) bool { return c >= '0' && c <= '9' }

// detailOGDescription is the pre-B1 behaviour, verbatim: the share card keeps
// the stored one-liner. B1 only asked for the SERP snippet, and og:description
// drives the chat/social preview where the short line next to the cover image
// already reads complete.
func detailOGDescription(bp *db.Blueprint, lang string) string {
	if lang == "en" {
		desc := "BrickPlan MOC blueprint: " + bp.TitleIn("en")
		if d := bp.DescriptionIn("en"); d != nil && *d != "" {
			desc = *d
		}
		return truncate(desc, metaHardLimit)
	}
	desc := "BrickPlan 积木图纸：" + bp.Title
	if bp.Description != nil && *bp.Description != "" {
		desc = *bp.Description
	}
	return truncate(desc, metaHardLimit)
}
