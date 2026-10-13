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

	// Sort by date descending (newest first).
	// Tie-break on slug so the order is fully deterministic: paginating a
	// date-sorted feed is only correct if equal-date posts never swap places
	// between two requests (otherwise an item could appear on two pages or
	// vanish entirely). ReadDir already returns filename order, and slug ==
	// filename minus ".md", so this preserves today's ordering exactly.
	sort.SliceStable(s.posts, func(i, j int) bool {
		if !s.posts[i].Date.Equal(s.posts[j].Date) {
			return s.posts[i].Date.After(s.posts[j].Date)
		}
		return s.posts[i].Slug < s.posts[j].Slug
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

// Pagination defaults shared by the JSON API and the SSR blog list page.
const (
	// DefaultPageSize is the page size for GET /api/blog when ?page_size= is absent.
	DefaultPageSize = 24
	// MaxPageSize caps ?page_size= so a caller cannot ask for the whole corpus.
	MaxPageSize = 100
	// SSRPageSize is the number of links rendered into the crawlable
	// (server-rendered) blog index page.
	SSRPageSize = 50
)

// Filter returns posts for the given category or tag (category wins when both
// are set), falling back to every post. Shared by the JSON API and the SSR
// handler so the two layers can never disagree about which posts belong to a
// filtered listing.
func (s *Store) Filter(cat, tag string) []Post {
	switch {
	case cat != "":
		return s.FilterByCategory(cat)
	case tag != "":
		return s.FilterByTag(tag)
	default:
		return s.posts
	}
}

// NormPageSize resolves a requested page size to the value that will actually
// be used: non-positive falls back to DefaultPageSize, oversized is capped at
// MaxPageSize. Callers must report THIS value, not the raw request, or the
// response advertises a page size it did not honour.
func NormPageSize(size int) int {
	if size <= 0 {
		return DefaultPageSize
	}
	if size > MaxPageSize {
		return MaxPageSize
	}
	return size
}

// Paginate slices posts into a 1-based page. page < 1 is clamped to 1; a page
// past the end yields an empty slice (never nil, so JSON encodes [] not null).
// size is resolved through NormPageSize. Returns the page items and the total
// number of posts before slicing.
func Paginate(posts []Post, page, size int) (items []Post, total int) {
	total = len(posts)
	size = NormPageSize(size)
	if page < 1 {
		page = 1
	}
	start := (page - 1) * size
	if start >= total {
		return []Post{}, total
	}
	end := start + size
	if end > total {
		end = total
	}
	return posts[start:end], total
}

// PageCount returns the number of pages needed to hold total items at size per
// page. size is resolved through NormPageSize so it agrees with Paginate.
// Always >= 1 so an empty listing still reports one (empty) page.
func PageCount(total, size int) int {
	size = NormPageSize(size)
	if total <= 0 {
		return 1
	}
	return (total + size - 1) / size
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

// Related returns up to `limit` posts related to the given slug, prioritizing
// same category, then shared tags, then recency. Excludes the current post.
func (s *Store) Related(slug string, limit int) []Post {
	current := s.bySlug[slug]
	if current == nil {
		return nil
	}
	if limit <= 0 {
		limit = 3
	}

	// Score each candidate: same category = 3pts, each shared tag = 1pt
	type scored struct {
		post  Post
		score int
	}
	var candidates []scored
	for i := range s.posts {
		p := &s.posts[i]
		if p.Slug == slug {
			continue
		}
		score := 0
		if p.Category == current.Category && p.Category != "" {
			score += 3
		}
		// Check shared tags
		currentTags := make(map[string]bool)
		for _, t := range current.Tags {
			currentTags[t] = true
		}
		for _, t := range p.Tags {
			if currentTags[t] {
				score += 1
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{post: *p, score: score})
		}
	}

	// Sort by score descending (posts already sorted by date desc, stable preserves that for ties)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// If not enough scored candidates, fill with recent posts
	result := make([]Post, 0, limit)
	seen := make(map[string]bool)
	for _, c := range candidates {
		if len(result) >= limit {
			break
		}
		result = append(result, c.post)
		seen[c.post.Slug] = true
	}
	if len(result) < limit {
		for _, p := range s.posts {
			if len(result) >= limit {
				break
			}
			if p.Slug == slug || seen[p.Slug] {
				continue
			}
			result = append(result, p)
			seen[p.Slug] = true
		}
	}

	return result
}
