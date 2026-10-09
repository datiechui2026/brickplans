package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
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

// platformStats aggregates the public counters. Shared by GET /api/stats and
// the homepage aggregate endpoint so both report identical numbers.
func platformStats(gdb *gorm.DB) dto.PlatformStats {
	var s dto.PlatformStats

	gdb.Model(&db.Blueprint{}).Where("is_published = ?", true).Count(&s.TotalBlueprints)
	gdb.Model(&db.User{}).Count(&s.TotalUsers)
	gdb.Model(&db.Favorite{}).Count(&s.TotalFavorites)
	gdb.Model(&db.Like{}).Count(&s.TotalLikes)
	gdb.Model(&db.Blueprint{}).Where("is_published = ?", true).
		Select("COALESCE(SUM(piece_count), 0)").Scan(&s.TotalPieces)
	gdb.Model(&db.Blueprint{}).
		Select("COALESCE(SUM(view_count), 0)").Scan(&s.TotalViews)

	return s
}

func (h *StatsHandler) get(c *gin.Context) {
	var pending, reportCount int64
	h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", false).Count(&pending)
	h.gdb.Model(&db.Report{}).Distinct("blueprint_id").Count(&reportCount)

	s := platformStats(h.gdb)
	c.JSON(http.StatusOK, gin.H{
		"total_blueprints": s.TotalBlueprints,
		"total_users":      s.TotalUsers,
		"total_favorites":  s.TotalFavorites,
		"total_pieces":     s.TotalPieces,
		"total_views":      s.TotalViews,
		"total_likes":      s.TotalLikes,
		"pending_count":    pending,
		"report_count":     reportCount,
	})
}
