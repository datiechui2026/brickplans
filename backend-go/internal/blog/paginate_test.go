package blog

import (
	"testing"
	"time"
)

func mkPosts(n int) []Post {
	base := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	posts := make([]Post, 0, n)
	for i := 0; i < n; i++ {
		posts = append(posts, Post{
			Slug: "post-" + itoa(i),
			Date: base,
		})
	}
	return posts
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}

func TestPaginate(t *testing.T) {
	posts := mkPosts(1533)

	cases := []struct {
		name      string
		page      int
		size      int
		wantLen   int
		wantTotal int
		wantFirst string
	}{
		{"first page default", 1, DefaultPageSize, DefaultPageSize, 1533, "post-0"},
		{"second page", 2, DefaultPageSize, DefaultPageSize, 1533, "post-24"},
		{"last partial page", 64, DefaultPageSize, 21, 1533, "post-1512"},
		{"past the end is empty not nil", 65, DefaultPageSize, 0, 1533, ""},
		{"page below 1 clamps to 1", 0, 10, 10, 1533, "post-0"},
		{"negative page clamps to 1", -7, 10, 10, 1533, "post-0"},
		{"size above max is capped", 1, 9999, MaxPageSize, 1533, "post-0"},
		{"size zero falls back to default", 1, 0, DefaultPageSize, 1533, "post-0"},
		{"size negative falls back", 1, -3, DefaultPageSize, 1533, "post-0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, total := Paginate(posts, tc.page, tc.size)
			if total != tc.wantTotal {
				t.Fatalf("total = %d, want %d", total, tc.wantTotal)
			}
			if len(items) != tc.wantLen {
				t.Fatalf("len(items) = %d, want %d", len(items), tc.wantLen)
			}
			if items == nil {
				t.Fatal("items is nil; JSON would encode null instead of []")
			}
			if tc.wantFirst != "" && items[0].Slug != tc.wantFirst {
				t.Fatalf("items[0].Slug = %q, want %q", items[0].Slug, tc.wantFirst)
			}
		})
	}
}

func TestPaginateEmptyCorpus(t *testing.T) {
	items, total := Paginate(nil, 1, 10)
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if items == nil || len(items) != 0 {
		t.Fatalf("items = %#v, want empty non-nil slice", items)
	}
}

// Pages must not overlap or skip: concatenating every page reproduces the input
// exactly once. This is the invariant that makes the pager safe for crawlers.
func TestPaginateCoversCorpusExactlyOnce(t *testing.T) {
	posts := mkPosts(1533)
	seen := make(map[string]int)
	for page := 1; page <= PageCount(1533, DefaultPageSize); page++ {
		items, _ := Paginate(posts, page, DefaultPageSize)
		for _, p := range items {
			seen[p.Slug]++
		}
	}
	if len(seen) != 1533 {
		t.Fatalf("distinct slugs = %d, want 1533", len(seen))
	}
	for slug, n := range seen {
		if n != 1 {
			t.Fatalf("slug %q appeared %d times, want exactly 1", slug, n)
		}
	}
}

func TestPageCount(t *testing.T) {
	cases := []struct{ total, size, want int }{
		{1533, 24, 64},
		{1533, 50, 31},
		{48, 50, 1},
		{50, 50, 1},
		{51, 50, 2},
		{0, 50, 1},   // empty listing still reports one page
		{-5, 50, 1},  // defensive
		{10, 0, 1},   // size falls back to default (10 items -> 1 page)
		{100, 24, 5}, // exact-ish
	}
	for _, tc := range cases {
		if got := PageCount(tc.total, tc.size); got != tc.want {
			t.Fatalf("PageCount(%d, %d) = %d, want %d", tc.total, tc.size, got, tc.want)
		}
	}
}

func TestNormPageSize(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, DefaultPageSize},
		{-1, DefaultPageSize},
		{1, 1},
		{24, 24},
		{MaxPageSize, MaxPageSize},
		{MaxPageSize + 1, MaxPageSize},
		{99999, MaxPageSize},
	}
	for _, tc := range cases {
		if got := NormPageSize(tc.in); got != tc.want {
			t.Fatalf("NormPageSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// An uncapped size must not produce a response that advertises one page_size
// while capping the item count — PageCount has to agree with Paginate.
func TestPageCountAgreesWithPaginateForOversizedSize(t *testing.T) {
	posts := mkPosts(1533)
	items, total := Paginate(posts, 1, 99999)
	pages := PageCount(total, 99999)
	if len(items) != MaxPageSize {
		t.Fatalf("len(items) = %d, want %d", len(items), MaxPageSize)
	}
	if pages != PageCount(total, MaxPageSize) {
		t.Fatalf("PageCount(total, 99999) = %d, want %d", pages, PageCount(total, MaxPageSize))
	}
}

func TestFilter(t *testing.T) {
	posts := []Post{
		{Slug: "a", Category: "MOC评测", Tags: []string{"MOC"}},
		{Slug: "b", Category: "积木教程", Tags: []string{"入门"}},
		{Slug: "c", Category: "积木教程", Tags: []string{"MOC", "入门"}},
	}
	s := &Store{posts: posts, bySlug: map[string]*Post{}}

	if got := len(s.Filter("", "")); got != 3 {
		t.Fatalf("Filter(all) len = %d, want 3", got)
	}
	if got := len(s.Filter("积木教程", "")); got != 2 {
		t.Fatalf("Filter(category) len = %d, want 2", got)
	}
	if got := len(s.Filter("", "MOC")); got != 2 {
		t.Fatalf("Filter(tag) len = %d, want 2", got)
	}
	// Category wins when both are present — matches the historical handler logic.
	if got := len(s.Filter("MOC评测", "入门")); got != 1 {
		t.Fatalf("Filter(category+tag) len = %d, want 1", got)
	}
}
