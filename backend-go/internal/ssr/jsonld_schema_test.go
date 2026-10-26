package ssr

// 工单 SEO-20261026-03 G-1 / G-2 的机器可读契约。
//
// 两条都是「风险项」而不是美化项：G-1 之前每个详情页都在 JSON-LD 里声明
// aggregateRating{ratingCount:1, ratingValue:难度}，等于主动告诉 Google 与
// 各类 AI 搜索「这条内容只有 1 票、而且只有 2 分」；G-2 之前页面并列输出
// Product + Offer{price:"0"}，而我们根本没有售价与购物车 —— 虚假 Offer 属于
// 结构化数据政策风险。这里把「不许再出现」写死成断言。

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"brickplans/internal/db"
)

func strp(s string) *string { return &s }
func intp(n int) *int       { return &n }

func fixtureBlueprint() *db.Blueprint {
	now := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	return &db.Blueprint{
		ID:          "11111111-2222-4333-8444-555555555555",
		Title:       "千年隼",
		TitleEN:     strp("Millennium Falcon"),
		Description: strp("乐高 75375 千年隼 搭建说明书。系列: Star Wars。共 1 份说明书。"),
		DescriptionEN: strp("LEGO 75375 Millennium Falcon building instructions."),
		Category:    strp("星战"),
		Brand:       strp("乐高"),
		ProductCode: strp("75375"),
		PieceCount:  intp(1329),
		Difficulty:  intp(4),
		ViewCount:   120,
		LikeCount:   7,
		CreatedAt:   now,
		UpdatedAt:   now,
		Author:      &db.User{Username: "brickfan"},
	}
}

func decodeJS(t *testing.T, js string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("JSON-LD is not valid JSON: %v\n%s", err, js)
	}
	return m
}

// G-1：评分条数 <3 或均值缺失 → 整个 aggregateRating 节点不出现。
func TestRatingNodeOnlyEmitsWithEnoughRealRatings(t *testing.T) {
	for _, c := range []struct {
		count int
		avg   float64
	}{
		{0, 0}, {1, 2}, {1, 5}, {2, 4.5}, {0, 4.9}, {5, 0},
	} {
		if node := ratingNode(c.count, c.avg); node != nil {
			t.Errorf("ratingCount=%d avg=%v must not emit aggregateRating, got %v", c.count, c.avg, node)
		}
	}
	node := ratingNode(3, 4.25)
	if node == nil {
		t.Fatal("3 real ratings must emit aggregateRating")
	}
	if node["ratingCount"] != 3 {
		t.Errorf("ratingCount = %v, want 3", node["ratingCount"])
	}
	if node["ratingValue"] != 4.3 { // 保留一位小数
		t.Errorf("ratingValue = %v, want 4.3", node["ratingValue"])
	}
}

// G-2：详情页主实体是 CreativeWork，没有 Product、没有 Offer、没有 price。
func TestDetailJSONLDIsCreativeWorkWithoutOffer(t *testing.T) {
	bp := fixtureBlueprint()
	raw := string(creativeWorkJSONLD(bp, "https://cdn.test/cover.jpg", "https://brickplan.test", "zh"))

	for _, banned := range []string{`"Product"`, `"Offer"`, `"price"`, `"priceCurrency"`, `"aggregateRating"`, `"sku"`, `"mpn"`} {
		if strings.Contains(raw, banned) {
			t.Errorf("detail JSON-LD must not contain %s: %s", banned, raw)
		}
	}

	m := decodeJS(t, raw)
	if m["@type"] != "CreativeWork" {
		t.Errorf("@type = %v, want CreativeWork", m["@type"])
	}
	if m["url"] != "https://brickplan.test/detail/"+bp.ID {
		t.Errorf("url = %v", m["url"])
	}
	if m["@id"] != m["url"] {
		t.Errorf("@id (%v) must match the page URL (%v)", m["@id"], m["url"])
	}
	if m["inLanguage"] != "zh-CN" {
		t.Errorf("inLanguage = %v", m["inLanguage"])
	}
	if m["identifier"] != "75375" {
		t.Errorf("identifier = %v, want the real set number 75375", m["identifier"])
	}
	if _, ok := m["isPartOf"].(map[string]interface{}); !ok {
		t.Errorf("isPartOf(WebSite) missing: %v", m["isPartOf"])
	}
	if _, ok := m["author"].(map[string]interface{}); !ok {
		t.Errorf("author missing: %v", m["author"])
	}
	props, ok := m["additionalProperty"].([]interface{})
	if !ok || len(props) != 2 {
		t.Fatalf("additionalProperty should carry 颗粒数 + 难度, got %v", m["additionalProperty"])
	}
	// 难度必须出现在 additionalProperty 里，而不是被当成 ratingValue。
	if strings.Contains(raw, `"contentRating"`) {
		t.Errorf("difficulty must not be emitted as contentRating: %s", raw)
	}
}

// 英文树：url/@id 必须落在 /en/detail/，inLanguage 是 en。
func TestDetailJSONLDEnglishStaysInEnglishTree(t *testing.T) {
	bp := fixtureBlueprint()
	m := decodeJS(t, string(creativeWorkJSONLD(bp, "", "https://brickplan.test", "en")))
	if !strings.HasPrefix(m["url"].(string), "https://brickplan.test/en/detail/") {
		t.Errorf("english url = %v", m["url"])
	}
	if m["inLanguage"] != "en" {
		t.Errorf("inLanguage = %v", m["inLanguage"])
	}
	if m["name"] != "Millennium Falcon" {
		t.Errorf("name = %v", m["name"])
	}
}

// 首页 ItemList 的 10 个列表项同规则：CreativeWork、无 Offer、无假评分。
func TestItemListItemsAreCreativeWorkWithoutRatings(t *testing.T) {
	bp := *fixtureBlueprint()
	second := *fixtureBlueprint()
	second.ID = "99999999-2222-4333-8444-555555555555"
	second.Difficulty = intp(1) // 过去这一条会输出「1 票 1 星」

	raw := string(itemListJSONLD([]db.Blueprint{bp, second}, "https://brickplan.test"))
	for _, banned := range []string{`"Product"`, `"Offer"`, `"price"`, `"aggregateRating"`, `"ratingCount"`} {
		if strings.Contains(raw, banned) {
			t.Errorf("home ItemList must not contain %s: %s", banned, raw)
		}
	}
	var list struct {
		Type  string `json:"@type"`
		Items []struct {
			Item map[string]interface{} `json:"item"`
		} `json:"itemListElement"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("ItemList is not valid JSON: %v", err)
	}
	if list.Type != "ItemList" || len(list.Items) != 2 {
		t.Fatalf("unexpected ItemList shape: %+v", list)
	}
	for i, it := range list.Items {
		if it.Item["@type"] != "CreativeWork" {
			t.Errorf("item %d @type = %v, want CreativeWork", i, it.Item["@type"])
		}
		if _, ok := it.Item["offers"]; ok {
			t.Errorf("item %d still carries offers", i)
		}
	}
}

// G-2 附带项：英文树的 genre 必须是英文（原来英文页 genre 直接输出中文「星战」），
// 中文树保持库内原值不动 —— 译表只在 /en 生效，中文页面不该被顺手改动。
func TestGenreFollowsLanguageTree(t *testing.T) {
	bp := fixtureBlueprint()
	zh := decodeJS(t, string(creativeWorkJSONLD(bp, "", "https://brickplan.test", "zh")))
	en := decodeJS(t, string(creativeWorkJSONLD(bp, "", "https://brickplan.test", "en")))
	if zh["genre"] != "星战" {
		t.Errorf("zh genre = %v, want 星战", zh["genre"])
	}
	if en["genre"] != "Star Wars" {
		t.Errorf("en genre = %v, want Star Wars", en["genre"])
	}
	if zh["inLanguage"] != "zh-CN" || en["inLanguage"] != "en" {
		t.Errorf("inLanguage zh=%v en=%v", zh["inLanguage"], en["inLanguage"])
	}
	if !strings.HasSuffix(en["url"].(string), "/en/detail/"+bp.ID) {
		t.Errorf("en url = %v, must stay in /en tree", en["url"])
	}
}
