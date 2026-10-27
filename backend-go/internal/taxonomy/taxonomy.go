// Package taxonomy holds the URL taxonomy BrickPlan advertises as crawlable
// landing pages: the six categories, two brands and five difficulty levels of
// ticket SEO-20261024.
//
// It is a leaf package on purpose (no imports at all). The SSR layer renders
// these pages and the sitemap writer advertises them; if the two ever disagreed
// the site would either advertise a 404 or hide a page from the index, and both
// failures are invisible in unit tests that only look at one side.
package taxonomy

// Family is the URL family of an aggregate page. The value doubles as the first
// URL segment, so /category/scene, /brand/qiji and /difficulty/2 all fall out of
// Path().
type Family string

const (
	Category   Family = "category"
	Brand      Family = "brand"
	Difficulty Family = "difficulty"
)

// Entry is one aggregate landing page.
//
// Value is what the database actually stores — Chinese category names, Chinese
// brand names, and the difficulty number as a string. It is deliberately not
// translated: filters match on the stored value and the API has always accepted
// it, so only the URL and the visible label are English on /en pages.
type Entry struct {
	Family  Family
	Slug    string
	Value   string
	LabelEN string
}

// AggregateEntries is the full list, in the order the pages should appear in
// navigation blocks on each other's pages. The slug spelling is a contract with
// the sitemap and with every breadcrumb already emitted — do not "improve" the
// translations.
var AggregateEntries = []Entry{
	{Category, "scene", "场景", "Scene"},
	{Category, "vehicle", "车辆", "Vehicles"},
	{Category, "fantasy", "奇幻", "Fantasy"},
	{Category, "mecha", "机甲", "Mecha"},
	{Category, "architecture", "建筑", "Architecture"},
	{Category, "scifi", "科幻", "Sci-Fi"},
	// 奇积 的英文名是品牌自用的 KEEPPLEY；slug 保持 qiji 不动（已进 sitemap 与面包屑）。
	{Brand, "qiji", "奇积", "KEEPPLEY"},
	{Brand, "lego", "乐高", "LEGO"},
	{Difficulty, "1", "1", "Difficulty 1"},
	{Difficulty, "2", "2", "Difficulty 2"},
	{Difficulty, "3", "3", "Difficulty 3"},
	{Difficulty, "4", "4", "Difficulty 4"},
	{Difficulty, "5", "5", "Difficulty 5"},
}

// Path returns the language-neutral URL path of an entry ("/category/scene").
// Callers add the language prefix themselves so the zh tree stays unchanged.
func Path(e Entry) string { return "/" + string(e.Family) + "/" + e.Slug }

// Lookup resolves the URL segment of one family ("scene", "qiji", "2").
func Lookup(f Family, slug string) (Entry, bool) {
	for _, e := range AggregateEntries {
		if e.Family == f && e.Slug == slug {
			return e, true
		}
	}
	return Entry{}, false
}

// ForValue resolves a stored database value back to its landing page. This is
// what detail-page breadcrumbs use: the row carries 场景, the crumb must point at
// /category/scene, and a value with no landing page (星战, 其他, 森宝 …) must
// come back false so the caller renders plain text instead of inventing a link.
func ForValue(f Family, value string) (Entry, bool) {
	for _, e := range AggregateEntries {
		if e.Family == f && e.Value == value {
			return e, true
		}
	}
	return Entry{}, false
}

// InFamily returns every entry of one family except the given slug, used by the
// "other categories / other brands" blocks.
func InFamily(f Family, exclude string) []Entry {
	out := make([]Entry, 0, len(AggregateEntries))
	for _, e := range AggregateEntries {
		if e.Family == f && e.Slug != exclude {
			out = append(out, e)
		}
	}
	return out
}

// Slugs returns the slugs of one family in display order.
func Slugs(f Family) []string {
	out := make([]string, 0, len(AggregateEntries))
	for _, e := range AggregateEntries {
		if e.Family == f {
			out = append(out, e.Slug)
		}
	}
	return out
}
