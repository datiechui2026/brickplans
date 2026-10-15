package blog

import (
	"sort"
	"strings"
)

// Category buckets
//
// The split sitemap groups every published post into exactly one bucket so a
// crawler — and Search Console's per-sitemap coverage report — can be read per
// topic. The 9 canonical buckets below are the ones the content pipeline is
// meant to use, but the ~1500 legacy posts carry 50+ near-duplicate category
// strings ("积木教程" vs "教程", "品牌评测" vs "品牌推荐", "MOC 教程" with a
// space, ...). `raws` therefore maps every string we actually ship onto its
// canonical bucket.
//
// Two invariants, both enforced by TestBucketPartitionIsTotal:
//   - mapping is total: a post whose categories appear nowhere in this table
//     still lands in OtherBucketSlug, so no post can silently drop out of the
//     sitemap (a URL missing from every sitemap is a de-indexing risk, and it
//     is invisible in Search Console);
//   - mapping is a function: one raw category maps to exactly one bucket, so
//     the buckets partition the corpus and no URL is listed twice.
type CategoryBucket struct {
	Slug  string // ASCII slug used in the child sitemap filename
	Label string // canonical Chinese category name (docs + tests)
	raws  []string
}

// OtherBucketSlug is the catch-all bucket. Posts whose frontmatter category is
// empty or unknown end up here.
const OtherBucketSlug = "other"

var categoryBuckets = []CategoryBucket{
	{
		Slug:  "brand",
		Label: "品牌推荐",
		raws:  []string{"品牌推荐", "品牌评测", "品牌选购", "国产积木", "积木推荐", "选购指南", "Brand Guide", "Brand Review", "Buying Guide", "Brand Comparison",
			// M2 英文作者实际会写出的简写/变体（实测已有 "Brand" 落到 other）
			"Brand", "Brands", "Brand Ranking", "Brand Overview"},
	},
	{
		Slug:  "beginner",
		Label: "新手教程",
		raws: []string{
			"新手教程", "新手入门", "新手指南", "积木教程", "搭建教程", "教程",
			"拼装教程", "拼装指南", "实用指南", "指南", "亲子玩法",
			"Beginner Guide", "Getting Started", "Tutorial", "How To",
		},
	},
	{
		Slug:  "parts",
		Label: "零件知识",
		raws:  []string{"零件知识", "零件指南", "积木零件", "Parts Guide", "Brick Parts", "Parts Explained"},
	},
	{
		Slug:  "building",
		Label: "搭建技巧",
		raws:  []string{"搭建技巧", "拼装技巧", "收纳技巧", "积木搭建", "MOC搭建", "场景MOC", "场景搭建", "Building Tips", "Techniques", "Storage Tips",
			// 实测 M2 英文稿写的是 "Building Techniques"（表里原来只有 "Techniques"/"Building Tips"）→ 落到 other
			"Building Techniques", "Building Technique", "Building Skills"},
	},
	{
		Slug:  "set-review",
		Label: "积木评测",
		raws:  []string{"积木评测", "套装评测", "积木套装评测", "系列评测", "套装推荐", "评测", "Set Review", "Set Reviews", "Reviews"},
	},
	{
		Slug:  "moc-review",
		Label: "MOC图纸评测",
		raws:  []string{"MOC图纸评测", "图纸评测", "MOC图纸", "MOC评测", "MOC Review", "Blueprint Review", "Instructions Review"},
	},
	{
		Slug:  "moc-tutorial",
		Label: "MOC教程",
		raws:  []string{"MOC教程", "MOC 教程", "MOC入门", "MOC指南", "图纸指南", "图纸教程", "设计教程", "MOC场景指南", "MOC Tutorial", "MOC Guide", "Building Guide", "Instructions Guide"},
	},
	{
		Slug:  "knowledge",
		Label: "知识科普",
		raws:  []string{"知识科普", "MOC知识", "Knowledge", "Explained"},
	},
	{
		Slug:  "resources",
		Label: "资源推荐",
		raws:  []string{"资源推荐", "图纸下载", "MOC分享", "找图指南", "推荐", "Resources", "Downloads", "Where To Find"},
	},
	{
		Slug:  OtherBucketSlug,
		Label: "其他",
		// Empty: everything not listed above falls through to here.
	},
}

// rawToBucket is the flattened lookup. Duplicate raws across buckets would
// silently shadow each other (last wins) and break the partition, so bucket
// registration panics on a duplicate instead of shipping a wrong sitemap.
var rawToBucket = buildRawToBucket()

// normToBucket is rawToBucket keyed by lowercased category, so author-side
// casing drift ("brand guide", "SET REVIEW") does not silently fall through to
// `other`. The M2 English batch is authored per-article by an AI, so
// exact-string matching is not a safe assumption: of 6 hand-checked sample
// posts, 2 carried variants the table did not list and landed in `other`.
//
// Deliberately NO whitespace trimming: `" 品牌推荐"` staying unmapped is an
// existing, tested decision (see TestBucketForMapsLegacyCategories) — a stray
// space is a content bug to fix at the source, not to paper over here.
//
// Same invariant as rawToBucket: a normalized key that maps to two different
// buckets panics at startup rather than shipping an ambiguous sitemap.
var normToBucket = buildNormToBucket()

func buildNormToBucket() map[string]string {
	m := make(map[string]string, 64)
	for raw, slug := range rawToBucket {
		k := strings.ToLower(raw)
		if prev, dup := m[k]; dup && prev != slug {
			panic("blog: normalized category " + k + " mapped to both " + prev + " and " + slug)
		}
		m[k] = slug
	}
	return m
}

func buildRawToBucket() map[string]string {
	m := make(map[string]string, 64)
	for _, b := range categoryBuckets {
		for _, raw := range b.raws {
			if prev, dup := m[raw]; dup {
				panic("blog: category " + raw + " mapped to both " + prev + " and " + b.Slug)
			}
			m[raw] = b.Slug
		}
	}
	return m
}

// BucketSlugs returns the child-sitemap slugs in declaration order (canonical
// buckets first, catch-all last).
func BucketSlugs() []string {
	slugs := make([]string, 0, len(categoryBuckets))
	for _, b := range categoryBuckets {
		slugs = append(slugs, b.Slug)
	}
	return slugs
}

// BucketLabel returns the Chinese label for a bucket slug, or "" if the slug is
// not a known bucket (callers use that to 404 an unknown child sitemap).
func BucketLabel(slug string) string {
	for _, b := range categoryBuckets {
		if b.Slug == slug {
			return b.Label
		}
	}
	return ""
}

// BucketFor maps a raw frontmatter category to its bucket slug. Total: an empty
// or unknown category returns OtherBucketSlug, never "".
//
// Exact match first, then a lowercased match — the English tree is authored
// per-article by an AI, so "Brand Guide" vs "brand guide" must not decide
// whether a post is discoverable in its topic sitemap. Whitespace is not
// normalized (see normToBucket).
func BucketFor(raw string) string {
	if slug, ok := rawToBucket[raw]; ok {
		return slug
	}
	if slug, ok := normToBucket[strings.ToLower(raw)]; ok {
		return slug
	}
	return OtherBucketSlug
}

// PostsInBucket returns the Chinese posts in one bucket, preserving the store's
// newest-first order.
func (s *Store) PostsInBucket(slug string) []Post {
	return s.PostsInBucketLang(LangZH, slug)
}

// PostsInBucketLang returns the posts of one language in one bucket. The bucket
// table is shared by both trees: English posts carry English category strings
// (M2 authors them), and anything not in the table still lands in "other", so
// the zero-loss invariant holds for the English sitemap too.
func (s *Store) PostsInBucketLang(lang, slug string) []Post {
	var out []Post
	for _, p := range s.AllByLang(lang) {
		if BucketFor(p.Category) == slug {
			out = append(out, p)
		}
	}
	return out
}

// BucketStats returns, per bucket slug, the number of posts and the newest post
// date (RFC3339 date). Used by the sitemap index for <lastmod> and by tests to
// assert the partition is total. Slugs are returned in declaration order.
type BucketStat struct {
	Slug      string
	Label     string
	Count     int
	LatestDay string // "2006-01-02", "" when the bucket is empty
}

// Stats summarises every Chinese bucket in one pass.
func (s *Store) Stats() []BucketStat { return s.StatsLang(LangZH) }

// StatsLang summarises every bucket of one language in one pass.
func (s *Store) StatsLang(lang string) []BucketStat {
	stats := make([]BucketStat, 0, len(categoryBuckets))
	index := make(map[string]int, len(categoryBuckets))
	for _, b := range categoryBuckets {
		index[b.Slug] = len(stats)
		stats = append(stats, BucketStat{Slug: b.Slug, Label: b.Label})
	}
	for _, p := range s.AllByLang(lang) {
		i, ok := index[BucketFor(p.Category)]
		if !ok {
			continue // unreachable: BucketFor is total over known slugs
		}
		stats[i].Count++
		if day := p.Date.Format("2006-01-02"); day > stats[i].LatestDay {
			stats[i].LatestDay = day
		}
	}
	return stats
}

// RawCategories returns the distinct raw frontmatter categories actually
// present, sorted. Used by tests to prove the mapping table covers real data
// (an unmapped category is not an error, but it should be a deliberate choice,
// not an oversight).
func (s *Store) RawCategories() []string {
	seen := make(map[string]bool, 64)
	var out []string
	for _, p := range s.posts {
		if p.Category != "" && !seen[p.Category] {
			seen[p.Category] = true
			out = append(out, p.Category)
		}
	}
	sort.Strings(out)
	return out
}
