package handler

import (
	"strings"
	"unicode"
)

// Titles created by the import/seeder pipeline end with the manufacturer reference
// in the LAST parenthesis group, e.g.:
//
//	钢铁侠 Mark 3 收藏版 (LEGO 76344)
//	林间蘑菇 (LEGO 11505)
//	某款场景（森宝 10236）
//
// parseBrandFromTitle splits that group into a brand and a product code (型号/编号)
// so existing rows can be back-filled into the brand / product_code columns.

// brandAliases maps the foreign brand spellings seen in titles to the canonical
// Chinese name used by the upload-page dropdown. Keys are lower-cased and stripped
// of spaces / ® / ™ / punctuation before the lookup.
var brandAliases = map[string]string{
	"lego":   "乐高",
	"lego乐高": "乐高",
	"乐高lego": "乐高",
}

// parseBrandFromTitle returns (brand, productCode, ok). ok is false when the title
// has no usable parenthesis group; brand may be empty (e.g. "(76344)") while the
// product code is still returned.
func parseBrandFromTitle(title string) (brand, productCode string, ok bool) {
	groups := parenGroups(title)
	if len(groups) == 0 {
		return "", "", false
	}
	inner := strings.TrimSpace(groups[len(groups)-1])
	if inner == "" {
		return "", "", false
	}

	fields := fieldsExceptCodes(stripParens(inner))
	codeIdx := -1
	for i, f := range fields {
		if digitCount(f) >= 3 && isCodeToken(f) {
			codeIdx = i
			break
		}
	}
	if codeIdx < 0 {
		return "", "", false
	}

	code := strings.ToUpper(strings.Trim(fields[codeIdx], "-_."))
	if code == "" || len([]rune(code)) > maxProductCodeLen {
		return "", "", false
	}

	brand = normalizeBrand(strings.Join(fields[:codeIdx], " "))
	if len([]rune(brand)) > maxBrandLen {
		brand = ""
	}
	return brand, code, true
}

// parenGroups returns the inner text of every TOP-LEVEL parenthesis group, in order,
// for both ASCII and full-width parentheses. Titles do contain nested groups such as
// "新型远程战略轰炸机（下册） (奇积 23025（下册）)", where only the outer group carries the
// brand and code; the nested part is dropped later by stripParens.
func parenGroups(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(', '（':
			depth++
			if depth == 1 {
				cur.Reset()
				continue
			}
		case ')', '）':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 {
				out = append(out, cur.String())
				continue
			}
		}
		if depth >= 1 {
			cur.WriteRune(r)
		}
	}
	return out
}

// stripParens removes nested parenthetical content (and the parentheses themselves)
// so a sub-label like "（下册）" glued to the code does not corrupt the code token.
func stripParens(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(', '（':
			depth++
		case ')', '）':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// fieldsExceptCodes splits a parenthesis body on spaces and separators, keeping '-'
// and '_' inside tokens so codes such as "60183-1" stay in one piece.
func fieldsExceptCodes(s string) []string {
	sep := func(r rune) bool {
		switch r {
		case ' ', '\t', '\u3000', '|', '·', ':', '：', ',', '，', '、', ';', '；', '=', '+', '~', '～':
			return true
		}
		return false
	}
	return strings.FieldsFunc(s, sep)
}

// isCodeToken reports whether a token looks like a set/model number: only letters,
// digits and the '-', '_' or '.' joiners.
func isCodeToken(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func digitCount(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// normalizeBrand canonicalises the brand text: trims decoration, collapses the
// known aliases to the dropdown spelling, and otherwise keeps the original.
func normalizeBrand(b string) string {
	b = strings.TrimSpace(b)
	if b == "" {
		return ""
	}
	key := strings.ToLower(b)
	key = strings.NewReplacer("®", "", "™", "", " ", "", "\u3000", "", ".", "", "-", "").Replace(key)
	if canonical, found := brandAliases[key]; found {
		return canonical
	}
	return b
}
