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

	// Split frontmatter and body
	var frontmatter, body string
	if strings.HasPrefix(raw, "---\n") {
		end := strings.Index(raw[4:], "\n---\n")
		if end < 0 {
			end = strings.Index(raw[4:], "\n---\r\n")
		}
		if end < 0 {
			// Maybe ends with --- at EOF without trailing newline
			end = strings.Index(raw[4:], "\n---")
		}
		if end >= 0 {
			frontmatter = raw[4 : 4+end]
			bodyStart := 4 + end + 4 // skip "\n---"
			if bodyStart < len(raw) && (raw[bodyStart] == '\n' || raw[bodyStart] == '\r') {
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
			// Parse [tag1, tag2, tag3] format
			val = strings.Trim(val, "[]")
			for _, tag := range strings.Split(val, ",") {
				tag = strings.TrimSpace(strings.Trim(tag, "\"'"))
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
