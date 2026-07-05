package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
)

type UsersHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewUsersHandler(cfg *config.Config, gdb *gorm.DB) *UsersHandler {
	return &UsersHandler{cfg: cfg, gdb: gdb}
}

func (h *UsersHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/users")
	g.GET("/:user_id", h.getProfile)
	g.GET("/:user_id/blueprints", auth.OptionalUser(h.cfg, h.gdb), h.getBlueprints)
	g.GET("/:user_id/favorites", auth.OptionalUser(h.cfg, h.gdb), h.getFavorites)
}

// byIdentifier resolves a user by id or (legacy) username. Writes 404 and returns nil on miss.
func (h *UsersHandler) byIdentifier(c *gin.Context, identifier string) *db.User {
	var user db.User
	if err := h.gdb.Where("id = ? OR username = ?", identifier, identifier).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "User not found"})
		return nil
	}
	return &user
}

func (h *UsersHandler) getProfile(c *gin.Context) {
	target := h.byIdentifier(c, c.Param("user_id"))
	if target == nil {
		return
	}
	var bpCount int64
	h.gdb.Model(&db.Blueprint{}).Where("author_id = ? AND is_published = ?", target.ID, true).Count(&bpCount)
	var favCount int64
	h.gdb.Model(&db.Favorite{}).Where("user_id = ?", target.ID).Count(&favCount)
	// Public profile — no email / is_admin.
	c.JSON(http.StatusOK, gin.H{
		"id":             target.ID,
		"username":       target.Username,
		"avatar_url":     target.AvatarURL,
		"bio":            target.Bio,
		"created_at":     dto.ISO(target.CreatedAt),
		"blueprint_count": bpCount,
		"favorite_count": favCount,
	})
}

func (h *UsersHandler) getBlueprints(c *gin.Context) {
	viewer := auth.CurrentUser(c)
	target := h.byIdentifier(c, c.Param("user_id"))
	if target == nil {
		return
	}
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 12, 1)
	if size > 50 {
		size = 50
	}

	// Authorization fix: only the author themselves sees unpublished blueprints.
	isSelf := viewer != nil && viewer.ID == target.ID
	qry := h.gdb.Model(&db.Blueprint{}).Where("author_id = ?", target.ID)
	if !isSelf {
		qry = qry.Where("is_published = ?", true)
	}

	var total int64
	qry.Count(&total)

	var bps []db.Blueprint
	qry.Preload("Author").Preload("Images").Preload("Tags.Tag").
		Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&bps)

	items := make([]dto.BlueprintOut, 0, len(bps))
	for i := range bps {
		items = append(items, *toBlueprintOut(&bps[i], 0, 0, false, false))
	}
	c.JSON(http.StatusOK, dto.BlueprintListOut{Items: items, Total: int(total), Page: page, PageSize: size})
}

func (h *UsersHandler) getFavorites(c *gin.Context) {
	viewer := auth.CurrentUser(c)
	target := h.byIdentifier(c, c.Param("user_id"))
	if target == nil {
		return
	}
	// Privacy fix: favorites are visible only to their owner.
	if viewer == nil || viewer.ID != target.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Favorites are private to the user"})
		return
	}
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 12, 1)
	if size > 50 {
		size = 50
	}

	var total int64
	h.gdb.Model(&db.Favorite{}).Where("user_id = ?", target.ID).Count(&total)

	var bps []db.Blueprint
	h.gdb.Preload("Author").Preload("Images").Preload("Tags.Tag").
		Joins("JOIN favorites ON favorites.blueprint_id = blueprints.id").
		Where("favorites.user_id = ? AND blueprints.is_published = ?", target.ID, true).
		Order("favorites.created_at DESC").
		Offset((page - 1) * size).Limit(size).Find(&bps)

	items := make([]dto.BlueprintOut, 0, len(bps))
	for i := range bps {
		items = append(items, *toBlueprintOut(&bps[i], 0, 0, false, false))
	}
	c.JSON(http.StatusOK, dto.BlueprintListOut{Items: items, Total: int(total), Page: page, PageSize: size})
}
