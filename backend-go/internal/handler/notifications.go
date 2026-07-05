package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
)

type NotificationsHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewNotificationsHandler(cfg *config.Config, gdb *gorm.DB) *NotificationsHandler {
	return &NotificationsHandler{cfg: cfg, gdb: gdb}
}

func (h *NotificationsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/notifications", auth.AuthRequired(h.cfg, h.gdb))
	g.GET("", h.list)
	g.GET("/unread-count", h.unreadCount)
	g.POST("/mark-read", h.markRead)
}

func (h *NotificationsHandler) list(c *gin.Context) {
	user := auth.CurrentUser(c)
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 20, 1)
	if size > 100 {
		size = 100
	}

	var total int64
	h.gdb.Model(&db.Notification{}).Where("user_id = ?", user.ID).Count(&total)
	var unread int64
	h.gdb.Model(&db.Notification{}).Where("user_id = ? AND is_read = ?", user.ID, false).Count(&unread)

	var items []db.Notification
	h.gdb.Preload("Actor").Where("user_id = ?", user.ID).
		Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&items)

	out := make([]dto.NotificationOut, 0, len(items))
	for i := range items {
		out = append(out, *toNotificationOut(&items[i]))
	}
	c.JSON(http.StatusOK, dto.NotificationListOut{
		Items:       out,
		Total:       int(total),
		UnreadCount: int(unread),
		Page:        page,
		PageSize:    size,
	})
}

func (h *NotificationsHandler) unreadCount(c *gin.Context) {
	user := auth.CurrentUser(c)
	var count int64
	h.gdb.Model(&db.Notification{}).Where("user_id = ? AND is_read = ?", user.ID, false).Count(&count)
	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}

func (h *NotificationsHandler) markRead(c *gin.Context) {
	user := auth.CurrentUser(c)
	now := time.Now().UTC()
	h.gdb.Model(&db.Notification{}).
		Where("user_id = ? AND is_read = ?", user.ID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})
	c.JSON(http.StatusOK, gin.H{"detail": "Marked as read"})
}

func toNotificationOut(n *db.Notification) *dto.NotificationOut {
	var readAt *string
	if n.ReadAt != nil {
		s := dto.ISO(*n.ReadAt)
		readAt = &s
	}
	return &dto.NotificationOut{
		ID:          n.ID,
		UserID:      n.UserID,
		ActorID:     n.ActorID,
		Type:        n.Type,
		BlueprintID: n.BlueprintID,
		CommentID:   n.CommentID,
		Payload:     n.Payload,
		IsRead:      n.IsRead,
		CreatedAt:   dto.ISO(n.CreatedAt),
		ReadAt:      readAt,
		Actor:       dto.FromUser(n.Actor),
	}
}
