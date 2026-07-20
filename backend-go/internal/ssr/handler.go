package ssr

import (
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/blog"
	"brickplans/internal/config"
	"brickplans/internal/db"
)

type Handler struct {
	cfg       *config.Config
	gdb       *gorm.DB
	r         *Renderer
	blogStore *blog.Store
}

func NewHandler(cfg *config.Config, gdb *gorm.DB, r *Renderer, blogStore *blog.Store) *Handler {
	return &Handler{cfg: cfg, gdb: gdb, r: r, blogStore: blogStore}
}

// RegisterRoutes mounts SSR page routes at the site root (alongside /api, /uploads).
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/", h.Home)
	r.GET("/explore", h.Explore)
	r.GET("/detail/:id", h.Detail)
	r.GET("/tags/:name", h.Tag)
	r.GET("/user/:id", h.User)
	r.GET("/upload", h.Simple("上传图纸", "在 BrickPlans 上传你的积木 MOC 图纸，分享给社区。"))
	r.GET("/edit/:id", h.Simple("编辑图纸", "编辑你的积木图纸信息。"))
	r.GET("/notifications", h.Simple("通知", "你在 BrickPlans 的互动通知。"))
	r.GET("/admin", h.Simple("管理后台", "BrickPlans 管理后台。"))
	r.GET("/privacy", h.Simple("隐私策略", "BrickPlans 隐私策略。"))
	r.GET("/faq", h.FAQ)
	r.GET("/blog", h.BlogList)
	r.GET("/blog/:slug", h.BlogDetail)
}

func (h *Handler) Home(c *gin.Context) {
	var bps []db.Blueprint
	h.gdb.Where("is_published = ?", true).Order("view_count DESC").Limit(10).Find(&bps)
	h.r.Render(c, PageData{
		Title:       "BrickPlans — 积木图纸分享社区",
		Description: "BrickPlans 是积木/MOC 图纸分享社区，发现和分享乐高 MOC 创意作品。浏览建筑、车辆、机甲、奇幻、科幻、场景等各类积木图纸。",
		Canonical:   h.cfg.PublicURL + "/",
		OGType:      "website",
		JSONLD:      append(h.siteJSONLD(), itemListJSONLD(bps, h.cfg.PublicURL)),
		Noscript:    homeNoscript(bps, h.cfg.PublicURL),
	})
}

func (h *Handler) Detail(c *gin.Context) {
	id := c.Param("id")
	var bp db.Blueprint
	if err := h.gdb.Preload("Author").Preload("Images", db.OrderImages).Preload("Tags.Tag").First(&bp, "id = ?", id).Error; err != nil {
		h.NotFound(c)
		return
	}
	if !bp.IsPublished {
		h.NotFound(c)
		return
	}
	cover := coverURL(bp.Images, h.cfg.PublicURL)
	desc := "BrickPlans 积木图纸：" + bp.Title
	if bp.Description != nil && *bp.Description != "" {
		desc = *bp.Description
	}
	category := ""
	if bp.Category != nil {
		category = *bp.Category
	}
	jsonld := h.siteJSONLD()
	jsonld = append(jsonld, creativeWorkJSONLD(&bp, cover, h.cfg.PublicURL))
	jsonld = append(jsonld, breadcrumbJSONLD(category, bp.Title, h.cfg.PublicURL))
	h.r.Render(c, PageData{
		Title:       bp.Title + " — BrickPlans 积木图纸",
		Description: truncate(desc, 160),
		Canonical:   h.cfg.PublicURL + "/detail/" + bp.ID,
		OGType:      "article",
		OGImage:     cover,
		JSONLD:      jsonld,
		Noscript:    detailNoscript(&bp, cover, h.cfg.PublicURL),
	})
}

func (h *Handler) Explore(c *gin.Context) {
	cat := c.Query("category")
	var bps []db.Blueprint
	qry := h.gdb.Where("is_published = ?", true)
	if cat != "" {
		qry = qry.Where("category = ?", cat)
	}
	qry.Order("created_at DESC").Limit(50).Find(&bps)
	title := "发现图纸 — BrickPlans"
	desc := "浏览社区积木 MOC 作品，按分类、标签、关键词搜索图纸。"
	if cat != "" {
		title = cat + "类积木图纸 — BrickPlans"
		desc = "浏览" + cat + "分类的积木 MOC 图纸作品。"
	}
	h.r.Render(c, PageData{
		Title:     title,
		Description: desc,
		Canonical: h.cfg.PublicURL + "/explore",
		OGType:    "website",
		JSONLD:    append(h.siteJSONLD(), itemListJSONLD(bps, h.cfg.PublicURL)),
		Noscript:  listNoscript(bps, h.cfg.PublicURL),
	})
}

func (h *Handler) Tag(c *gin.Context) {
	name := c.Param("name")
	var bps []db.Blueprint
	h.gdb.Where("is_published = ? AND id IN (SELECT bt.blueprint_id FROM blueprint_tags bt JOIN tags t ON t.id = bt.tag_id WHERE t.name = ?)", true, name).
		Order("created_at DESC").Limit(50).Find(&bps)
	h.r.Render(c, PageData{
		Title:       "#" + name + " 标签 — BrickPlans",
		Description: "标签「" + name + "」下的积木 MOC 图纸作品。",
		Canonical:   h.cfg.PublicURL + "/tags/" + name,
		OGType:      "website",
		JSONLD:      append(h.siteJSONLD(), itemListJSONLD(bps, h.cfg.PublicURL)),
		Noscript:    listNoscript(bps, h.cfg.PublicURL),
	})
}

func (h *Handler) User(c *gin.Context) {
	id := c.Param("id")
	var user db.User
	if err := h.gdb.Where("id = ? OR username = ?", id, id).First(&user).Error; err != nil {
		h.NotFound(c)
		return
	}
	var bps []db.Blueprint
	h.gdb.Where("author_id = ? AND is_published = ?", user.ID, true).Order("created_at DESC").Limit(50).Find(&bps)
	var bpCount int64
	h.gdb.Model(&db.Blueprint{}).Where("author_id = ? AND is_published = ?", user.ID, true).Count(&bpCount)
	h.r.Render(c, PageData{
		Title:       user.Username + " 的作品 — BrickPlans",
		Description: "查看 " + user.Username + " 分享的积木 MOC 图纸作品。",
		Canonical:   h.cfg.PublicURL + "/user/" + user.ID,
		OGType:      "profile",
		JSONLD:      append(h.siteJSONLD(), profileJSONLD(&user, int(bpCount), h.cfg.PublicURL)),
		Noscript: template.HTML("<h1>"+esc(user.Username)+" 的作品</h1>") +
			listNoscript(bps, h.cfg.PublicURL),
	})
}

// Simple renders pages with only site-wide meta (no DB content).
func (h *Handler) Simple(title, desc string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.r.Render(c, PageData{
			Title:       title + " — BrickPlans",
			Description: desc,
			Canonical:   h.cfg.PublicURL + c.Request.URL.Path,
			OGType:      "website",
			JSONLD:      h.siteJSONLD(),
		})
	}
}

func (h *Handler) FAQ(c *gin.Context) {
	qa := faqData()
	h.r.Render(c, PageData{
		Title:       "常见问题 — BrickPlans",
		Description: "BrickPlans 常见问题解答：什么是 MOC、如何上传、版权与举报等。",
		Canonical:   h.cfg.PublicURL + "/faq",
		OGType:      "website",
		JSONLD:      append(h.siteJSONLD(), faqJSONLD(qa)),
		Noscript:    faqNoscript(qa),
	})
}

func (h *Handler) NotFound(c *gin.Context) {
	c.Status(http.StatusNotFound)
	h.r.Render(c, PageData{
		Title:       "页面不存在 — BrickPlans",
		Description: "页面不存在。",
		Canonical:   h.cfg.PublicURL + "/",
		OGType:      "website",
		JSONLD:      h.siteJSONLD(),
		Noscript:    template.HTML("<h1>404</h1><p>页面不存在。</p><p><a href=\"/\">返回首页</a></p>"),
	})
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

func faqData() []struct{ Q, A string } {
	return []struct{ Q, A string }{
		{"什么是 MOC？", "MOC（My Own Creation）是乐高玩家自己设计的原创作品，区别于官方套装。BrickPlans 是分享 MOC 图纸的社区。"},
		{"如何上传我的作品？", "注册登录后，点击右上角「上传」按钮，填写标题、分类、难度、零件数等信息，并上传图片或 PDF 图纸。"},
		{"支持什么文件格式？", "支持 JPG/PNG/WebP 图片和 PDF 文件，单文件最大 20MB，一次最多 10 个文件。图片会自动压缩转码。"},
		{"图纸版权归谁？", "作品版权归创作者所有。请勿上传他人受版权保护的作品；发现侵权可在作品页底部「举报」反馈。"},
		{"怎样让作品更受欢迎？", "完善描述、添加标签、上传清晰封面图的作品更易被发现。浏览量高的作品会出现在首页热门区。"},
		{"支持哪些分类？", "建筑、车辆、机甲、奇幻、科幻、场景六大分类，可配合标签进一步细化。"},
	}
}
