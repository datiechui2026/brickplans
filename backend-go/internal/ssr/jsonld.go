package ssr

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"brickplans/internal/db"
)

func mustJSON(v interface{}) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return template.JS(b)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func authorName(u *db.User) string {
	if u == nil {
		return "匿名"
	}
	return u.Username
}

// siteJSONLD returns Organization + WebSite (with SearchAction) — emitted on every page.
// The Organization node is referenced by @id from WebSite.publisher and the
// AboutPage, so every page exposes one canonical site entity to search engines
// and AI answer engines.
func (h *Handler) siteJSONLD() []template.JS {
	public := h.cfg.PublicURL
	orgID := public + "/#organization"
	org := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "Organization",
		"@id":           orgID,
		"name":          "BrickPlan",
		"alternateName": "积木图纸社区",
		"url":           public,
		"description":   "BrickPlan 是面向积木/MOC 爱好者的图纸分享社区，收录乐高及国产积木（双鹰、宇星、高砖、森宝、奇积等）的 MOC 图纸、零件知识、搭建教程与作品评测。",
		"slogan":        "发现和分享积木 MOC 图纸",
		"logo": map[string]interface{}{
			"@type":  "ImageObject",
			"url":    public + "/og-default.png",
			"width":  1200,
			"height": 630,
		},
		"image":      public + "/og-default.png",
		"inLanguage": "zh-CN",
		"areaServed": map[string]interface{}{"@type": "Country", "name": "China"},
		"knowsAbout": []string{"积木图纸", "MOC", "乐高 MOC", "积木搭建教程", "积木零件", "国产积木品牌"},
		"contactPoint": []map[string]interface{}{
			{
				"@type":             "ContactPoint",
				"contactType":       "customer support",
				"email":             "ghostjeky@hotmail.com",
				"url":               public + "/about",
				"availableLanguage": []string{"zh-CN"},
			},
		},
		"mainEntityOfPage": public + "/about",
	}
	website := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "WebSite",
		"@id":           public + "/#website",
		"url":           public,
		"name":          "BrickPlan",
		"alternateName": "积木图纸社区",
		"description":   "积木/MOC 图纸分享社区",
		"inLanguage":    "zh-CN",
		"publisher":     map[string]interface{}{"@id": orgID},
		"potentialAction": map[string]interface{}{
			"@type": "SearchAction",
			"target": map[string]interface{}{
				"@type":       "EntryPoint",
				"urlTemplate": public + "/explore?q={search_term_string}",
			},
			"query-input": "required name=search_term_string",
		},
	}
	return []template.JS{mustJSON(org), mustJSON(website)}
}

func creativeWorkJSONLD(bp *db.Blueprint, cover, public string) template.JS {
	m := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "CreativeWork",
		"name":          bp.Title,
		"description":   derefStr(bp.Description),
		"image":         cover,
		"url":           public + "/detail/" + bp.ID,
		"datePublished": bp.CreatedAt.Format("2006-01-02"),
		"dateModified":  bp.UpdatedAt.Format("2006-01-02"),
		"author":        map[string]interface{}{"@type": "Person", "name": authorName(bp.Author)},
		"inLanguage":    "zh-CN",
	}
	if bp.Difficulty != nil {
		m["contentRating"] = fmt.Sprintf("难度 %d/5", *bp.Difficulty)
	}
	if bp.PieceCount != nil {
		m["material"] = fmt.Sprintf("%d 个积木零件", *bp.PieceCount)
	}
	if bp.Category != nil {
		m["genre"] = *bp.Category
	}
	tags := []string{}
	for _, bt := range bp.Tags {
		if bt.Tag != nil {
			tags = append(tags, bt.Tag.Name)
		}
	}
	if len(tags) > 0 {
		m["keywords"] = strings.Join(tags, ", ")
	}
	return mustJSON(m)
}

// creativeWorkJSONLDEN is the inLanguage=en variant emitted by /en/detail.
// The blueprint content itself is Chinese (title/description stay in their
// published language — translating user content in SSR would desync from the
// catalogue), so only the machine-readable metadata switches to English.
func creativeWorkJSONLDEN(bp *db.Blueprint, cover, public string) template.JS {
	m := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "CreativeWork",
		"name":          bp.Title,
		"description":   derefStr(bp.Description),
		"image":         cover,
		"url":           public + "/en/detail/" + bp.ID,
		"datePublished": bp.CreatedAt.Format("2006-01-02"),
		"dateModified":  bp.UpdatedAt.Format("2006-01-02"),
		"author":        map[string]interface{}{"@type": "Person", "name": authorName(bp.Author)},
		"inLanguage":    "en",
	}
	if bp.Difficulty != nil {
		m["contentRating"] = fmt.Sprintf("Difficulty %d/5", *bp.Difficulty)
	}
	if bp.PieceCount != nil {
		m["material"] = fmt.Sprintf("%d brick pieces", *bp.PieceCount)
	}
	if bp.Category != nil {
		m["genre"] = *bp.Category
	}
	tags := []string{}
	for _, bt := range bp.Tags {
		if bt.Tag != nil {
			tags = append(tags, bt.Tag.Name)
		}
	}
	if len(tags) > 0 {
		m["keywords"] = strings.Join(tags, ", ")
	}
	return mustJSON(m)
}

// brandNode returns the Schema.org Brand node for a blueprint. It prefers the
// brick brand the uploader selected (乐高/森宝/双鹰/...) and falls back to the
// site's own name for rows uploaded before the field existed.
func brandNode(bp *db.Blueprint) map[string]interface{} {
	name := "BrickPlan"
	if bp.Brand != nil && strings.TrimSpace(*bp.Brand) != "" {
		name = strings.TrimSpace(*bp.Brand)
	}
	return map[string]interface{}{"@type": "Brand", "name": name}
}

// productJSONLD emits a Schema.org Product structured data for blueprint detail pages.
// This helps search engines (especially Baidu) display rich snippets with product info.
// The blueprint is always a free (price 0) downloadable design, so offers carries
// price 0 and interactionStatistic exposes the real engagement counters.
func productJSONLD(bp *db.Blueprint, cover, public string) template.JS {
	m := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "Product",
		"name":        bp.Title,
		"description": derefStr(bp.Description),
		"image":       cover,
		"url":         public + "/detail/" + bp.ID,
		"sku":         bp.ID,
		"brand":       brandNode(bp),
		"inLanguage":  "zh-CN",
		"offers": map[string]interface{}{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "CNY",
			"availability":  "https://schema.org/InStock",
			"url":           public + "/detail/" + bp.ID,
			"seller":        map[string]interface{}{"@type": "Organization", "name": "BrickPlan", "url": public},
		},
	}
	if bp.Category != nil {
		m["category"] = *bp.Category
	}
	// mpn (manufacturer part number) carries the real set/model number, which is
	// what people literally search for ("乐高 42115"). sku stays the internal id.
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		m["mpn"] = strings.TrimSpace(*bp.ProductCode)
	}
	if bp.PieceCount != nil {
		m["additionalProperty"] = map[string]interface{}{
			"@type": "PropertyValue",
			"name":  "零件数",
			"value": *bp.PieceCount,
		}
	}
	if bp.Difficulty != nil {
		m["aggregateRating"] = map[string]interface{}{
			"@type":       "AggregateRating",
			"ratingValue": *bp.Difficulty,
			"bestRating":  5,
			"worstRating": 1,
			"ratingCount": 1,
		}
	}
	m["interactionStatistic"] = engagementStatistic(bp.ViewCount, bp.LikeCount)
	return mustJSON(m)
}

// productJSONLDEN is the /en/detail variant of productJSONLD: English
// machine-readable labels, /en/detail URLs, and USD offers. The visible
// content (title/description) stays in its published language, so the JSON-LD
// matches what the page shows.
func productJSONLDEN(bp *db.Blueprint, cover, public string) template.JS {
	detailURL := public + "/en/detail/" + bp.ID
	m := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "Product",
		"name":        bp.Title,
		"description": derefStr(bp.Description),
		"image":       cover,
		"url":         detailURL,
		"sku":         bp.ID,
		"brand":       brandNode(bp),
		"inLanguage":  "en",
		"offers": map[string]interface{}{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "USD",
			"availability":  "https://schema.org/InStock",
			"url":           detailURL,
			"seller":        map[string]interface{}{"@type": "Organization", "name": "BrickPlan", "url": public},
		},
	}
	if bp.Category != nil {
		m["category"] = *bp.Category
	}
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		m["mpn"] = strings.TrimSpace(*bp.ProductCode)
	}
	if bp.PieceCount != nil {
		m["additionalProperty"] = map[string]interface{}{
			"@type": "PropertyValue",
			"name":  "Piece count",
			"value": *bp.PieceCount,
		}
	}
	if bp.Difficulty != nil {
		m["aggregateRating"] = map[string]interface{}{
			"@type":       "AggregateRating",
			"ratingValue": *bp.Difficulty,
			"bestRating":  5,
			"worstRating": 1,
			"ratingCount": 1,
		}
	}
	m["interactionStatistic"] = engagementStatistic(bp.ViewCount, bp.LikeCount)
	return mustJSON(m)
}

// engagementStatistic builds the InteractionCounter array shared by detail and
// list pages. AI answer engines and search engines use these to judge how much
// real usage a design has.
func engagementStatistic(views, likes int) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"@type":                "InteractionCounter",
			"interactionType":      "https://schema.org/ViewAction",
			"userInteractionCount": views,
		},
		{
			"@type":                "InteractionCounter",
			"interactionType":      "https://schema.org/LikeAction",
			"userInteractionCount": likes,
		},
	}
}

// blueprintListItem maps one blueprint to the nested Product node used inside
// ItemList. Image is deliberately omitted: list handlers do not preload images,
// so including it would mean an N+1 query on every listing page.
func blueprintListItem(bp db.Blueprint, public string) map[string]interface{} {
	url := public + "/detail/" + bp.ID
	m := map[string]interface{}{
		"@type":       "Product",
		"name":        bp.Title,
		"url":         url,
		"description": derefStr(bp.Description),
		"brand":       brandNode(&bp),
		"offers": map[string]interface{}{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "CNY",
			"availability":  "https://schema.org/InStock",
			"url":           url,
		},
	}
	if bp.Category != nil {
		m["category"] = *bp.Category
	}
	if bp.ProductCode != nil && strings.TrimSpace(*bp.ProductCode) != "" {
		m["mpn"] = strings.TrimSpace(*bp.ProductCode)
	}
	if bp.Difficulty != nil {
		m["aggregateRating"] = map[string]interface{}{
			"@type":       "AggregateRating",
			"ratingValue": *bp.Difficulty,
			"bestRating":  5,
			"worstRating": 1,
			"ratingCount": 1,
		}
	}
	if bp.PieceCount != nil {
		m["additionalProperty"] = map[string]interface{}{
			"@type": "PropertyValue",
			"name":  "零件数",
			"value": *bp.PieceCount,
		}
	}
	m["interactionStatistic"] = engagementStatistic(bp.ViewCount, bp.LikeCount)
	return m
}

func breadcrumbJSONLD(category, title, public string) template.JS {
	items := []map[string]interface{}{
		{"@type": "ListItem", "position": 1, "name": "首页", "item": public + "/"},
	}
	pos := 2
	if category != "" {
		items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": category, "item": public + "/explore?category=" + category})
		pos++
	}
	items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": title})
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// breadcrumbJSONLDEN is the English variant for /en/detail: Home → category →
// title, with the category step pointing into the English explore tree.
func breadcrumbJSONLDEN(category, title, public string) template.JS {
	items := []map[string]interface{}{
		{"@type": "ListItem", "position": 1, "name": "Home", "item": public + "/en/"},
	}
	pos := 2
	if category != "" {
		items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": category, "item": public + "/en/explore?category=" + url.QueryEscape(category)})
		pos++
	}
	items = append(items, map[string]interface{}{"@type": "ListItem", "position": pos, "name": title})
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// itemListJSONLD emits the ItemList for listing pages (home/explore/tags).
// Each entry nests a full Product node so list pages carry the same GEO signal
// (free offer, rating, interaction counters) as detail pages.
func itemListJSONLD(bps []db.Blueprint, public string) template.JS {
	return itemListJSONLDURL(bps, public, func(id string) string { return public + "/detail/" + id })
}

// itemListJSONLDEN is the English-tree variant: same shape as itemListJSONLD
// (Product/Offer/AggregateRating per item) but every URL is in the /en/detail
// space and the Offer currency is USD — the page audience is the international
// reader and USD is the only currency that tree actually implies. The CN tree
// keeps CNY, so the two ItemLists never claim different currencies for the
// same blueprint.
func itemListJSONLDEN(bps []db.Blueprint, public string) template.JS {
	return itemListJSONLDURL(bps, public, func(id string) string { return public + "/en/detail/" + id })
}

func itemListJSONLDURL(bps []db.Blueprint, public string, urlFor func(id string) string) template.JS {
	items := make([]map[string]interface{}, 0, len(bps))
	for i, bp := range bps {
		u := urlFor(bp.ID)
		item := blueprintListItem(bp, public)
		// Localize the nested node for the English tree: URL, Offer URL and
		// price currency. The title/description stay in their published
		// language (user content), matching the page's visible copy.
		item["url"] = u
		if offer, ok := item["offers"].(map[string]interface{}); ok {
			offer["url"] = u
			if strings.HasPrefix(u, public+"/en/") {
				offer["priceCurrency"] = "USD"
			}
		}
		if m, ok := item["@type"].(string); ok && m == "Product" {
			item["@id"] = u
		}
		items = append(items, map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     bp.Title,
			"url":      u,
			"item":     item,
		})
	}
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "ItemList",
		"numberOfItems":   len(items),
		"itemListOrder":   "https://schema.org/ItemListOrderDescending",
		"itemListElement": items,
	})
}

func profileJSONLD(u *db.User, bpCount int, public string) template.JS {
	return mustJSON(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "ProfilePage",
		"url":         public + "/user/" + u.ID,
		"name":        u.Username,
		"description": derefStr(u.Bio),
		"mainEntity": map[string]interface{}{
			"@type":         "Person",
			"name":          u.Username,
			"contributions": bpCount,
		},
	})
}

func faqJSONLD(qa []QA) template.JS {
	entities := make([]map[string]interface{}, 0, len(qa))
	for _, item := range qa {
		entities = append(entities, map[string]interface{}{
			"@type": "Question",
			"name":  item.Q,
			"acceptedAnswer": map[string]interface{}{
				"@type": "Answer",
				"text":  item.A,
			},
		})
	}
	return mustJSON(map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "FAQPage",
		"mainEntity": entities,
	})
}
