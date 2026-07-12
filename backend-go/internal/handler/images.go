package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
	"brickplans/internal/middleware"
	"brickplans/internal/storage"
	"brickplans/internal/upload"
)

type ImagesHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewImagesHandler(cfg *config.Config, gdb *gorm.DB) *ImagesHandler {
	return &ImagesHandler{cfg: cfg, gdb: gdb}
}

func (h *ImagesHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/blueprints")
	// 500 uploads per 10s per IP: 3000/min (50/s) steady rate, burst 500 lets a
	// fresh IP fire 500 at once. Keyed by ClientIP + route template (see ratelimit.go).
	g.POST("/:blueprint_id/images", auth.AuthRequired(h.cfg, h.gdb), middleware.RateLimit(3000, 500), h.upload)
	g.PUT("/:blueprint_id/images/reorder", auth.AuthRequired(h.cfg, h.gdb), h.reorder)
	g.PUT("/:blueprint_id/images/:image_id/cover", auth.AuthRequired(h.cfg, h.gdb), h.setCover)
	g.DELETE("/:blueprint_id/images/:image_id", auth.AuthRequired(h.cfg, h.gdb), h.delete)
}

func (h *ImagesHandler) upload(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")

	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Only the blueprint author can upload images"})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "No files provided"})
		return
	}
	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "No files provided"})
		return
	}
	if len(files) > upload.MaxFilesPerBatch {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "单次最多上传 10 个文件"})
		return
	}

	st, err := storage.Get(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "storage unavailable"})
		return
	}

	// Compute next sort_order: COALESCE(MAX(sort_order), -1) + 1.
	var maxSort int
	h.gdb.Model(&db.BlueprintImage{}).Where("blueprint_id = ?", id).
		Select("COALESCE(MAX(sort_order), -1)").Scan(&maxSort)
	nextSort := maxSort + 1

	// Process and stage all uploads first; commit DB rows in a single transaction.
	type staged struct {
		url       string
		objectKey string
		fileType  string
	}
	stagedFiles := make([]staged, 0, len(files))
	for _, fh := range files {
		src, err := fh.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "无法读取文件"})
			return
		}
		data, err := io.ReadAll(io.LimitReader(src, upload.MaxImageSize+1))
		src.Close()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "无法读取文件"})
			return
		}
		if len(data) > upload.MaxImageSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"detail": "文件过大，最大 20MB"})
			return
		}
		processed, fileType, contentType, ext, err := upload.ProcessFile(data, fh.Filename)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": err.Error()})
			return
		}
		// "file" + ext → object key uses a forced .jpg/.pdf extension (not user-controlled).
		obj, err := st.Upload(processed, "file"+ext, contentType, "blueprints")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "storage error"})
			return
		}
		stagedFiles = append(stagedFiles, staged{url: obj.URL, objectKey: obj.ObjectKey, fileType: fileType})
	}

	tx := h.gdb.Begin()
	for i, s := range stagedFiles {
		img := db.BlueprintImage{
			BlueprintID: id,
			URL:         s.url,
			ObjectKey:   s.objectKey,
			SortOrder:   nextSort + i,
			FileType:    s.fileType,
		}
		if err := tx.Create(&img).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}

	var imgs []db.BlueprintImage
	h.gdb.Where("blueprint_id = ?", id).Order("sort_order, id").Find(&imgs)
	c.JSON(http.StatusCreated, imagesToOut(imgs))
}

func (h *ImagesHandler) reorder(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Only the blueprint author can manage images"})
		return
	}
	var payload struct {
		Images []struct {
			ID        string `json:"id"`
			SortOrder int    `json:"sort_order"`
		} `json:"images"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	tx := h.gdb.Begin()
	for _, item := range payload.Images {
		if err := tx.Model(&db.BlueprintImage{}).
			Where("id = ? AND blueprint_id = ?", item.ID, id).
			Update("sort_order", item.SortOrder).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
			return
		}
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *ImagesHandler) setCover(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	imgID := c.Param("image_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Only the blueprint author can manage images"})
		return
	}
	tx := h.gdb.Begin()
	// Clear all covers for this blueprint, then set the target.
	if err := tx.Model(&db.BlueprintImage{}).
		Where("blueprint_id = ?", id).Update("is_cover", false).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	res := tx.Model(&db.BlueprintImage{}).
		Where("id = ? AND blueprint_id = ?", imgID, id).Update("is_cover", true)
	if res.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	if res.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"detail": "Image not found"})
		return
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *ImagesHandler) delete(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	imgID := c.Param("image_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Only the blueprint author can manage images"})
		return
	}
	var img db.BlueprintImage
	if err := h.gdb.First(&img, "id = ? AND blueprint_id = ?", imgID, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Image not found"})
		return
	}
	if st, err := storage.Get(h.cfg); err == nil {
		key := img.ObjectKey
		if key == "" {
			key = img.URL
		}
		_ = st.Delete(key)
	}
	if err := h.gdb.Delete(&img).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

func imagesToOut(imgs []db.BlueprintImage) []dto.ImageOut {
	out := make([]dto.ImageOut, 0, len(imgs))
	for _, im := range imgs {
		out = append(out, toImageOut(im))
	}
	return out
}
