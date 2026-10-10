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
	g.GET("/comments", h.listComments)
	g.PUT("/comments/:comment_id/pin", h.pinComment)
	g.PUT("/comments/:comment_id/status", h.setCommentStatus)
	g.DELETE("/comments/:comment_id", h.deleteComment)
	g.GET("/users", h.listUsers)
	g.DELETE("/users/:user_id", h.deleteUser)
	g.PUT("/users/:user_id/admin", h.setAdmin)
	g.PUT("/users/:user_id/ban", h.setBanned)
	g.GET("/featured", h.listFeatured)
	g.POST("/featured", h.addFeatured)
	g.PUT("/featured/reorder", h.reorderFeatured)
	g.DELETE("/featured/:id", h.removeFeatured)
	// 扫描本机图纸标题，把最后 () 里的品牌与产品编码回填到 brand / product_code。
	g.POST("/blueprints/backfill-brands", h.backfillBrands)
}

// ── BACKFILL 品牌 / 编号(型号) ─────────────────────────

type backfillBrandReq struct {
	// DryRun previews the extraction without writing. Defaults to true so a
	// no-body call can never mutate data by accident.
	DryRun *bool `json:"dry_run"`
	// Overwrite re-extracts and replaces brand / product_code even on rows that
	// already carry both values. Defaults to false: rows that already have a brand
	// AND a product code are left untouched.
	Overwrite bool `json:"overwrite"`
	// Limit caps how many rows are scanned (0 = all).
	Limit int `json:"limit"`
}

// backfillBrands scans the blueprints table on THIS machine and fills the brand /
// product_code attributes from the manufacturer reference in the last parenthesis
// group of the title, e.g. "钢铁侠 Mark 3 收藏版 (LEGO 76344)" -> 乐高 / 76344.
//
// Rules:
//   - rows that already have BOTH brand and product_code are skipped (no re-extraction)
//   - a row missing only one of the two has just the missing field filled
//   - dry_run defaults to true, so previewing never writes
func (h *AdminHandler) backfillBrands(c *gin.Context) {
	var req backfillBrandReq
	if c.Request != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	qry := h.gdb.Model(&db.Blueprint{}).Order("created_at ASC")
	if req.Limit > 0 {
		qry = qry.Limit(req.Limit)
	}
	var bps []db.Blueprint
	if err := qry.Find(&bps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}

	type item struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Brand       string   `json:"brand"`
		ProductCode string   `json:"product_code"`
		Updated     []string `json:"updated_fields"`
	}
	items := []item{}
	parsed, alreadySet, skipped := 0, 0, 0

	for i := range bps {
		bp := &bps[i]
		if !req.Overwrite && nonEmpty(bp.Brand) && nonEmpty(bp.ProductCode) {
			alreadySet++
			continue
		}
		brand, code, ok := parseBrandFromTitle(bp.Title)
		if !ok {
			continue
		}
		parsed++

		updates := map[string]interface{}{}
		filled := []string{}
		if brand != "" && (req.Overwrite || !nonEmpty(bp.Brand)) {
			updates["brand"] = brand
			filled = append(filled, "brand")
		}
		if code != "" && (req.Overwrite || !nonEmpty(bp.ProductCode)) {
			updates["product_code"] = code
			filled = append(filled, "product_code")
		}
		if len(updates) == 0 {
			skipped++
			continue
		}
		if !dryRun {
			if err := h.gdb.Model(&db.Blueprint{}).Where("id = ?", bp.ID).Updates(updates).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
				return
			}
		}
		items = append(items, item{ID: bp.ID, Title: bp.Title, Brand: brand, ProductCode: code, Updated: filled})
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":     dryRun,
		"scanned":     len(bps),
		"parsed":      parsed,
		"already_set": alreadySet,
		"changed":     len(items),
		"skipped":     skipped,
		"items":       items,
	})
}

// nonEmpty reports whether an optional string column carries a usable value.
func nonEmpty(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
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

// ── Comment moderation ──────────────────────────────

// commentStatuses are the values accepted by setCommentStatus.
var commentStatuses = map[string]bool{"approved": true, "hidden": true}

func toAdminCommentOut(cm *db.Comment) dto.AdminCommentOut {
	title := ""
	if cm.Blueprint != nil {
		title = cm.Blueprint.Title
	}
	return dto.AdminCommentOut{
		ID:             cm.ID,
		BlueprintID:    cm.BlueprintID,
		BlueprintTitle: title,
		UserID:         cm.UserID,
		ParentID:       cm.ParentID,
		Content:        cm.Content,
		Status:         cm.Status,
		IsPinned:       cm.IsPinned,
		CreatedAt:      dto.ISO(cm.CreatedAt),
		User:           dto.FromUser(cm.User),
	}
}

func (h *AdminHandler) listComments(c *gin.Context) {
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 20, 1)
	if size > 100 {
		size = 100
	}
	q := strings.TrimSpace(c.Query("q"))
	status := strings.TrimSpace(c.Query("status"))

	qry := h.gdb.Model(&db.Comment{}).
		Joins("LEFT JOIN users ON users.id = comments.user_id").
		Joins("LEFT JOIN blueprints ON blueprints.id = comments.blueprint_id")
	if q != "" {
		like := "%" + escapeLike(q) + "%"
		qry = qry.Where("comments.content LIKE ? OR users.username LIKE ? OR blueprints.title LIKE ?", like, like, like)
	}
	if commentStatuses[status] {
		qry = qry.Where("comments.status = ?", status)
	}
	var total int64
	qry.Count(&total)

	var comments []db.Comment
	qry.Preload("User").Preload("Blueprint").
		Order("comments.created_at DESC").Offset((page - 1) * size).Limit(size).Find(&comments)

	items := make([]dto.AdminCommentOut, 0, len(comments))
	for i := range comments {
		items = append(items, toAdminCommentOut(&comments[i]))
	}
	c.JSON(http.StatusOK, dto.AdminCommentListOut{Items: items, Total: int(total), Page: page, PageSize: size})
}

func (h *AdminHandler) pinComment(c *gin.Context) {
	var payload struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	res := h.gdb.Model(&db.Comment{}).Where("id = ?", c.Param("comment_id")).
		Update("is_pinned", payload.Pinned)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Comment not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *AdminHandler) setCommentStatus(c *gin.Context) {
	var payload struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if !commentStatuses[payload.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "status must be approved or hidden"})
		return
	}
	id := c.Param("comment_id")
	res := h.gdb.Model(&db.Comment{}).Where("id = ?", id).
		Update("status", payload.Status)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Comment not found"})
		return
	}
	// Hiding a top-level comment also hides its replies, so they don't surface as
	// orphaned replies in the public list. Re-approving restores only the parent;
	// each reply must be approved individually.
	if payload.Status == "hidden" {
		h.gdb.Model(&db.Comment{}).Where("parent_id = ?", id).Update("status", "hidden")
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *AdminHandler) deleteComment(c *gin.Context) {
	id := c.Param("comment_id")
	var cm db.Comment
	if err := h.gdb.First(&cm, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Comment not found"})
		return
	}
	// Delete replies explicitly so the cascade does not depend on DB FK settings.
	h.gdb.Where("parent_id = ?", id).Delete(&db.Comment{})
	if err := h.gdb.Delete(&db.Comment{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

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
