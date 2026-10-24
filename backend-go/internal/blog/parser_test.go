package blog

import (
	"strings"
	"testing"
)

// TestParseFrontmatterTags guards the quote/whitespace handling for inline list
// tags. The English corpus writes `tags: ['MOC', 'brickplan']`; trimming the
// quotes before the spaces left tags like `'brickplan`, which leaked into
// JSON-LD keywords and produced broken /blog?tag='brickplan filter URLs.
func TestParseFrontmatterTags(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{"quoted single-word tags", `tags: ['MOC', 'brickplan', 'treehousemocguide' ]`, []string{"MOC", "brickplan", "treehousemocguide"}},
		{"double quoted", `tags: ["MOC", "parts"]`, []string{"MOC", "parts"}},
		{"bare list", `tags: [MOC, 零件]`, []string{"MOC", "零件"}},
		{"no spaces", `tags: ['a','b']`, []string{"a", "b"}},
		{"empty", `tags: []`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := "---\ntitle: \"T\"\n" + tc.line + "\n---\n\nbody\n"
			post, err := parsePost("slug", raw)
			if err != nil {
				t.Fatalf("parsePost: %v", err)
			}
			if len(post.Tags) != len(tc.want) {
				t.Fatalf("tags = %#v, want %#v", post.Tags, tc.want)
			}
			for i, w := range tc.want {
				if post.Tags[i] != w {
					t.Fatalf("tags = %#v, want %#v", post.Tags, tc.want)
				}
			}
			for _, tag := range post.Tags {
				if strings.ContainsAny(tag, "'\"") {
					t.Fatalf("tag %q still carries a quote character", tag)
				}
			}
		})
	}
}
