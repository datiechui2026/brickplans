package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
)

type TagsHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewTagsHandler(cfg *config.Config, gdb *gorm.DB) *TagsHandler {
	return &TagsHandler{cfg: cfg, gdb: gdb}
}

func (h *TagsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/tags", h.listAll)
	bp := rg.Group("/blueprints")
	bp.GET("/:blueprint_id/tags", h.getBlueprintTags)
	bp.POST("/:blueprint_id/tags", auth.AuthRequired(h.cfg, h.gdb), h.bindTags)
	bp.DELETE("/:blueprint_id/tags/:tag_id", auth.AuthRequired(h.cfg, h.gdb), h.removeTag)
}

func (h *TagsHandler) listAll(c *gin.Context) {
	var tags []db.Tag
	h.gdb.Order("name ASC").Find(&tags)
	out := make([]gin.H, 0, len(tags))
	for _, t := range tags {
		out = append(out, gin.H{"id": t.ID, "name": t.Name})
	}
	c.JSON(http.StatusOK, out)
}

func (h *TagsHandler) getBlueprintTags(c *gin.Context) {
	id := c.Param("blueprint_id")
	if err := h.gdb.First(&db.Blueprint{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	var tags []db.Tag
	h.gdb.Joins("JOIN blueprint_tags ON blueprint_tags.tag_id = tags.id").
		Where("blueprint_tags.blueprint_id = ?", id).
		Order("tags.name ASC").Find(&tags)
	out := make([]gin.H, 0, len(tags))
	for _, t := range tags {
		out = append(out, gin.H{"id": t.ID, "name": t.Name})
	}
	c.JSON(http.StatusOK, out)
}

type tagBindReq struct {
	Tags []string `json:"tags" binding:"required,min=1,max=20"`
}

func (h *TagsHandler) bindTags(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Only the blueprint author can manage tags"})
		return
	}
	var req tagBindReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	// Normalize: trim, dedupe (preserve order), enforce length.
	names := make([]string, 0, len(req.Tags))
	seen := map[string]bool{}
	for _, n := range req.Tags {
		n = strings.TrimSpace(n)
		if n == "" || len(n) > 30 || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	if len(names) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "no valid tags"})
		return
	}

	tx := h.gdb.Begin()
	// Find existing tags.
	var existing []db.Tag
	tx.Where("name IN ?", names).Find(&existing)
	byName := map[string]*db.Tag{}
	for i := range existing {
		byName[existing[i].Name] = &existing[i]
	}
	// Create missing tags.
	for _, n := range names {
		if _, ok := byName[n]; !ok {
			t := db.Tag{Name: n}
			if err := tx.Create(&t).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
				return
			}
			byName[n] = &t
		}
	}
	// Find existing associations for this blueprint.
	var existingBT []db.BlueprintTag
	tx.Where("blueprint_id = ?", id).Find(&existingBT)
	haveTag := map[string]bool{}
	for _, bt := range existingBT {
		haveTag[bt.TagID] = true
	}
	for _, n := range names {
		t := byName[n]
		if !haveTag[t.ID] {
			if err := tx.Create(&db.BlueprintTag{BlueprintID: id, TagID: t.ID}).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
				return
			}
		}
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	// Return all tags for this blueprint.
	var tags []db.Tag
	h.gdb.Joins("JOIN blueprint_tags ON blueprint_tags.tag_id = tags.id").
		Where("blueprint_tags.blueprint_id = ?", id).
		Order("tags.name ASC").Find(&tags)
	out := make([]gin.H, 0, len(tags))
	for _, t := range tags {
		out = append(out, gin.H{"id": t.ID, "name": t.Name})
	}
	c.JSON(http.StatusCreated, out)
}

func (h *TagsHandler) removeTag(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	tagID := c.Param("tag_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Only the blueprint author can manage tags"})
		return
	}
	res := h.gdb.Where("blueprint_id = ? AND tag_id = ?", id, tagID).Delete(&db.BlueprintTag{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Tag not found on this blueprint"})
		return
	}
	c.Status(http.StatusNoContent)
}
