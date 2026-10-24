package blog

// RetiredENSlugs maps an English slug that was taken off the site to the live
// article that inherits its traffic and link equity.
//
// Two retirement waves, both on 2026-10-24:
//
//  1. 51 thin English articles (480–797 words) were retired in favour of the
//     long-form set (≥1197 words). Slugs that never earned an index entry simply
//     404; the ones that were already serving — and carried inbound links — are
//     listed here and answer with a 301 from the SSR layer.
//  2. The full English rollout re-derived every article from the Chinese corpus,
//     which renamed 13 slugs that were already in the English sitemap. Those
//     URLs are indexed, so they must land on the closest live article instead of
//     going dark — a 404 on an indexed URL is the one outcome worse than a thin
//     page.
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
//
// Every value must be a slug that exists in the English corpus; the redirect
// test asserts that, because a 301 into a 404 is worse than no redirect.
var RetiredENSlugs = map[string]string{
	// 626-word thin post → live parts-buying guide on the same intent (where to
	// buy parts), so the 301 is a topical successor rather than a soft-land.
	// Retargeted 2026-10-24: the previous target was itself renamed by the full
	// English rollout, and a 301 → 301 chain costs weight.
	"brick-parts-buy-guide": "bricklink-buy-parts-tutorial",

	// ── Renamed by the full English rollout (2026-10-24) ──
	"airplane-moc-blueprint-collection": "airplane-moc-blueprint-collection-2",
	"brick-lighting-moc-guide":          "moc-light-bricks-review",
	"bricklink-parts-ordering-guide":    "how-to-order-parts-by-blueprint",
	"brick-town-layout-design-guide":    "brick-town-design",
	"lego-color-matching-guide":         "moc-color-scheme-guide",
	"lego-parts-classification-guide":   "parts-classification-guide",
	"lego-parts-storage-guide":          "brick-parts-storage-sorting",
	"lego-train-track-parts-guide":      "railway-lego-building-guide",
	"minifigure-customization-guide":    "minifigure-moc-custom",
	"moc-assembly-order-guide":          "moc-assembly-order-tips",
	"moc-instructions-creation-guide":   "moc-blueprint-software-guide",
	"moc-snot-techniques-guide":         "moc-snot-techniques",
	"moc-sticker-application-guide":     "moc-sticker-guide",
}
