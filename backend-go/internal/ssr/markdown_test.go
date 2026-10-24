package ssr

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"brickplans/internal/blog"
)

// setTestImageDims installs an in-memory dimension manifest for a test.
// Tests share the package-level registry, so each test that depends on image
// sizes must call this instead of relying on the file on disk.
func setTestImageDims(m map[string]imgDim) {
	imgDimsMu.Lock()
	imgDimByURL = m
	imgDimsLoaded = true
	imgDimsMu.Unlock()
}

const (
	heroURL = "https://cdn.example.com/en-blog/treehouse/treehouse-1.jpg"
	bodyURL = "https://cdn.example.com/en-blog/treehouse/treehouse-2.jpg"
)

func TestMDToHTMLDecoratesImages(t *testing.T) {
	setTestImageDims(map[string]imgDim{
		heroURL: {W: 1200, H: 800},
		bodyURL: {W: 1024, H: 1024},
	})

	md := "Intro paragraph.\n\n" +
		"![" + "A LEGO treehouse MOC hero" + "](" + heroURL + ")\n\n" +
		"*Figure 1: hero*\n\n" +
		"More text.\n\n" +
		"![second](" + bodyURL + ")\n"
	out := string(mdToHTML(md))

	imgs := strings.Split(out, "<img")
	if len(imgs) != 3 {
		t.Fatalf("expected 2 <img> tags, got %d:\n%s", len(imgs)-1, out)
	}
	hero, second := imgs[1], imgs[2]

	for _, want := range []string{`width="1200"`, `height="800"`, `loading="eager"`, `fetchpriority="high"`, `decoding="async"`} {
		if !strings.Contains(hero, want) {
			t.Errorf("hero image missing %s:\n%s", want, hero)
		}
	}
	if strings.Contains(hero, `loading="lazy"`) {
		t.Errorf("hero image must not be lazy:\n%s", hero)
	}
	for _, want := range []string{`width="1024"`, `height="1024"`, `loading="lazy"`, `decoding="async"`} {
		if !strings.Contains(second, want) {
			t.Errorf("second image missing %s:\n%s", want, second)
		}
	}
	if strings.Contains(second, "fetchpriority") {
		t.Errorf("only the first image may claim fetchpriority=high:\n%s", second)
	}
}

func TestMDToHTMLKeepsUnknownImagesUnpainted(t *testing.T) {
	setTestImageDims(map[string]imgDim{})

	out := string(mdToHTML("![x](https://cdn.example.com/unknown.jpg)\n"))
	if !strings.Contains(out, `loading="eager"`) || !strings.Contains(out, `decoding="async"`) {
		t.Fatalf("unknown image still needs loading/decoding hints: %s", out)
	}
	if strings.Contains(out, "width=") || strings.Contains(out, "height=") {
		t.Fatalf("unknown image must not invent a size: %s", out)
	}
}

func TestMDToHTMLLeavesTextOnlyBodyAlone(t *testing.T) {
	setTestImageDims(map[string]imgDim{})
	out := string(mdToHTML("Just **text** and a [link](/blog/x).\n"))
	if strings.Contains(out, "<img") {
		t.Fatalf("unexpected img: %s", out)
	}
	if !strings.Contains(out, "<strong>text</strong>") {
		t.Fatalf("markdown rendering regressed: %s", out)
	}
}

func TestBlogPostJSONLDIsSingleCompleteEntity(t *testing.T) {
	setTestImageDims(map[string]imgDim{heroURL: {W: 1024, H: 1024}})
	body := "Intro.\n\n![A LEGO treehouse MOC](" + heroURL + ")\n\n" + strings.Repeat("word ", 300)
	post := &blog.Post{
		Slug:        "treehouse-moc-guide",
		Title:       "Treehouse MOC Guide",
		Description: "How to build a LEGO treehouse MOC.",
		Date:        time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC),
		Author:      "BrickPlan",
		Category:    "MOC Tutorial",
		Tags:        []string{"treehouse", "MOC"},
		Body:        body,
		Lang:        blog.LangEN,
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(blogPostJSONLD(post, "https://brickplan.cn", blog.LangEN)), &m); err != nil {
		t.Fatalf("invalid JSON-LD: %v", err)
	}
	if m["@type"] != "BlogPosting" {
		t.Fatalf("@type = %v, want BlogPosting", m["@type"])
	}
	imgs, ok := m["image"].([]interface{})
	if !ok || len(imgs) == 0 {
		t.Fatalf("image must be a non-empty array, got %#v", m["image"])
	}
	first, _ := imgs[0].(map[string]interface{})
	if first["url"] != heroURL {
		t.Errorf("image[0].url = %v, want %s", first["url"], heroURL)
	}
	if first["width"] != float64(1024) || first["height"] != float64(1024) {
		t.Errorf("image[0] missing intrinsic size: %#v", first)
	}
	if wc, _ := m["wordCount"].(float64); wc < 300 {
		t.Errorf("wordCount = %v, want >= 300", m["wordCount"])
	}
	if m["dateModified"] == nil || m["datePublished"] == nil {
		t.Errorf("datePublished/dateModified must both be present: %#v", m)
	}
	mer, _ := m["mainEntityOfPage"].(map[string]interface{})
	if mer["@id"] != "https://brickplan.cn/en/blog/treehouse-moc-guide" {
		t.Errorf("mainEntityOfPage.@id = %v", mer["@id"])
	}
	if m["inLanguage"] != "en" {
		t.Errorf("inLanguage = %v, want en", m["inLanguage"])
	}
	// Publisher must reference the site Organization node and carry a sized logo.
	pub, _ := m["publisher"].(map[string]interface{})
	if pub["@id"] != "https://brickplan.cn/#organization" {
		t.Errorf("publisher.@id = %v", pub["@id"])
	}
	if logo, ok := pub["logo"].(map[string]interface{}); !ok || logo["width"] == nil {
		t.Errorf("publisher.logo must be a sized ImageObject: %#v", pub["logo"])
	}
}
