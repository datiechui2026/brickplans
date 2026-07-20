package blog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Post represents a single blog post parsed from a markdown file.
type Post struct {
	Slug        string    // URL slug (derived from filename)
	Title       string    // From YAML frontmatter
	Description string    // From YAML frontmatter
	Date        time.Time // From YAML frontmatter
	Author      string    // From YAML frontmatter
	Category    string    // From YAML frontmatter
	Tags        []string  // From YAML frontmatter
	Body        string    // Markdown body (after frontmatter)
}

// Store holds all loaded blog posts, sorted by date descending.
type Store struct {
	posts []Post
	bySlug map[string]*Post
}

// Load reads all .md files from the given directory and parses them.
// Returns an empty store (not nil) if the directory doesn't exist.
func Load(dir string) (*Store, error) {
	s := &Store{bySlug: make(map[string]*Post)}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("blog Load: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".md")
		post, err := parsePost(slug, string(raw))
		if err != nil {
			continue
		}
		s.posts = append(s.posts, *post)
	}

	// Sort by date descending (newest first)
	sort.Slice(s.posts, func(i, j int) bool {
		return s.posts[i].Date.After(s.posts[j].Date)
	})

	// Build slug index
	for i := range s.posts {
		s.bySlug[s.posts[i].Slug] = &s.posts[i]
	}

	return s, nil
}

// All returns all posts sorted by date descending.
func (s *Store) All() []Post {
	return s.posts
}

// Get returns a post by slug, or nil if not found.
func (s *Store) Get(slug string) *Post {
	return s.bySlug[slug]
}

// PrevNext returns the previous and next posts (by date) relative to the given slug.
// "Previous" = newer post, "Next" = older post (matching blog reading order).
func (s *Store) PrevNext(slug string) (prev, next *Post) {
	for i, p := range s.posts {
		if p.Slug != slug {
			continue
		}
		// posts are sorted newest-first, so index i-1 is newer (prev), i+1 is older (next)
		if i > 0 {
			prev = &s.posts[i-1]
		}
		if i < len(s.posts)-1 {
			next = &s.posts[i+1]
		}
		return
	}
	return nil, nil
}

// Categories returns unique category names.
func (s *Store) Categories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, p := range s.posts {
		if p.Category != "" && !seen[p.Category] {
			seen[p.Category] = true
			cats = append(cats, p.Category)
		}
	}
	return cats
}

// Tags returns unique tag names.
func (s *Store) Tags() []string {
	seen := make(map[string]bool)
	var tags []string
	for _, p := range s.posts {
		for _, t := range p.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	return tags
}

// FilterByCategory returns posts matching the given category.
func (s *Store) FilterByCategory(cat string) []Post {
	if cat == "" {
		return s.posts
	}
	var result []Post
	for _, p := range s.posts {
		if p.Category == cat {
			result = append(result, p)
		}
	}
	return result
}

// FilterByTag returns posts matching the given tag.
func (s *Store) FilterByTag(tag string) []Post {
	if tag == "" {
		return s.posts
	}
	var result []Post
	for _, p := range s.posts {
		for _, t := range p.Tags {
			if t == tag {
				result = append(result, p)
				break
			}
		}
	}
	return result
}
