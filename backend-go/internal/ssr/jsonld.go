package ssr

import (
	"encoding/json"
	"fmt"
	"html/template"
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
				"email":             "privacy@brickplan.cn",
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
		"brand":       map[string]interface{}{"@type": "Brand", "name": "BrickPlan"},
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
		"brand":       map[string]interface{}{"@type": "Brand", "name": "BrickPlan"},
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

// itemListJSONLD emits the ItemList for listing pages (home/explore/tags).
// Each entry nests a full Product node so list pages carry the same GEO signal
// (free offer, rating, interaction counters) as detail pages.
func itemListJSONLD(bps []db.Blueprint, public string) template.JS {
	items := make([]map[string]interface{}, 0, len(bps))
	for i, bp := range bps {
		url := public + "/detail/" + bp.ID
		items = append(items, map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     bp.Title,
			"url":      url,
			"item":     blueprintListItem(bp, public),
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
