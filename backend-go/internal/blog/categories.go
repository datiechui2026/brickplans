package blog

import "sort"

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
		raws:  []string{"品牌推荐", "品牌评测", "品牌选购", "国产积木", "积木推荐", "选购指南"},
	},
	{
		Slug:  "beginner",
		Label: "新手教程",
		raws: []string{
			"新手教程", "新手入门", "新手指南", "积木教程", "搭建教程", "教程",
			"拼装教程", "拼装指南", "实用指南", "指南", "亲子玩法",
		},
	},
	{
		Slug:  "parts",
		Label: "零件知识",
		raws:  []string{"零件知识", "零件指南", "积木零件"},
	},
	{
		Slug:  "building",
		Label: "搭建技巧",
		raws:  []string{"搭建技巧", "拼装技巧", "收纳技巧", "积木搭建", "MOC搭建", "场景MOC", "场景搭建"},
	},
	{
		Slug:  "set-review",
		Label: "积木评测",
		raws:  []string{"积木评测", "套装评测", "积木套装评测", "系列评测", "套装推荐", "评测"},
	},
	{
		Slug:  "moc-review",
		Label: "MOC图纸评测",
		raws:  []string{"MOC图纸评测", "图纸评测", "MOC图纸", "MOC评测"},
	},
	{
		Slug:  "moc-tutorial",
		Label: "MOC教程",
		raws:  []string{"MOC教程", "MOC 教程", "MOC入门", "MOC指南", "图纸指南", "图纸教程", "设计教程", "MOC场景指南"},
	},
	{
		Slug:  "knowledge",
		Label: "知识科普",
		raws:  []string{"知识科普", "MOC知识"},
	},
	{
		Slug:  "resources",
		Label: "资源推荐",
		raws:  []string{"资源推荐", "图纸下载", "MOC分享", "找图指南", "推荐"},
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
func BucketFor(raw string) string {
	if slug, ok := rawToBucket[raw]; ok {
		return slug
	}
	return OtherBucketSlug
}

// PostsInBucket returns the posts in one bucket, preserving the store's
// newest-first order.
func (s *Store) PostsInBucket(slug string) []Post {
	var out []Post
	for _, p := range s.posts {
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

// Stats summarises every bucket in one pass.
func (s *Store) Stats() []BucketStat {
	stats := make([]BucketStat, 0, len(categoryBuckets))
	index := make(map[string]int, len(categoryBuckets))
	for _, b := range categoryBuckets {
		index[b.Slug] = len(stats)
		stats = append(stats, BucketStat{Slug: b.Slug, Label: b.Label})
	}
	for _, p := range s.posts {
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
