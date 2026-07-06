package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
)

type AdminHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewAdminHandler(cfg *config.Config, gdb *gorm.DB) *AdminHandler {
	return &AdminHandler{cfg: cfg, gdb: gdb}
}

func (h *AdminHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/admin", auth.AdminRequired(h.cfg, h.gdb))
	g.GET("/blueprints", h.listBlueprints)
	g.GET("/blueprints/pending", h.pendingBlueprints)
	g.PUT("/blueprints/:blueprint_id/publish", h.publish)
	g.PUT("/blueprints/:blueprint_id/unpublish", h.unpublish)
	g.DELETE("/blueprints/:blueprint_id", h.delete)
	g.GET("/reports", h.listReports)
}

func (h *AdminHandler) listBlueprints(c *gin.Context) {
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 20, 1)
	if size > 100 {
		size = 100
	}
	q := strings.TrimSpace(c.Query("q"))

	qry := h.gdb.Model(&db.Blueprint{}).Joins("LEFT JOIN users ON users.id = blueprints.author_id")
	if q != "" {
		like := "%" + escapeLike(q) + "%"
		qry = qry.Where("blueprints.title LIKE ? OR users.username LIKE ?", like, like)
	}
	var total int64
	qry.Count(&total)

	var bps []db.Blueprint
	qry.Preload("Author").Preload("Images", db.OrderImages).Preload("Tags.Tag").
		Order("blueprints.created_at DESC").Offset((page - 1) * size).Limit(size).Find(&bps)

	items := make([]dto.BlueprintOut, 0, len(bps))
	for i := range bps {
		items = append(items, *toBlueprintOut(&bps[i], 0, 0, false, false))
	}
	c.JSON(http.StatusOK, dto.BlueprintListOut{Items: items, Total: int(total), Page: page, PageSize: size})
}

func (h *AdminHandler) pendingBlueprints(c *gin.Context) {
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 20, 1)
	if size > 100 {
		size = 100
	}
	qry := h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", false)
	var total int64
	qry.Count(&total)

	var bps []db.Blueprint
	qry.Preload("Author").Preload("Images", db.OrderImages).Preload("Tags.Tag").
		Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&bps)

	items := make([]dto.BlueprintOut, 0, len(bps))
	for i := range bps {
		items = append(items, *toBlueprintOut(&bps[i], 0, 0, false, false))
	}
	c.JSON(http.StatusOK, dto.BlueprintListOut{Items: items, Total: int(total), Page: page, PageSize: size})
}

func (h *AdminHandler) publish(c *gin.Context) {
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	bp.IsPublished = true
	h.gdb.Save(&bp)
	c.JSON(http.StatusOK, gin.H{"detail": "Published"})
}

func (h *AdminHandler) unpublish(c *gin.Context) {
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	bp.IsPublished = false
	h.gdb.Save(&bp)
	c.JSON(http.StatusOK, gin.H{"detail": "Unpublished"})
}

func (h *AdminHandler) delete(c *gin.Context) {
	id := c.Param("blueprint_id")
	if err := h.gdb.Delete(&db.Blueprint{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AdminHandler) listReports(c *gin.Context) {
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 20, 1)
	if size > 100 {
		size = 100
	}

	var total int64
	h.gdb.Model(&db.Report{}).Distinct("blueprint_id").Count(&total)

	type bpRow struct {
		BlueprintID string
		ReportCount int
		Latest      time.Time
	}
	var rows []bpRow
	h.gdb.Model(&db.Report{}).
		Select("blueprint_id, count(*) as report_count, max(created_at) as latest").
		Group("blueprint_id").Order("latest DESC").
		Offset((page - 1) * size).Limit(size).Scan(&rows)

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		var bp db.Blueprint
		if err := h.gdb.Preload("Author").Preload("Images", db.OrderImages).Preload("Tags.Tag").
			First(&bp, "id = ?", r.BlueprintID).Error; err != nil {
			continue
		}
		var reports []db.Report
		h.gdb.Preload("Reporter").Where("blueprint_id = ?", r.BlueprintID).
			Order("created_at DESC").Find(&reports)
		repOut := make([]gin.H, 0, len(reports))
		for _, rp := range reports {
			repOut = append(repOut, gin.H{
				"id":           rp.ID,
				"reporter_id":  rp.ReporterID,
				"blueprint_id": rp.BlueprintID,
				"reason":       rp.Reason,
				"detail":       rp.Detail,
				"status":       rp.Status,
				"created_at":   dto.ISO(rp.CreatedAt),
				"reporter":     dto.FromUser(rp.Reporter),
			})
		}
		items = append(items, gin.H{
			"blueprint":    toBlueprintOut(&bp, 0, 0, false, false),
			"report_count": r.ReportCount,
			"reports":      repOut,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}
