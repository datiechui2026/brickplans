package ssr

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"brickplans/internal/db"
)

func mustJSON(v interface{}) template.HTML {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return template.HTML(b)
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
func (h *Handler) siteJSONLD() []template.HTML {
	public := h.cfg.PublicURL
	org := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "Organization",
		"name":        "BrickPlans",
		"url":         public,
		"description": "积木/MOC 图纸分享社区",
		"logo":        public + "/og-default.png",
	}
	website := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "WebSite",
		"url":      public,
		"name":     "BrickPlans",
		"inLanguage": "zh-CN",
		"potentialAction": map[string]interface{}{
			"@type": "SearchAction",
			"target": map[string]interface{}{
				"@type":       "EntryPoint",
				"urlTemplate": public + "/explore?q={search_term_string}",
			},
			"query-input": "required name=search_term_string",
		},
	}
	return []template.HTML{mustJSON(org), mustJSON(website)}
}

func creativeWorkJSONLD(bp *db.Blueprint, cover, public string) template.HTML {
	m := map[string]interface{}{
		"@context":     "https://schema.org",
		"@type":        "CreativeWork",
		"name":         bp.Title,
		"description":  derefStr(bp.Description),
		"image":        cover,
		"url":          public + "/detail/" + bp.ID,
		"datePublished": bp.CreatedAt.Format("2006-01-02"),
		"dateModified":  bp.UpdatedAt.Format("2006-01-02"),
		"author":       map[string]interface{}{"@type": "Person", "name": authorName(bp.Author)},
		"inLanguage":   "zh-CN",
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

func breadcrumbJSONLD(category, title, public string) template.HTML {
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

func itemListJSONLD(bps []db.Blueprint, public string) template.HTML {
	items := make([]map[string]interface{}, 0, len(bps))
	for i, bp := range bps {
		items = append(items, map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     bp.Title,
			"url":      public + "/detail/" + bp.ID,
		})
	}
	return mustJSON(map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "ItemList",
		"itemListElement": items,
	})
}

func profileJSONLD(u *db.User, bpCount int, public string) template.HTML {
	return mustJSON(map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "ProfilePage",
		"url":        public + "/user/" + u.ID,
		"name":       u.Username,
		"description": derefStr(u.Bio),
		"mainEntity": map[string]interface{}{
			"@type":       "Person",
			"name":        u.Username,
			"contributions": bpCount,
		},
	})
}

func faqJSONLD(qa []struct{ Q, A string }) template.HTML {
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
