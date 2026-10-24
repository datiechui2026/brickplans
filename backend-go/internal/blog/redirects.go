package blog

// RetiredENSlugs maps an English slug that was taken off the site to the live
// article that inherits its traffic and link equity.
//
// 2026-10-24: 51 thin English articles (480–797 words) were retired in favour of
// the long-form set (≥1197 words). Slugs that never earned an index entry simply
// 404; the ones that were already serving — and carried inbound links — are
// listed here and answer with a 301 from the SSR layer.
//
// This map is the single source of truth for "this English URL no longer
// exists", so the three surfaces cannot drift apart:
//
//   - blog.LoadDirs skips a retired slug even if its markdown reappears in a
//     content sync, which is what keeps it out of /sitemap-en-blog-*.xml (the
//     sitemap is data-driven off the store) and out of the /en/blog listing;
//   - ssr.BlogDetailEN answers the URL itself with a 301, not a 404, so indexed
//     links and external backlinks keep their weight;
//   - Chinese slugs are deliberately NOT covered. The ZH twin of a retired
//     English article is untouched and stays live at /blog/<slug>.
var RetiredENSlugs = map[string]string{
	// 626-word thin post → 1597-word guide on the same intent (where to buy
	// parts), so the 301 is a topical successor rather than a generic soft-land.
	"brick-parts-buy-guide": "bricklink-parts-ordering-guide",
}
