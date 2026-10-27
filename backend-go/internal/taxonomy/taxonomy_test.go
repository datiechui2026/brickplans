package taxonomy

import "testing"

// The slug table is a contract: these strings are in the sitemap, in the
// breadcrumbs of 2,134 detail pages and in the hreflang cluster. A silent
// "improvement" here breaks internal links site-wide, so pin them.
func TestAggregateEntryContract(t *testing.T) {
	want := map[Family][]string{
		Category:   {"scene", "vehicle", "fantasy", "mecha", "architecture", "scifi"},
		Brand:      {"qiji", "lego"},
		Difficulty: {"1", "2", "3", "4", "5"},
	}
	for fam, slugs := range want {
		got := Slugs(fam)
		if len(got) != len(slugs) {
			t.Fatalf("%s: %d slugs, want %d (%v)", fam, len(got), len(slugs), got)
		}
		for i := range slugs {
			if got[i] != slugs[i] {
				t.Fatalf("%s slug[%d] = %q, want %q", fam, i, got[i], slugs[i])
			}
		}
	}
}

func TestLookupAndForValueRoundTrip(t *testing.T) {
	for _, e := range AggregateEntries {
		byPath := Path(e)
		if byPath != "/"+string(e.Family)+"/"+e.Slug {
			t.Fatalf("Path(%v) = %q", e, byPath)
		}
		if got, ok := Lookup(e.Family, e.Slug); !ok || got.Value != e.Value {
			t.Fatalf("Lookup(%s,%s) = %v,%v", e.Family, e.Slug, got, ok)
		}
		if got, ok := ForValue(e.Family, e.Value); !ok || got.Slug != e.Slug {
			t.Fatalf("ForValue(%s,%s) = %v,%v", e.Family, e.Value, got, ok)
		}
	}
	// A stored value without a landing page must not resolve — detail
	// breadcrumbs rely on false to render plain text instead of a dead link.
	if _, ok := ForValue(Category, "星战"); ok {
		t.Fatal("星战 resolved to a landing page that does not exist")
	}
	if _, ok := Lookup(Category, "scenes"); ok {
		t.Fatal("unknown slug resolved")
	}
}
