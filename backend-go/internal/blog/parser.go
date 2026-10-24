package blog

import (
	"fmt"
	"strings"
	"time"
)

// parsePost parses a markdown file with YAML frontmatter.
// Expected format:
//   ---
//   title: "..."
//   description: "..."
//   date: 2026-07-20
//   author: "..."
//   category: "..."
//   tags: [tag1, tag2, tag3]
//   ---
//   # Markdown body...
func parsePost(slug, raw string) (*Post, error) {
	post := &Post{Slug: slug}

	// Split frontmatter and body. Both LF and CRLF are accepted: content files
	// are LF in git, but a Windows checkout (core.autocrlf) turns them into
	// CRLF, and a parser that only recognises "---\n" would silently load zero
	// posts there (every file fails the missing-title check).
	var frontmatter, body string
	prefix := ""
	if strings.HasPrefix(raw, "---\r\n") {
		prefix = "---\r\n"
	} else if strings.HasPrefix(raw, "---\n") {
		prefix = "---\n"
	}
	if prefix != "" {
		rest := raw[len(prefix):]
		// The closing delimiter is a line containing only "---".
		if end := strings.Index(rest, "\n---"); end >= 0 {
			frontmatter = rest[:end]
			bodyStart := len(prefix) + end + 4 // just past "\n---"
			if bodyStart < len(raw) && raw[bodyStart] == '\r' {
				bodyStart++
			}
			if bodyStart < len(raw) && raw[bodyStart] == '\n' {
				bodyStart++
			}
			body = raw[bodyStart:]
		} else {
			body = raw
		}
	} else {
		body = raw
	}

	// Parse frontmatter (simple key: value parser, not full YAML)
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		// Strip surrounding quotes
		val = strings.Trim(val, "\"'")

		switch key {
		case "title":
			post.Title = val
		case "description":
			post.Description = val
		case "date":
			// Try multiple date formats
			layouts := []string{"2006-01-02", "2006-01-02 15:04:05", "2006/01/02"}
			for _, layout := range layouts {
				if t, err := time.Parse(layout, val); err == nil {
					post.Date = t
					break
				}
			}
		case "author":
			post.Author = val
		case "category":
			post.Category = val
		case "tags":
			// Parse [tag1, tag2, tag3] format. Whitespace must be stripped
			// BEFORE the quote trim: `'brickplan'` after a comma is
			// " 'brickplan'", and trimming quotes on the raw field leaves the
			// leading apostrophe behind (producing tags like `'brickplan` and
			// garbage tag URLs/keywords).
			val = strings.Trim(val, "[]")
			for _, tag := range strings.Split(val, ",") {
				tag = strings.Trim(strings.TrimSpace(tag), "\"'")
				if tag != "" {
					post.Tags = append(post.Tags, tag)
				}
			}
		}
	}

	// Strip leading blank lines from body
	body = strings.TrimLeft(body, "\n\r")

	post.Body = body

	if post.Title == "" {
		return nil, fmt.Errorf("post %s: missing title", slug)
	}

	return post, nil
}
