package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
)

type StatsHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewStatsHandler(cfg *config.Config, gdb *gorm.DB) *StatsHandler {
	return &StatsHandler{cfg: cfg, gdb: gdb}
}

func (h *StatsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stats", h.get)
}

func (h *StatsHandler) get(c *gin.Context) {
	var totalBlueprints, totalUsers, totalFavorites, totalLikes, pending, reportCount int64
	var totalPieces, totalViews int64

	h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", true).Count(&totalBlueprints)
	h.gdb.Model(&db.User{}).Count(&totalUsers)
	h.gdb.Model(&db.Favorite{}).Count(&totalFavorites)
	h.gdb.Model(&db.Like{}).Count(&totalLikes)
	h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", false).Count(&pending)
	h.gdb.Model(&db.Report{}).Distinct("blueprint_id").Count(&reportCount)

	h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", true).
		Select("COALESCE(SUM(piece_count), 0)").Scan(&totalPieces)
	h.gdb.Model(&db.Blueprint{}).
		Select("COALESCE(SUM(view_count), 0)").Scan(&totalViews)

	c.JSON(http.StatusOK, gin.H{
		"total_blueprints": totalBlueprints,
		"total_users":      totalUsers,
		"total_favorites":  totalFavorites,
		"total_pieces":     totalPieces,
		"total_views":      totalViews,
		"total_likes":      totalLikes,
		"pending_count":    pending,
		"report_count":     reportCount,
	})
}
