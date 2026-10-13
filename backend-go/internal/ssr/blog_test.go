package ssr

import "testing"

func TestBlogListPath(t *testing.T) {
	cases := []struct {
		name string
		cat  string
		tag  string
		page int
		want string
	}{
		{"unfiltered first page", "", "", 1, "/blog"},
		{"unfiltered page 2 self-references the page", "", "", 2, "/blog?page=2"},
		{"category page 1", "积木教程", "", 1, "/blog?category=%E7%A7%AF%E6%9C%A8%E6%95%99%E7%A8%8B"},
		{"category page 2 keeps both", "积木教程", "", 2, "/blog?category=%E7%A7%AF%E6%9C%A8%E6%95%99%E7%A8%8B&page=2"},
		{"tag page 2 keeps both", "", "MOC", 2, "/blog?page=2&tag=MOC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := blogListPath(tc.cat, tc.tag, tc.page); got != tc.want {
				t.Fatalf("blogListPath(%q, %q, %d) = %q, want %q", tc.cat, tc.tag, tc.page, got, tc.want)
			}
		})
	}
}

func TestPagerWindow(t *testing.T) {
	t.Run("single page has no window links", func(t *testing.T) {
		got := pagerWindow(1, 1)
		if len(got) != 1 || got[0] != 1 {
			t.Fatalf("pagerWindow(1,1) = %v, want [1]", got)
		}
	})

	t.Run("first page shows first and last with a gap", func(t *testing.T) {
		got := pagerWindow(1, 31)
		want := []int{1, 2, 3, 0, 31}
		if !equalInts(got, want) {
			t.Fatalf("pagerWindow(1,31) = %v, want %v", got, want)
		}
	})

	t.Run("middle page has gaps on both sides", func(t *testing.T) {
		got := pagerWindow(15, 31)
		want := []int{1, 0, 13, 14, 15, 16, 17, 0, 31}
		if !equalInts(got, want) {
			t.Fatalf("pagerWindow(15,31) = %v, want %v", got, want)
		}
	})

	t.Run("last page", func(t *testing.T) {
		got := pagerWindow(31, 31)
		want := []int{1, 0, 29, 30, 31}
		if !equalInts(got, want) {
			t.Fatalf("pagerWindow(31,31) = %v, want %v", got, want)
		}
	})

	t.Run("no gaps when few pages", func(t *testing.T) {
		got := pagerWindow(2, 4)
		want := []int{1, 2, 3, 4}
		if !equalInts(got, want) {
			t.Fatalf("pagerWindow(2,4) = %v, want %v", got, want)
		}
	})

	t.Run("link count stays bounded", func(t *testing.T) {
		// A 1000-page blog must not emit anywhere near 1000 links.
		got := pagerWindow(500, 1000)
		if len(got) > 12 {
			t.Fatalf("pagerWindow(500,1000) emitted %d entries, want <= 12", len(got))
		}
	})
}

func TestAtoiOrSSR(t *testing.T) {
	cases := []struct {
		in   string
		def  int
		want int
	}{
		{"3", 1, 3},
		{"", 1, 1},
		{"abc", 1, 1},
		{"0", 1, 1},
		{"-2", 1, 1},
	}
	for _, tc := range cases {
		if got := atoiOrSSR(tc.in, tc.def); got != tc.want {
			t.Fatalf("atoiOrSSR(%q, %d) = %d, want %d", tc.in, tc.def, got, tc.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
