package blog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Language codes used across the store, the SSR layer and the sitemaps.
const (
	LangZH = "zh"
	LangEN = "en"
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
	Lang        string    // LangZH or LangEN — which directory the file came from
}

// Store holds all loaded blog posts, sorted by date descending.
//
// Both language trees live in one store so that a slug can be resolved across
// languages (hreflang needs to know whether the counterpart exists) and so the
// sitemap can be built without loading the directories twice.
type Store struct {
	posts []Post
	// byKey is indexed "lang\x00slug". A single bySlug map would let an English
	// file silently shadow the Chinese post with the same slug (the two trees
	// intentionally share slugs), which would break every existing /blog URL.
	byKey map[string]*Post
}

const keySep = "\x00"

func storeKey(lang, slug string) string { return lang + keySep + slug }

// Load reads all .md files from one directory as Chinese content.
// Kept for callers/tests that have a single tree.
func Load(dir string) (*Store, error) { return LoadDirs(dir, "") }

// LoadDirs reads the Chinese tree plus an optional English tree.
//
// The English directory is optional by design: the schema ships before the
// content does (M1 = architecture, M2 = the 50 localized articles). A missing
// or empty dir yields no English posts and must NOT fail startup — an absent
// translation tree is a content problem, not a server problem.
func LoadDirs(dirZH, dirEN string) (*Store, error) {
	s := &Store{byKey: make(map[string]*Post)}

	if err := s.loadDir(dirZH, LangZH); err != nil {
		return nil, err
	}
	if dirEN != "" {
		if err := s.loadDir(dirEN, LangEN); err != nil {
			return nil, err
		}
	}

	// Sort by date descending (newest first).
	// Tie-break on lang then slug so the order is fully deterministic: paginating
	// a date-sorted feed is only correct if equal-date posts never swap places
	// between two requests (otherwise an item could appear on two pages or
	// vanish entirely). ReadDir already returns filename order, and slug ==
	// filename minus ".md", so this preserves today's ordering exactly.
	sort.SliceStable(s.posts, func(i, j int) bool {
		if !s.posts[i].Date.Equal(s.posts[j].Date) {
			return s.posts[i].Date.After(s.posts[j].Date)
		}
		if s.posts[i].Lang != s.posts[j].Lang {
			return s.posts[i].Lang < s.posts[j].Lang
		}
		return s.posts[i].Slug < s.posts[j].Slug
	})

	for i := range s.posts {
		p := &s.posts[i]
		s.byKey[storeKey(p.Lang, p.Slug)] = p
	}

	return s, nil
}

func (s *Store) loadDir(dir, lang string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("blog Load: %w", err)
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
		post.Lang = lang
		s.posts = append(s.posts, *post)
	}
	return nil
}

// All returns every Chinese post sorted by date descending — the historical
// meaning of this method. Use AllByLang for anything language-aware.
func (s *Store) All() []Post { return s.AllByLang(LangZH) }

// AllByLang returns the posts of one language, newest first.
func (s *Store) AllByLang(lang string) []Post {
	out := make([]Post, 0, len(s.posts))
	for _, p := range s.posts {
		if p.Lang == lang {
			out = append(out, p)
		}
	}
	return out
}

// CountByLang returns how many posts exist in a language tree.
func (s *Store) CountByLang(lang string) int {
	n := 0
	for _, p := range s.posts {
		if p.Lang == lang {
			n++
		}
	}
	return n
}

// Get returns the Chinese post for a slug, or nil.
func (s *Store) Get(slug string) *Post { return s.GetByLang(slug, LangZH) }

// GetByLang returns the post for (slug, lang).
//
// Fallback rule (deliberate): when an English translation is missing we return
// the Chinese post rather than nil, so /en/blog/<slug> renders instead of
// 404ing. Orphan 404s on a translated site waste crawl budget and show up in
// Search Console as soft errors. Callers must check the returned post's Lang to
// decide canonical/hreflang — a fallback render must NOT claim to be English.
func (s *Store) GetByLang(slug, lang string) *Post {
	if p, ok := s.byKey[storeKey(lang, slug)]; ok {
		return p
	}
	if lang != LangZH {
		if p, ok := s.byKey[storeKey(LangZH, slug)]; ok {
			return p
		}
	}
	return nil
}

// HasLang reports whether a real (non-fallback) translation exists, which is
// what decides whether an hreflang pair may be emitted. Claiming a language
// version that serves the other language's text is worse than staying silent.
func (s *Store) HasLang(slug, lang string) bool {
	_, ok := s.byKey[storeKey(lang, slug)]
	return ok
}

// PrevNext returns the previous and next Chinese posts (by date) relative to
// the given slug. "Previous" = newer post, "Next" = older post (matching blog
// reading order).
func (s *Store) PrevNext(slug string) (prev, next *Post) {
	return s.PrevNextLang(slug, LangZH)
}

// PrevNextLang is PrevNext within one language tree: navigating from an English
// article must not jump the reader into a Chinese one.
func (s *Store) PrevNextLang(slug, lang string) (prev, next *Post) {
	posts := s.AllByLang(lang)
	for i, p := range posts {
		if p.Slug != slug {
			continue
		}
		// posts are sorted newest-first, so index i-1 is newer (prev), i+1 is older (next)
		if i > 0 {
			prev = &posts[i-1]
		}
		if i < len(posts)-1 {
			next = &posts[i+1]
		}
		return
	}
	return nil, nil
}

// Categories returns unique Chinese category names.
func (s *Store) Categories() []string { return s.CategoriesLang(LangZH) }

// CategoriesLang returns unique category names in one language tree.
func (s *Store) CategoriesLang(lang string) []string {
	seen := make(map[string]bool)
	var cats []string
	for _, p := range s.AllByLang(lang) {
		if p.Category != "" && !seen[p.Category] {
			seen[p.Category] = true
			cats = append(cats, p.Category)
		}
	}
	return cats
}

// Tags returns unique Chinese tag names.
func (s *Store) Tags() []string { return s.TagsLang(LangZH) }

// TagsLang returns unique tag names in one language tree.
func (s *Store) TagsLang(lang string) []string {
	seen := make(map[string]bool)
	var tags []string
	for _, p := range s.AllByLang(lang) {
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

// Filter returns Chinese posts for the given category or tag (category wins when
// both are set), falling back to every post.
func (s *Store) Filter(cat, tag string) []Post { return s.FilterLang(LangZH, cat, tag) }

// FilterLang is the language-aware Filter, shared by the JSON API and the SSR
// handler so the two layers can never disagree about which posts belong to a
// filtered listing.
func (s *Store) FilterLang(lang, cat, tag string) []Post {
	switch {
	case cat != "":
		return s.FilterByCategoryLang(lang, cat)
	case tag != "":
		return s.FilterByTagLang(lang, tag)
	default:
		return s.AllByLang(lang)
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

// FilterByCategory returns Chinese posts matching the given category.
func (s *Store) FilterByCategory(cat string) []Post {
	return s.FilterByCategoryLang(LangZH, cat)
}

// FilterByCategoryLang returns posts of one language matching the category.
func (s *Store) FilterByCategoryLang(lang, cat string) []Post {
	if cat == "" {
		return s.AllByLang(lang)
	}
	var result []Post
	for _, p := range s.AllByLang(lang) {
		if p.Category == cat {
			result = append(result, p)
		}
	}
	return result
}

// FilterByTag returns Chinese posts matching the given tag.
func (s *Store) FilterByTag(tag string) []Post { return s.FilterByTagLang(LangZH, tag) }

// FilterByTagLang returns posts of one language matching the tag.
func (s *Store) FilterByTagLang(lang, tag string) []Post {
	if tag == "" {
		return s.AllByLang(lang)
	}
	var result []Post
	for _, p := range s.AllByLang(lang) {
		for _, t := range p.Tags {
			if t == tag {
				result = append(result, p)
				break
			}
		}
	}
	return result
}

// Related returns up to `limit` Chinese posts related to the given slug.
func (s *Store) Related(slug string, limit int) []Post {
	return s.RelatedLang(slug, LangZH, limit)
}

// RelatedLang returns up to `limit` posts related to (slug, lang), prioritizing
// same category, then shared tags, then recency. Excludes the current post.
func (s *Store) RelatedLang(slug, lang string, limit int) []Post {
	current := s.GetByLang(slug, lang)
	if current == nil || current.Lang != lang {
		// A fallback render has no same-language neighbours to relate to.
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
	currentTags := make(map[string]bool, len(current.Tags))
	for _, t := range current.Tags {
		currentTags[t] = true
	}
	for _, p := range s.AllByLang(lang) {
		if p.Slug == slug {
			continue
		}
		score := 0
		if p.Category == current.Category && p.Category != "" {
			score += 3
		}
		for _, t := range p.Tags {
			if currentTags[t] {
				score += 1
			}
		}
		if score > 0 {
			candidates = append(candidates, scored{post: p, score: score})
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
		for _, p := range s.AllByLang(lang) {
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
