package handler

import (
	"fmt"
	"net/http"
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

func (h *SEOHandler) sitemap(c *gin.Context) {
	var bps []db.Blueprint
	h.gdb.Where("is_published = ?", true).Order("updated_at DESC").Find(&bps)

	const baseURL = "https://brickplans.com"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, bp := range bps {
		lastmod := bp.UpdatedAt.Format("2006-01-02")
		fmt.Fprintf(&b, "  <url>\n    <loc>%s/#/detail?id=%s</loc>\n    <lastmod>%s</lastmod>\n  </url>\n",
			baseURL, bp.ID, lastmod)
	}
	b.WriteString("</urlset>")
	c.Data(http.StatusOK, "application/xml", []byte(b.String()))
}
