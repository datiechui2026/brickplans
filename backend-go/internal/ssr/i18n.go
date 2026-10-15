package ssr

import (
	"fmt"
	"html/template"
	"strings"
)

// ── 站点语言注册表 ─────────────────────────────────────────────────────────
//
// 加一门新语言 = 往 siteLangs 里加一项 + 补该语言的 navLabels / 页面文案。
// 导航、语言切换器、hreflang、静态页路由全部由这张表推导，**不要写 if/else 两分支**
// （历史上每加一种语言都被迫改多处，这就是这张表存在的理由）。
//
// Prefix 是 URL 前缀（中文为 ""，因为它就是根路径）；HTMLLang 同时用于
// <html lang> 和 hreflang 的取值 —— 这两处必须一致，否则 Google 会丢弃整个
// hreflang 簇（踩过：hreflang="en" 指向中文 URL）。
type Lang struct {
	Code     string // 内部代码：zh / en
	Label    string // 切换器上的显示名（用它自己的语言写）
	HTMLLang string // <html lang> + hreflang 取值：zh-CN / en
	Prefix   string // URL 前缀："" / "/en"
}

var siteLangs = []Lang{
	{Code: "zh", Label: "中文", HTMLLang: "zh-CN", Prefix: ""},
	{Code: "en", Label: "English", HTMLLang: "en", Prefix: "/en"},
}

// defaultLangCode is the language served at the bare path (""), i.e. Chinese.
const defaultLangCode = "zh"

// LangCodes returns every language code in declaration order.
func LangCodes() []string {
	out := make([]string, 0, len(siteLangs))
	for _, l := range siteLangs {
		out = append(out, l.Code)
	}
	return out
}

// langFor returns the Lang for a code, falling back to the default language so
// an unknown/empty code can never render a nav with no items.
func langFor(code string) Lang {
	for _, l := range siteLangs {
		if l.Code == code {
			return l
		}
	}
	return langFor(defaultLangCode)
}

// URL maps a language-neutral path ("/", "/about", "/blog") into this language's
// URL space. "" prefix (zh) returns the path unchanged — this is what keeps every
// already-indexed Chinese URL byte-identical.
func (l Lang) URL(path string) string {
	if l.Prefix == "" {
		return path
	}
	if path == "/" {
		return l.Prefix + "/"
	}
	return l.Prefix + path
}

// otherLangs returns every language except the given one (switcher options).
func otherLangs(code string) []Lang {
	out := make([]Lang, 0, len(siteLangs))
	for _, l := range siteLangs {
		if l.Code != code {
			out = append(out, l)
		}
	}
	return out
}

// ── 导航 ───────────────────────────────────────────────────────────────────
//
// navPaths is the language-neutral nav skeleton. Labels come from navLabels[lang];
// a missing label falls back to the path itself rather than rendering an empty
// <a>, so adding a language never silently produces a blank menu item.
var navPaths = []string{"/", "/explore", "/blog", "/about", "/faq", "/privacy"}

var navLabels = map[string]map[string]string{
	"zh": {"/": "首页", "/explore": "发现图纸", "/blog": "博客", "/about": "关于我们", "/faq": "常见问题", "/privacy": "隐私策略"},
	"en": {"/": "Home", "/explore": "Explore", "/blog": "Blog", "/about": "About", "/faq": "FAQ", "/privacy": "Privacy"},
}

func navLabel(lang, path string) string {
	if m, ok := navLabels[lang]; ok {
		if s, ok := m[path]; ok {
			return s
		}
	}
	return path
}

// navHTML renders the server-side navigation for one language: the menu items in
// that language, plus the language switcher.
//
// It is emitted into #app ahead of the page body so crawlers (and users without
// JS) see a complete menu; the SPA replaces #app on boot and renders its own
// equivalent (see frontend/src/main.js renderNavbar + LANGS).
//
// targets maps language code → URL the switcher should link to for the current
// page. A missing entry falls back to that language's blog list, which is the
// only subtree guaranteed to have content in every language today.
func navHTML(lang Lang, targets map[string]string) template.HTML {
	var b strings.Builder
	// "navbar" is added on top of "site-nav" so the server-rendered bar gets the
	// same sticky glass treatment as the SPA's navbar (.navbar in main.css).
	// It matters on pages the SPA never hydrates: there the SSR nav is final.
	b.WriteString(fmt.Sprintf(`<nav class="site-nav navbar" aria-label="%s">`, esc(navAriaLabel(lang.Code))))
	b.WriteString(fmt.Sprintf(`<a class="nav-brand" href="%s"><span class="logo">🧱</span> BrickPlan</a>`, esc(lang.URL("/"))))
	b.WriteString(`<ul class="nav-links">`)
	for _, p := range navPaths {
		b.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`, esc(lang.URL(p)), esc(navLabel(lang.Code, p))))
	}
	b.WriteString(`</ul>`)

	// 语言切换器：列表驱动，当前语言高亮（不是链接），其他语言是可点链接。
	b.WriteString(fmt.Sprintf(`<span class="lang-switch" role="group" aria-label="%s">`, esc(langSwitchAriaLabel(lang.Code))))
	for _, l := range siteLangs {
		if l.Code == lang.Code {
			b.WriteString(fmt.Sprintf(`<span class="lang-opt is-current" lang="%s" aria-current="true">%s</span>`,
				esc(l.HTMLLang), esc(l.Label)))
			continue
		}
		href := targets[l.Code]
		if href == "" {
			href = l.URL("/blog")
		}
		b.WriteString(fmt.Sprintf(`<a class="lang-opt" href="%s" hreflang="%s" lang="%s" rel="alternate">%s</a>`,
			esc(href), esc(l.HTMLLang), esc(l.HTMLLang), esc(l.Label)))
	}
	b.WriteString(`</span></nav>`)
	return template.HTML(b.String())
}

func navAriaLabel(code string) string {
	if code == "en" {
		return "Site navigation"
	}
	return "站点导航"
}

func langSwitchAriaLabel(code string) string {
	if code == "en" {
		return "Language"
	}
	return "语言"
}

// ── hreflang（静态页）──────────────────────────────────────────────────────
//
// staticPaths are the language-neutral paths that exist in EVERY language. They
// get a full zh-CN / en / x-default alternate cluster. Blog URLs are deliberately
// not here: an article only gets alternates when its counterpart really exists in
// the other tree (see seo postAlternates / blog.go), because an alternate
// pointing at a page serving the other language's text makes Google discard the
// whole cluster.
var staticPaths = []string{"/", "/explore", "/blog", "/about", "/faq", "/privacy"}

// staticAlternates builds the hreflang cluster for a language-neutral path.
// x-default points at the English version — the same choice the blog tree already
// makes, kept consistent so the site never emits two contradictory conventions.
func staticAlternates(path, public string) []AltLink {
	alts := make([]AltLink, 0, len(siteLangs)+1)
	for _, l := range siteLangs {
		alts = append(alts, AltLink{Hreflang: l.HTMLLang, Href: public + l.URL(path)})
	}
	alts = append(alts, AltLink{Hreflang: "x-default", Href: public + langFor("en").URL(path)})
	return alts
}

// ── 英文页面文案 ───────────────────────────────────────────────────────────
//
// 与 aboutSections() 同构，保证页面正文与 JSON-LD 不会各说各话。

func aboutSectionsEN() []aboutSection {
	return []aboutSection{
		{
			ID:      "background",
			Heading: "Background",
			Paras: []string{
				"BrickPlan is a blueprint-sharing community for brick and MOC builders, served from brickplan.cn.",
				"The problem it solves: building instructions are scattered across forums, cloud drives and social feeds, where they are hard to search, inconsistently formatted, and frequently dead links. BrickPlan collects them into one searchable, categorised site with real metadata — part count, difficulty, dimensions and recommended brands.",
				"The library currently covers six categories (architecture, vehicles, mecha, fantasy, sci-fi, scenery) across LEGO and Chinese brick brands such as Double Eagle, Mould King, Gobricks, SEMBO and Qiji, plus articles on part knowledge, brand comparisons and building techniques.",
			},
		},
		{
			ID:      "what-we-offer",
			Heading: "What we offer",
			Bullets: []string{
				"Blueprint library: MOC instructions searchable by category, tag and keyword, with in-page preview of images and PDFs.",
				"Metadata: every blueprint lists part count, difficulty, dimensions and recommended brands so you can judge feasibility before you start.",
				"Guides and reviews: the blog publishes MOC building tutorials, part-classification knowledge, brand reviews and buying advice.",
				"Community: likes, favourites, comments and author pages — you can follow the builders whose work you like.",
			},
		},
		{
			ID:      "team",
			Heading: "The team",
			Paras:   []string{"BrickPlan is maintained by a small independent team:"},
			Bullets: []string{
				"Content and operations: curating blueprints, category tagging, writing tutorials and reviews.",
				"Engineering: the website, the WeChat Mini Program and search experience.",
				"Community creators: blueprints and builds remain the property of their creators; BrickPlan only provides a platform to display and share them.",
			},
		},
		{
			ID:      "contact",
			Heading: "Contact",
			Bullets: []string{
				"Email: ghostjeky@hotmail.com (privacy and content matters).",
				"Infringement and takedowns: use the Report link at the bottom of a build page, or email us directly — we act after verification.",
				"Business and partnerships: same address, please put \"Partnership\" in the subject.",
			},
		},
		{
			ID:      "inclusion",
			Heading: "Content policy",
			Bullets: []string{
				"Scope: we list MOC blueprints and original articles recommended by builders or available in public channels. Copyright stays with the original creator.",
				"Submission: registered users can upload directly; others can recommend content by email and we will index it after review.",
				"Update frequency: the library and the blog are updated continuously; /sitemap.xml always reflects the latest content.",
				"Metadata: part counts and difficulty come from the original creator or public sources — corrections are welcome.",
				"Copyright: if you own the rights and do not want your work listed, contact us via Report or email and we will take it down promptly.",
				"Crawling and citation: search engines and AI assistants are welcome to crawl and cite our public content. Please keep the source link (https://brickplan.cn). We publish sitemap.xml, llms.txt and structured data to make machine reading easy.",
			},
		},
	}
}

func faqDataEN() []QA {
	return []QA{
		{"What is a MOC?", "MOC stands for My Own Creation — a build designed by a fan rather than released as an official set. BrickPlan is a community that shares MOC blueprints."},
		{"How do I upload my work?", "Register and sign in, then use the Upload button and fill in the title, category, difficulty and part count, and attach images or a PDF blueprint."},
		{"Which file types are supported?", "JPG/PNG/WebP images and PDF files, up to 20 MB each and 10 files per upload. Images are compressed and re-encoded automatically."},
		{"Who owns the blueprint copyright?", "Copyright stays with the creator. Do not upload work you do not own. If you find an infringement, use the Report link at the bottom of the build page."},
		{"How do I get more visibility?", "Complete descriptions, accurate tags and a clear cover image are the biggest factors. Builds with high view counts surface on the homepage."},
		{"Which categories are supported?", "Six: architecture, vehicles, mecha, fantasy, sci-fi and scenery. Tags refine them further."},
	}
}

// aboutNoscriptFor renders the /about body in the page's language.
func aboutNoscriptFor(lang string) template.HTML {
	if lang == "en" {
		return aboutBodyEN()
	}
	return aboutNoscript()
}

func aboutBodyEN() template.HTML {
	var b strings.Builder
	b.WriteString("<article>")
	b.WriteString("<h1>About BrickPlan</h1>")
	b.WriteString("<p>BrickPlan is a blueprint-sharing community for brick and MOC builders. This page covers the project background, the team, contact details and our content policy.</p>")
	for _, s := range aboutSectionsEN() {
		b.WriteString(fmt.Sprintf(`<section id="%s">`, esc(s.ID)))
		b.WriteString(fmt.Sprintf("<h2>%s</h2>", esc(s.Heading)))
		for _, p := range s.Paras {
			b.WriteString(fmt.Sprintf("<p>%s</p>", esc(p)))
		}
		if len(s.Bullets) > 0 {
			b.WriteString("<ul>")
			for _, li := range s.Bullets {
				b.WriteString(fmt.Sprintf("<li>%s</li>", esc(li)))
			}
			b.WriteString("</ul>")
		}
		b.WriteString("</section>")
	}
	b.WriteString(`<p><a href="/en/blog">Read building guides and reviews</a> · <a href="/en/explore">Browse all blueprints</a> · <a href="/en/faq">FAQ</a></p>`)
	b.WriteString("</article>")
	return template.HTML(b.String())
}

// privacyBodyEN renders /en/privacy. The Chinese /privacy page is a title-only
// shell (Handler.Simple); the English page carries real copy because it is a new
// indexed URL and a title-only page has nothing for a crawler to read.
func privacyBodyEN() template.HTML {
	var b strings.Builder
	b.WriteString("<article>")
	b.WriteString("<h1>Privacy Policy</h1>")
	b.WriteString("<p>BrickPlan (brickplan.cn) collects the minimum data needed to run a blueprint-sharing community. This page summarises what we store and why.</p>")
	b.WriteString("<h2>What we collect</h2><ul>")
	b.WriteString("<li>Account data: username, email address and a password hash. Passwords are never stored in plain text.</li>")
	b.WriteString("<li>Content you publish: blueprints, images, PDFs, descriptions, tags, comments and likes.</li>")
	b.WriteString("<li>Usage data: aggregate page views and, where you are signed in, your recent activity such as views and favourites.</li>")
	b.WriteString("</ul>")
	b.WriteString("<h2>How we use it</h2><ul>")
	b.WriteString("<li>To operate the site: authentication, publishing your work, showing comments and notifications.</li>")
	b.WriteString("<li>To improve discovery: aggregate, non-identifying statistics about which blueprints and articles are read.</li>")
	b.WriteString("</ul>")
	b.WriteString("<h2>What we do not do</h2><ul>")
	b.WriteString("<li>We do not sell your personal data.</li>")
	b.WriteString("<li>We do not publish your email address. Public pages show your username only.</li>")
	b.WriteString("</ul>")
	b.WriteString("<h2>Third parties</h2><ul>")
	b.WriteString("<li>Images and PDFs are served from Tencent Cloud COS (Hong Kong region).</li>")
	b.WriteString("<li>Anonymous visit statistics are collected by a self-hosted analytics service.</li>")
	b.WriteString("</ul>")
	b.WriteString("<h2>Your choices</h2><ul>")
	b.WriteString("<li>You can edit or delete your published work at any time from your account.</li>")
	b.WriteString("<li>For deletion of your account or any stored data, email ghostjeky@hotmail.com.</li>")
	b.WriteString("</ul>")
	b.WriteString(`<p><a href="/en/about">About BrickPlan</a> · <a href="/en/faq">FAQ</a> · <a href="/en/blog">Blog</a></p>`)
	b.WriteString("</article>")
	return template.HTML(b.String())
}

// exploreBodyEN is the English shell for the SPA browse page. The blueprint data
// is Chinese-language today, so this page is an English frame that links into the
// existing /explore list rather than pretending to be a translated catalogue.
func exploreBodyEN() template.HTML {
	var b strings.Builder
	b.WriteString("<article>")
	b.WriteString("<h1>Explore MOC blueprints</h1>")
	b.WriteString("<p>Browse brick blueprints shared by the community — architecture, vehicles, mecha, fantasy, sci-fi and scenery. Filter by category, tag or keyword.</p>")
	b.WriteString("<p>Blueprint titles and metadata are currently published in Chinese, so the catalogue itself is served from the main <a href=\"/explore\">explore page</a>. English guides and reviews live in the <a href=\"/en/blog\">English blog</a>.</p>")
	b.WriteString("</article>")
	return template.HTML(b.String())
}

// homeFooterEN is the English footer emitted on the English home page. The SPA
// builds its own footer for JS users; this one exists so crawlers get the same
// internal links.
func homeFooterEN(public string) template.HTML {
	var b strings.Builder
	b.WriteString(`<footer class="footer">`)
	b.WriteString(`<p>BrickPlan — MOC blueprints, building guides and brick reviews.</p>`)
	b.WriteString(`<p>`)
	for i, p := range navPaths {
		if i > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, esc(langFor("en").URL(p)), esc(navLabel("en", p))))
	}
	b.WriteString(`</p>`)
	b.WriteString(fmt.Sprintf(`<p><a href="%s">BrickPlan 中文站</a></p>`, public+"/"))
	b.WriteString(`</footer>`)
	return template.HTML(b.String())
}
