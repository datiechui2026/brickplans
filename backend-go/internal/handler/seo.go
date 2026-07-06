package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
)

type SEOHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewSEOHandler(cfg *config.Config, gdb *gorm.DB) *SEOHandler {
	return &SEOHandler{cfg: cfg, gdb: gdb}
}

// RegisterRoutes mounts the sitemap at the site root (not under /api).
func (h *SEOHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/sitemap.xml", h.sitemap)
}

func writeURL(b *strings.Builder, loc, lastmod, prio, freq string) {
	b.WriteString("  <url>\n    <loc>" + loc + "</loc>\n")
	if lastmod != "" {
		b.WriteString("    <lastmod>" + lastmod + "</lastmod>\n")
	}
	if freq != "" {
		b.WriteString("    <changefreq>" + freq + "</changefreq>\n")
	}
	if prio != "" {
		b.WriteString("    <priority>" + prio + "</priority>\n")
	}
	b.WriteString("  </url>\n")
}

func (h *SEOHandler) sitemap(c *gin.Context) {
	base := h.cfg.PublicURL
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	// Static pages
	writeURL(&b, base+"/", "", "1.0", "daily")
	writeURL(&b, base+"/explore", "", "0.9", "daily")
	writeURL(&b, base+"/faq", "", "0.6", "weekly")
	writeURL(&b, base+"/privacy", "", "0.3", "yearly")

	// Category pages
	for _, cat := range []string{"建筑", "车辆", "机甲", "奇幻", "科幻", "场景"} {
		writeURL(&b, base+"/explore?category="+url.QueryEscape(cat), "", "0.7", "weekly")
	}

	// Tag pages
	var tags []db.Tag
	h.gdb.Order("name ASC").Find(&tags)
	for _, t := range tags {
		writeURL(&b, base+"/tags/"+url.PathEscape(t.Name), "", "0.6", "weekly")
	}

	// Detail pages (only published)
	var bps []db.Blueprint
	h.gdb.Where("is_published = ?", true).Order("updated_at DESC").Find(&bps)
	for _, bp := range bps {
		writeURL(&b, base+"/detail/"+bp.ID, bp.UpdatedAt.Format("2006-01-02"), "0.8", "weekly")
	}

	b.WriteString("</urlset>")
	c.Data(http.StatusOK, "application/xml", []byte(b.String()))
}
