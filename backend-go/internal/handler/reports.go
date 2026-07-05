package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
	"brickplans/internal/middleware"
)

var validReportReasons = map[string]bool{
	"inappropriate": true, "copyright": true, "incomplete": true, "spam": true, "other": true,
}

type ReportsHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewReportsHandler(cfg *config.Config, gdb *gorm.DB) *ReportsHandler {
	return &ReportsHandler{cfg: cfg, gdb: gdb}
}

func (h *ReportsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/reports", auth.AuthRequired(h.cfg, h.gdb), middleware.RateLimit(10, 10), h.create)
}

type reportCreateReq struct {
	BlueprintID string  `json:"blueprint_id" binding:"required"`
	Reason      string  `json:"reason" binding:"required"`
	Detail      *string `json:"detail"`
}

func (h *ReportsHandler) create(c *gin.Context) {
	user := auth.CurrentUser(c)
	var req reportCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if len(req.Reason) > 20 || !validReportReasons[req.Reason] {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "Invalid reason"})
		return
	}
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", req.BlueprintID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	var existing db.Report
	if h.gdb.Where("reporter_id = ? AND blueprint_id = ?", user.ID, req.BlueprintID).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "You have already reported this blueprint"})
		return
	}
	report := db.Report{
		ReporterID:  user.ID,
		BlueprintID: req.BlueprintID,
		Reason:      req.Reason,
		Detail:      req.Detail,
		Status:      "pending",
	}
	if err := h.gdb.Create(&report).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	// Security fix: reports no longer auto-unpublish the blueprint. They enter the
	// admin review queue and a moderator decides. This prevents 3-account griefing.
	c.JSON(http.StatusCreated, gin.H{
		"id":           report.ID,
		"reporter_id":  report.ReporterID,
		"blueprint_id": report.BlueprintID,
		"reason":       report.Reason,
		"detail":       report.Detail,
		"status":       report.Status,
		"created_at":   dto.ISO(report.CreatedAt),
	})
}
