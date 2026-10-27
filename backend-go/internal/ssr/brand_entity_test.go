package ssr

import (
	"strings"
	"testing"
)

// GEO batch: 奇积 must resolve to one entity across three spellings. The Chinese
// tree keeps the trade name as name and lists the aliases; the English tree
// promotes KEEPPLEY (the brand's own English name) to name. 乐高 is deliberately
// left alone -- LEGO is already unambiguous and extra aliases only dilute it.

func aliases(t *testing.T, org map[string]interface{}) []string {
	t.Helper()
	raw, ok := org["alternateName"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}: // JSON-LD round-trip through decodeJS
		out := make([]string, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				t.Fatalf("alternateName element is %T, want string", e)
			}
			out = append(out, s)
		}
		return out
	}
	t.Fatalf("alternateName is %T, want a string list", raw)
	return nil
}

func TestBrandOrganizationQijiZH(t *testing.T) {
	org := brandOrganization("奇积", "zh")
	if org["name"] != "奇积" {
		t.Fatalf("zh name = %v, want 奇积 (page display name must not change)", org["name"])
	}
	got := strings.Join(aliases(t, org), "|")
	if got != "奇妙积木|KEEPPLEY" {
		t.Fatalf("zh alternateName = %q, want 奇妙积木|KEEPPLEY", got)
	}
}

func TestBrandOrganizationQijiEN(t *testing.T) {
	org := brandOrganization("奇积", "en")
	if org["name"] != "KEEPPLEY" {
		t.Fatalf("en name = %v, want KEEPPLEY", org["name"])
	}
	got := strings.Join(aliases(t, org), "|")
	if got != "奇积|奇妙积木" {
		t.Fatalf("en alternateName = %q, want 奇积|奇妙积木 (no self-alias)", got)
	}
}

func TestBrandOrganizationLegoUntouched(t *testing.T) {
	for _, c := range []struct{ lang, name string }{{"zh", "乐高"}, {"en", "LEGO"}} {
		org := brandOrganization("乐高", c.lang)
		if org["name"] != c.name {
			t.Fatalf("%s name = %v, want %s", c.lang, org["name"], c.name)
		}
		if a := aliases(t, org); a != nil {
			t.Fatalf("%s 乐高 grew alternateName %v; LEGO was to stay as-is", c.lang, a)
		}
	}
}

// The creator node on the detail page is the same entity, in both trees.
func TestCreativeWorkCreatorCarriesAliases(t *testing.T) {
	bp := bpFixture()
	zh := decodeJS(t, string(creativeWorkJSONLD(bp, "", "https://brickplan.test", "zh")))
	creator, ok := zh["creator"].(map[string]interface{})
	if !ok {
		t.Fatalf("creator missing: %v", zh["creator"])
	}
	if creator["@type"] != "Organization" || creator["name"] != "奇积" {
		t.Fatalf("zh creator = %v", creator)
	}
	if a := aliases(t, creator); strings.Join(a, "|") != "奇妙积木|KEEPPLEY" {
		t.Fatalf("zh creator alternateName = %v", a)
	}
	en := decodeJS(t, string(creativeWorkJSONLD(bp, "", "https://brickplan.test", "en")))
	enCreator := en["creator"].(map[string]interface{})
	if enCreator["name"] != "KEEPPLEY" {
		t.Fatalf("en creator name = %v, want KEEPPLEY", enCreator["name"])
	}
	// The old pinyin guess must be gone from structured data.
	if strings.Contains(string(creativeWorkJSONLD(bp, "", "https://brickplan.test", "en")), "Qiji") {
		t.Fatal("\"Qiji\" still present in the English CreativeWork JSON-LD")
	}
}

// Visible chrome: the English detail page and its breadcrumb both say KEEPPLEY,
// the Chinese page still says 奇积.
func TestBrandDisplayKEEPPLEYOnEnglishPages(t *testing.T) {
	bp := bpFixture()
	en := string(detailNoscriptFor("en", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(en, "KEEPPLEY") {
		t.Fatal("English detail page does not display KEEPPLEY")
	}
	if strings.Contains(en, "Qiji") {
		t.Fatal("English detail page still displays the pinyin Qiji")
	}
	zh := string(detailNoscriptFor("zh", bp, "", "https://brickplan.test", nil))
	if !strings.Contains(zh, "奇积") {
		t.Fatal("Chinese detail page lost its 奇积 brand label")
	}
	enCrumb := string(detailBreadcrumbJSONLD("en", bp, "https://brickplan.test"))
	if !strings.Contains(enCrumb, `"name":"KEEPPLEY"`) {
		t.Fatalf("English breadcrumb JSON-LD does not use KEEPPLEY: %s", enCrumb)
	}
	// Same underlying entity, same @id-bearing URL: /brand/qiji stays the slug.
	if !strings.Contains(enCrumb, `"item":"https://brickplan.test/en/brand/qiji"`) {
		t.Fatalf("brand breadcrumb item changed: %s", enCrumb)
	}
}
