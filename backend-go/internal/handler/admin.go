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
	g.GET("/users", h.listUsers)
	g.DELETE("/users/:user_id", h.deleteUser)
	g.PUT("/users/:user_id/admin", h.setAdmin)
	g.PUT("/users/:user_id/ban", h.setBanned)
	g.GET("/featured", h.listFeatured)
	g.POST("/featured", h.addFeatured)
	g.PUT("/featured/reorder", h.reorderFeatured)
	g.DELETE("/featured/:id", h.removeFeatured)
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
	// Best-effort: delete storage files before the cascade removes the DB rows.
	deleteBlueprintImageFiles(h.cfg, h.gdb, id)
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

// ── User management ─────────────────────────────────

func (h *AdminHandler) listUsers(c *gin.Context) {
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 20, 1)
	if size > 100 {
		size = 100
	}
	q := strings.TrimSpace(c.Query("q"))

	qry := h.gdb.Model(&db.User{})
	if q != "" {
		like := "%" + escapeLike(q) + "%"
		qry = qry.Where("username LIKE ? OR email LIKE ?", like, like)
	}
	var total int64
	qry.Count(&total)

	var users []db.User
	qry.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&users)

	// Batch blueprint counts per author.
	counts := make(map[string]int, len(users))
	if len(users) > 0 {
		ids := make([]string, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID)
		}
		type cntRow struct {
			AuthorID string
			Cnt      int
		}
		var rows []cntRow
		h.gdb.Model(&db.Blueprint{}).Select("author_id, count(*) as cnt").
			Where("author_id IN ?", ids).Group("author_id").Scan(&rows)
		for _, r := range rows {
			counts[r.AuthorID] = r.Cnt
		}
	}

	items := make([]dto.AdminUserOut, 0, len(users))
	for _, u := range users {
		items = append(items, dto.AdminUserOut{
			ID:             u.ID,
			Username:       u.Username,
			Email:          u.Email,
			AvatarURL:      u.AvatarURL,
			IsAdmin:        u.IsAdmin,
			EmailVerified:  u.EmailVerified,
			Banned:         u.Banned,
			BlueprintCount: counts[u.ID],
			CreatedAt:      dto.ISO(u.CreatedAt),
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func (h *AdminHandler) deleteUser(c *gin.Context) {
	targetID := c.Param("user_id")
	current := auth.CurrentUser(c)
	if targetID == current.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "不能删除自己"})
		return
	}
	var target db.User
	if err := h.gdb.First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "User not found"})
		return
	}
	// Clean storage files for all the user's blueprints before the cascade
	// (the DB cascade only removes BlueprintImage rows, not the physical files).
	var bpIDs []string
	h.gdb.Model(&db.Blueprint{}).Where("author_id = ?", targetID).Pluck("id", &bpIDs)
	for _, bpid := range bpIDs {
		deleteBlueprintImageFiles(h.cfg, h.gdb, bpid)
	}
	if err := h.gdb.Delete(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AdminHandler) setAdmin(c *gin.Context) {
	targetID := c.Param("user_id")
	current := auth.CurrentUser(c)
	if targetID == current.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "不能修改自己的管理员身份"})
		return
	}
	var payload struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	res := h.gdb.Model(&db.User{}).Where("id = ?", targetID).Update("is_admin", payload.IsAdmin)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "User not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *AdminHandler) setBanned(c *gin.Context) {
	targetID := c.Param("user_id")
	current := auth.CurrentUser(c)
	var payload struct {
		Banned bool `json:"banned"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var target db.User
	if err := h.gdb.First(&target, "id = ?", targetID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "User not found"})
		return
	}
	if targetID == current.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "不能禁用自己"})
		return
	}
	if target.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"detail": "不能禁用管理员"})
		return
	}
	updates := map[string]interface{}{"banned": payload.Banned, "banned_at": nil}
	if payload.Banned {
		now := time.Now()
		updates["banned_at"] = &now
	}
	if err := h.gdb.Model(&target).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// ── Featured (home 热门推荐) management ──────────────

// featuredOut is one curated entry with its blueprint preloaded for the admin tab.
func featuredOut(f *db.FeaturedBlueprint, bp *db.Blueprint) gin.H {
	return gin.H{
		"id":           f.ID,
		"blueprint_id": f.BlueprintID,
		"sort_order":   f.SortOrder,
		"created_at":   dto.ISO(f.CreatedAt),
		"blueprint":    toBlueprintOut(bp, 0, 0, false, false),
	}
}

func (h *AdminHandler) listFeatured(c *gin.Context) {
	var featured []db.FeaturedBlueprint
	h.gdb.Order("sort_order ASC, created_at ASC").Find(&featured)
	items := make([]gin.H, 0, len(featured))
	for i := range featured {
		var bp db.Blueprint
		if err := h.gdb.Preload("Author").Preload("Images", db.OrderImages).Preload("Tags.Tag").
			First(&bp, "id = ?", featured[i].BlueprintID).Error; err != nil {
			continue // blueprint deleted; skip the orphaned featured entry
		}
		items = append(items, featuredOut(&featured[i], &bp))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

func (h *AdminHandler) addFeatured(c *gin.Context) {
	var payload struct {
		BlueprintID string `json:"blueprint_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", payload.BlueprintID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	var existing db.FeaturedBlueprint
	if err := h.gdb.Where("blueprint_id = ?", payload.BlueprintID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "Already featured"})
		return
	}
	var maxOrder int
	h.gdb.Model(&db.FeaturedBlueprint{}).Select("COALESCE(MAX(sort_order), -1)").Scan(&maxOrder)
	f := db.FeaturedBlueprint{BlueprintID: payload.BlueprintID, SortOrder: maxOrder + 1}
	if err := h.gdb.Create(&f).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, featuredOut(&f, &bp))
}

func (h *AdminHandler) removeFeatured(c *gin.Context) {
	id := c.Param("id")
	if err := h.gdb.Delete(&db.FeaturedBlueprint{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AdminHandler) reorderFeatured(c *gin.Context) {
	var payload struct {
		Items []struct {
			ID        string `json:"id"`
			SortOrder int    `json:"sort_order"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	for _, it := range payload.Items {
		h.gdb.Model(&db.FeaturedBlueprint{}).Where("id = ?", it.ID).Update("sort_order", it.SortOrder)
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}
