package ssr

import (
	"regexp"
	"strings"

	"brickplans/internal/blog"
)

// QA is a question/answer pair used for FAQPage structured data.
// It is also used by the hand-written /faq page (see faqData).
type QA struct{ Q, A string }

// mdSection is one heading (H2/H3) of a markdown body together with the
// plain-text content that follows it, up to the next heading.
type mdSection struct {
	Level   int
	Heading string
	Text    string
	// Raw is the section body before markdown stripping, with line breaks
	// preserved. Needed to parse line-oriented structures such as hand-written
	// Q/A lists.
	Raw string
}

var (
	// Matches level 2 and level 3 ATX headings only (H1 is the page title,
	// H4+ are usually minor details not worth a Q&A entry).
	mdHeadingRe = regexp.MustCompile(`^(#{2,3})[ \t]+(.+?)[ \t]*#*[ \t]*$`)
	mdLinkRe    = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdStripRe   = regexp.MustCompile("[*_`]+")
	// Heading enumeration prefixes such as "一、" / "1." / "第一步：" carry no
	// meaning once the heading becomes a question, so they are stripped.
	stepOrdinalRe = regexp.MustCompile(`^第\s*[0-9一二三四五六七八九十]+\s*[步节部分]\s*[:：.、]?\s*`)
	ordinalRe     = regexp.MustCompile(`^(?:[0-9]+|[一二三四五六七八九十]+)\s*[、.．)）:：]\s*`)
)

// mdSections splits a markdown body into H2/H3 sections. Content before the
// first H2/H3 heading is ignored (it is normally just the duplicate H1 title).
func mdSections(body string) []mdSection {
	out := make([]mdSection, 0, 8)
	var cur *mdSection
	var buf []string

	flush := func() {
		if cur == nil {
			return
		}
		cur.Raw = strings.Join(buf, "\n")
		cur.Text = plainMDText(cur.Raw)
		out = append(out, *cur)
		cur = nil
		buf = nil
	}

	for _, line := range strings.Split(body, "\n") {
		if m := mdHeadingRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &mdSection{Level: len(m[1]), Heading: plainMDText(m[2])}
			continue
		}
		if cur != nil {
			buf = append(buf, line)
		}
	}
	flush()
	return out
}

// plainMDText reduces markdown syntax to readable text for structured data:
// links become their label, emphasis/backticks are dropped, whitespace collapses.
func plainMDText(s string) string {
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = mdStripRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ">"))
	return strings.Join(strings.Fields(s), " ")
}

// toQuestion turns a section heading into a question form suitable for
// FAQPage. Enumeration prefixes ("一、", "1.", "第一步：") are dropped and
// headings that already read as questions are kept verbatim.
func toQuestion(heading string) string {
	heading = strings.TrimSpace(heading)
	heading = stepOrdinalRe.ReplaceAllString(heading, "")
	heading = ordinalRe.ReplaceAllString(heading, "")
	heading = strings.TrimRight(strings.TrimSpace(heading), "？?。.：:")
	if heading == "" {
		return ""
	}
	return heading + "？"
}

// isFAQHeading reports whether a section heading marks a FAQ block. Such
// sections are handled by explicitFAQ (or skipped) instead of being fed to the
// generic heading-to-question conversion, which would produce a nonsense
// "常见问题？" entry.
func isFAQHeading(heading string) bool {
	lower := strings.ToLower(heading)
	return strings.Contains(heading, "常见问题") ||
		strings.Contains(heading, "问答") ||
		strings.Contains(lower, "faq") ||
		strings.Contains(lower, "q&a")
}

var (
	qLineRe = regexp.MustCompile(`^\s*\**\s*(?:Q|问|问题)\s*[：:]\s*\**\s*(.*)$`)
	aLineRe = regexp.MustCompile(`^\s*\**\s*(?:A|答|答案)\s*[：:]\s*\**\s*(.*)$`)
)

// explicitFAQ parses hand-written Q&A pairs out of a "常见问题 FAQ" section.
// Blog posts use the convention "**Q：question**" followed by "A：answer", so
// these curated pairs are preferred over questions auto-derived from headings.
func explicitFAQ(body string) []QA {
	for _, s := range mdSections(body) {
		if !isFAQHeading(s.Heading) || s.Raw == "" {
			continue
		}
		pairs := make([]QA, 0, 6)
		cur := -1
		for _, line := range strings.Split(s.Raw, "\n") {
			if m := qLineRe.FindStringSubmatch(line); m != nil {
				q := toQuestion(plainMDText(m[1]))
				if q == "" {
					continue
				}
				pairs = append(pairs, QA{Q: q})
				cur = len(pairs) - 1
				continue
			}
			if cur < 0 {
				continue
			}
			ans := ""
			if m := aLineRe.FindStringSubmatch(line); m != nil {
				ans = plainMDText(m[1])
			} else if pairs[cur].A != "" {
				// Continuation of the previous answer paragraph.
				ans = plainMDText(line)
			}
			if ans == "" {
				continue
			}
			if pairs[cur].A == "" {
				pairs[cur].A = ans
			} else {
				pairs[cur].A += " " + ans
			}
		}
		// Keep only complete pairs.
		complete := make([]QA, 0, len(pairs))
		for _, p := range pairs {
			if p.Q != "" && len([]rune(p.A)) >= 8 {
				complete = append(complete, QA{Q: p.Q, A: truncate(p.A, 300)})
			}
		}
		if len(complete) >= 2 {
			return complete
		}
	}
	return nil
}

// buildFAQFromBody derives FAQPage Q&A pairs for a post. Hand-written
// "常见问题" sections win; otherwise the article's own H2/H3 sections are
// converted into questions. Everything emitted is already visible on the page,
// which is what search engines require of FAQPage markup.
func buildFAQFromBody(body string, max int) []QA {
	if max <= 0 {
		return nil
	}
	if qa := explicitFAQ(body); len(qa) > 0 {
		if len(qa) > max {
			qa = qa[:max]
		}
		return qa
	}

	sections := mdSections(body)
	pick := func(level int) []QA {
		qa := make([]QA, 0, len(sections))
		for _, s := range sections {
			if s.Level != level || s.Heading == "" || isFAQHeading(s.Heading) {
				continue
			}
			text := s.Text
			if text == "" {
				continue
			}
			if len([]rune(text)) < 12 { // too short to be a useful answer
				continue
			}
			q := toQuestion(s.Heading)
			if q == "" {
				continue
			}
			qa = append(qa, QA{Q: q, A: truncate(text, 260)})
			if len(qa) >= max {
				break
			}
		}
		return qa
	}

	qa := pick(2)
	if len(qa) < 2 {
		qa = pick(3)
	}
	if len(qa) < 2 { // one lone Q&A is not a FAQ page
		return nil
	}
	return qa
}

// isTutorialPost reports whether a post should carry HowTo structured data.
func isTutorialPost(post *blog.Post) bool {
	if post == nil {
		return false
	}
	hay := post.Category + " " + post.Title + " " + post.Slug + " " + strings.Join(post.Tags, " ")
	for _, kw := range []string{"教程", "指南", "入门", "tutorial", "guide", "how-to", "howto"} {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

// howToJSONLD emits HowTo structured data whose steps mirror the article's
// H2 sections in order. Returns ("", false) when the body has fewer than two
// usable steps, since an incomplete HowTo is worse than none.
func howToJSONLD(post *blog.Post, public string) (js string, ok bool) {
	if post == nil || !isTutorialPost(post) {
		return "", false
	}
	steps := make([]map[string]interface{}, 0, 8)
	for _, s := range mdSections(post.Body) {
		if s.Level != 2 || s.Heading == "" || s.Text == "" || isFAQHeading(s.Heading) {
			continue
		}
		steps = append(steps, map[string]interface{}{
			"@type":    "HowToStep",
			"position": len(steps) + 1,
			"name":     s.Heading,
			"text":     truncate(s.Text, 300),
		})
	}
	if len(steps) < 2 {
		return "", false
	}
	m := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "HowTo",
		"name":          post.Title,
		"description":   post.Description,
		"datePublished": post.Date.Format("2006-01-02T15:04:05Z07:00"),
		"inLanguage":    "zh-CN",
		"step":          steps,
	}
	return string(mustJSON(m)), true
}
