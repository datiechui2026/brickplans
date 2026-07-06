package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
)

var (
	nonAlphaNumDash = regexp.MustCompile(`[^a-z0-9-]`)
	multiDash       = regexp.MustCompile(`-{2,}`)
)

// slugify mirrors the Python _slugify: lowercase, non [a-z0-9-] → "-", collapse, trim.
// Note: pure-CJK titles collapse to "" — the frontend addresses blueprints by id,
// not slug, so an empty slug is acceptable (and matches the prior behavior).
func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlphaNumDash.ReplaceAllString(s, "-")
	s = multiDash.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// escapeLike escapes LIKE wildcards so user input can't broaden the match.
// MySQL's default LIKE escape character is backslash.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// viewDedup prevents a single IP from inflating a blueprint's view_count by
// repeated requests within a short window.
type viewDedup struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func newViewDedup() *viewDedup {
	v := &viewDedup{m: make(map[string]time.Time)}
	go v.sweep()
	return v
}

func (v *viewDedup) allow(key string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	if t, ok := v.m[key]; ok && now.Sub(t) < 5*time.Minute {
		return false
	}
	v.m[key] = now
	return true
}

func (v *viewDedup) sweep() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		v.mu.Lock()
		for k, t := range v.m {
			if t.Before(cutoff) {
				delete(v.m, k)
			}
		}
		v.mu.Unlock()
	}
}

type BlueprintsHandler struct {
	cfg   *config.Config
	gdb   *gorm.DB
	views *viewDedup
}

func NewBlueprintsHandler(cfg *config.Config, gdb *gorm.DB) *BlueprintsHandler {
	return &BlueprintsHandler{cfg: cfg, gdb: gdb, views: newViewDedup()}
}

func (h *BlueprintsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/blueprints")
	g.POST("", auth.AuthRequired(h.cfg, h.gdb), h.create)
	g.GET("", auth.OptionalUser(h.cfg, h.gdb), h.list)
	g.GET("/:blueprint_id", auth.OptionalUser(h.cfg, h.gdb), h.get)
	g.PUT("/:blueprint_id", auth.AuthRequired(h.cfg, h.gdb), h.update)
	g.DELETE("/:blueprint_id", auth.AuthRequired(h.cfg, h.gdb), h.delete)
	g.POST("/:blueprint_id/favorite", auth.AuthRequired(h.cfg, h.gdb), h.favorite)
	g.DELETE("/:blueprint_id/favorite", auth.AuthRequired(h.cfg, h.gdb), h.unfavorite)
	g.POST("/:blueprint_id/like", auth.AuthRequired(h.cfg, h.gdb), h.like)
	g.DELETE("/:blueprint_id/like", auth.AuthRequired(h.cfg, h.gdb), h.unlike)
	g.POST("/:blueprint_id/comments", auth.AuthRequired(h.cfg, h.gdb), h.createComment)
	g.GET("/:blueprint_id/comments", h.listComments)
	g.GET("/:blueprint_id/related", h.related)
}

// ── CREATE ──────────────────────────────────────────

type blueprintCreateReq struct {
	Title       string          `json:"title" binding:"required,min=1,max=100"`
	Description *string         `json:"description"`
	Difficulty  *int            `json:"difficulty"`
	PieceCount  *int            `json:"piece_count"`
	Category    *string         `json:"category"`
	Dimensions  *string         `json:"dimensions"`
	PartList    json.RawMessage `json:"part_list"`
	IsPublished *bool           `json:"is_published"`
}

func (h *BlueprintsHandler) create(c *gin.Context) {
	user := auth.CurrentUser(c)
	var req blueprintCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.Difficulty != nil && (*req.Difficulty < 1 || *req.Difficulty > 5) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "difficulty must be 1-5"})
		return
	}
	bp := db.Blueprint{
		Title:       req.Title,
		Slug:        slugify(req.Title),
		Description: req.Description,
		Difficulty:  req.Difficulty,
		PieceCount:  req.PieceCount,
		Category:    req.Category,
		Dimensions:  req.Dimensions,
		PartList:    req.PartList,
		IsPublished: true,
		AuthorID:    user.ID,
	}
	if req.IsPublished != nil {
		bp.IsPublished = *req.IsPublished
	}
	if err := h.gdb.Create(&bp).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	if err := h.gdb.Preload("Author").First(&bp, "id = ?", bp.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, toBlueprintOut(&bp, 0, 0, false, false))
}

// ── READ (detail) ───────────────────────────────────

func (h *BlueprintsHandler) get(c *gin.Context) {
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	err := h.gdb.Preload("Author").Preload("Images").Preload("Tags.Tag").
		First(&bp, "id = ?", id).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	user := auth.CurrentUser(c)
	if !bp.IsPublished {
		if user == nil || user.ID != bp.AuthorID {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
			return
		}
	}
	isFav := false
	if user != nil {
		var fav db.Favorite
		if h.gdb.Where("user_id = ? AND blueprint_id = ?", user.ID, id).First(&fav).Error == nil {
			isFav = true
		}
	}
	var favCount int64
	h.gdb.Model(&db.Favorite{}).Where("blueprint_id = ?", id).Count(&favCount)

	if h.views.allow(c.ClientIP() + "|" + id) {
		h.gdb.Model(&db.Blueprint{}).Where("id = ?", id).
			UpdateColumn("view_count", gorm.Expr("view_count + 1"))
		bp.ViewCount++
	}
	c.JSON(http.StatusOK, toBlueprintDetail(&bp, isFav, int(favCount)))
}

// ── UPDATE ──────────────────────────────────────────

type blueprintUpdateReq struct {
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	Difficulty  *int            `json:"difficulty"`
	PieceCount  *int            `json:"piece_count"`
	Category    *string         `json:"category"`
	Dimensions  *string         `json:"dimensions"`
	PartList    json.RawMessage `json:"part_list"`
	IsPublished *bool           `json:"is_published"`
}

func (h *BlueprintsHandler) update(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Not authorized to edit this blueprint"})
		return
	}
	var req blueprintUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.Difficulty != nil && (*req.Difficulty < 1 || *req.Difficulty > 5) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "difficulty must be 1-5"})
		return
	}
	if req.Title != nil {
		if len(*req.Title) < 1 || len(*req.Title) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "title must be 1-100 chars"})
			return
		}
		bp.Title = *req.Title
		bp.Slug = slugify(*req.Title)
	}
	if req.Description != nil {
		bp.Description = req.Description
	}
	if req.Difficulty != nil {
		bp.Difficulty = req.Difficulty
	}
	if req.PieceCount != nil {
		bp.PieceCount = req.PieceCount
	}
	if req.Category != nil {
		bp.Category = req.Category
	}
	if req.Dimensions != nil {
		bp.Dimensions = req.Dimensions
	}
	if req.PartList != nil {
		bp.PartList = req.PartList
	}
	if req.IsPublished != nil {
		bp.IsPublished = *req.IsPublished
	}
	if err := h.gdb.Save(&bp).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	h.gdb.Preload("Author").First(&bp, "id = ?", bp.ID)
	c.JSON(http.StatusOK, toBlueprintOut(&bp, 0, 0, false, false))
}

// ── DELETE ──────────────────────────────────────────

func (h *BlueprintsHandler) delete(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	if bp.AuthorID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"detail": "Not authorized to delete this blueprint"})
		return
	}
	// Best-effort: delete uploaded image files from storage before the cascade.
	var imgs []db.BlueprintImage
	h.gdb.Where("blueprint_id = ?", id).Find(&imgs)
	// (storage deletion is best-effort; image handler owns object keys — skipped here
	// to avoid a circular import; orphaned files are acceptable and cleaned manually.)
	if err := h.gdb.Delete(&bp).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── LIST ────────────────────────────────────────────

func (h *BlueprintsHandler) list(c *gin.Context) {
	page := atoiOr(c.Query("page"), 1, 1)
	size := atoiOr(c.Query("size"), 12, 12)
	if size > 50 {
		size = 50
	}
	q := strings.TrimSpace(c.Query("q"))
	category := c.Query("category")
	sort := c.Query("sort")
	tag := c.Query("tag")
	user := auth.CurrentUser(c)

	qry := h.gdb.Model(&db.Blueprint{}).Where("is_published = ?", true)
	if q != "" {
		like := "%" + escapeLike(q) + "%"
		qry = qry.Where("title LIKE ? OR description LIKE ?", like, like)
	}
	if category != "" {
		qry = qry.Where("category = ?", category)
	}
	if tag != "" {
		qry = qry.Where("id IN (SELECT bt.blueprint_id FROM blueprint_tags bt JOIN tags t ON t.id = bt.tag_id WHERE t.name = ?)", tag)
	}

	var total int64
	qry.Count(&total)

	if sort == "popular" {
		qry = qry.Order("view_count DESC")
	} else {
		qry = qry.Order("created_at DESC")
	}

	var bps []db.Blueprint
	if err := qry.Preload("Author").Preload("Images").Preload("Tags.Tag").
		Offset((page - 1) * size).Limit(size).Find(&bps).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}

	bpIDs := make([]string, 0, len(bps))
	for _, b := range bps {
		bpIDs = append(bpIDs, b.ID)
	}
	favCounts, likeCounts, userLiked, userFav := h.bulkCounts(bpIDs, user)

	items := make([]dto.BlueprintOut, 0, len(bps))
	for i := range bps {
		items = append(items, *toBlueprintOut(&bps[i], favCounts[bps[i].ID], likeCounts[bps[i].ID], userLiked[bps[i].ID], userFav[bps[i].ID]))
	}
	c.JSON(http.StatusOK, dto.BlueprintListOut{Items: items, Total: int(total), Page: page, PageSize: size})
}

func (h *BlueprintsHandler) bulkCounts(bpIDs []string, user *db.User) (favCounts, likeCounts map[string]int, userLiked, userFav map[string]bool) {
	favCounts = map[string]int{}
	likeCounts = map[string]int{}
	userLiked = map[string]bool{}
	userFav = map[string]bool{}
	if len(bpIDs) == 0 {
		return
	}
	type cntRow struct {
		BlueprintID string
		Cnt         int
	}
	var favRows []cntRow
	h.gdb.Model(&db.Favorite{}).Select("blueprint_id, count(*) as cnt").
		Where("blueprint_id IN ?", bpIDs).Group("blueprint_id").Scan(&favRows)
	for _, r := range favRows {
		favCounts[r.BlueprintID] = r.Cnt
	}
	var likeRows []cntRow
	h.gdb.Model(&db.Like{}).Select("blueprint_id, count(*) as cnt").
		Where("blueprint_id IN ?", bpIDs).Group("blueprint_id").Scan(&likeRows)
	for _, r := range likeRows {
		likeCounts[r.BlueprintID] = r.Cnt
	}
	if user != nil {
		var likedIDs []string
		h.gdb.Model(&db.Like{}).Where("blueprint_id IN ? AND user_id = ?", bpIDs, user.ID).Pluck("blueprint_id", &likedIDs)
		for _, id := range likedIDs {
			userLiked[id] = true
		}
		var favIDs []string
		h.gdb.Model(&db.Favorite{}).Where("blueprint_id IN ? AND user_id = ?", bpIDs, user.ID).Pluck("blueprint_id", &favIDs)
		for _, id := range favIDs {
			userFav[id] = true
		}
	}
	return
}

// ── FAVORITE ────────────────────────────────────────

func (h *BlueprintsHandler) favorite(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	var existing db.Favorite
	if h.gdb.Where("user_id = ? AND blueprint_id = ?", user.ID, id).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "Already favorited"})
		return
	}
	fav := db.Favorite{UserID: user.ID, BlueprintID: id}
	if err := h.gdb.Create(&fav).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	addNotification(h.gdb, bp.AuthorID, user.ID, "favorite", &bp.ID, nil, map[string]interface{}{"blueprint_title": bp.Title})
	c.JSON(http.StatusCreated, gin.H{"detail": "Favorited"})
}

func (h *BlueprintsHandler) unfavorite(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	res := h.gdb.Where("user_id = ? AND blueprint_id = ?", user.ID, id).Delete(&db.Favorite{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Not favorited"})
		return
	}
	c.Status(http.StatusNoContent)
}

// ── LIKE ────────────────────────────────────────────

func (h *BlueprintsHandler) like(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	var existing db.Like
	if h.gdb.Where("user_id = ? AND blueprint_id = ?", user.ID, id).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "Already liked"})
		return
	}
	like := db.Like{UserID: user.ID, BlueprintID: id}
	if err := h.gdb.Create(&like).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	h.gdb.Model(&db.Blueprint{}).Where("id = ?", id).UpdateColumn("like_count", gorm.Expr("like_count + 1"))
	bp.LikeCount++
	addNotification(h.gdb, bp.AuthorID, user.ID, "like", &bp.ID, nil, map[string]interface{}{"blueprint_title": bp.Title})
	c.JSON(http.StatusCreated, gin.H{"detail": "Liked", "like_count": bp.LikeCount})
}

func (h *BlueprintsHandler) unlike(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	res := h.gdb.Where("user_id = ? AND blueprint_id = ?", user.ID, id).Delete(&db.Like{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Not liked"})
		return
	}
	h.gdb.Model(&db.Blueprint{}).Where("id = ? AND like_count > 0", id).
		UpdateColumn("like_count", gorm.Expr("like_count - 1"))
	c.Status(http.StatusNoContent)
}

// ── COMMENTS ────────────────────────────────────────

type commentCreateReq struct {
	Content  string  `json:"content" binding:"required,min=1,max=2000"`
	ParentID *string `json:"parent_id"`
}

func (h *BlueprintsHandler) createComment(c *gin.Context) {
	user := auth.CurrentUser(c)
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	var req commentCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var parent *db.Comment
	if req.ParentID != nil && *req.ParentID != "" {
		var p db.Comment
		if err := h.gdb.First(&p, "id = ?", *req.ParentID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Parent comment not found"})
			return
		}
		if p.BlueprintID != id {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Parent comment not found"})
			return
		}
		if p.ParentID != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": "Only one-level replies are supported"})
			return
		}
		parent = &p
	}
	comment := db.Comment{BlueprintID: id, UserID: user.ID, ParentID: req.ParentID, Content: req.Content}
	if err := h.gdb.Create(&comment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	excerpt := req.Content
	if len(excerpt) > 80 {
		excerpt = excerpt[:80]
	}
	payload := map[string]interface{}{"blueprint_title": bp.Title, "comment_excerpt": excerpt}
	if parent != nil {
		addNotification(h.gdb, parent.UserID, user.ID, "comment_reply", &bp.ID, &comment.ID, payload)
	} else {
		addNotification(h.gdb, bp.AuthorID, user.ID, "comment", &bp.ID, &comment.ID, payload)
	}
	h.gdb.Preload("User").First(&comment, "id = ?", comment.ID)
	c.JSON(http.StatusCreated, toCommentOut(&comment))
}

func (h *BlueprintsHandler) listComments(c *gin.Context) {
	id := c.Param("blueprint_id")
	var comments []db.Comment
	h.gdb.Preload("User").Where("blueprint_id = ?", id).Order("created_at ASC").Find(&comments)
	out := make([]dto.CommentOut, 0, len(comments))
	for i := range comments {
		out = append(out, *toCommentOut(&comments[i]))
	}
	c.JSON(http.StatusOK, out)
}

// related returns up to 6 published blueprints in the same category (fallback:
// same author), excluding the current one — powers the "相关作品" internal links
// on the detail page for SEO/GEO discoverability.
func (h *BlueprintsHandler) related(c *gin.Context) {
	id := c.Param("blueprint_id")
	var bp db.Blueprint
	if err := h.gdb.First(&bp, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blueprint not found"})
		return
	}
	q := h.gdb.Where("is_published = ? AND id <> ?", true, id)
	if bp.Category != nil && *bp.Category != "" {
		q = q.Where("category = ?", *bp.Category)
	} else {
		q = q.Where("author_id = ?", bp.AuthorID)
	}
	var bps []db.Blueprint
	q.Preload("Author").Preload("Images").Preload("Tags.Tag").
		Order("view_count DESC").Limit(6).Find(&bps)
	items := make([]dto.BlueprintOut, 0, len(bps))
	for i := range bps {
		items = append(items, *toBlueprintOut(&bps[i], 0, 0, false, false))
	}
	c.JSON(http.StatusOK, dto.BlueprintListOut{Items: items, Total: len(items), Page: 1, PageSize: 6})
}

// ── Serialization ───────────────────────────────────

func toBlueprintOut(bp *db.Blueprint, favCount, likeCount int, isLiked, isFav bool) *dto.BlueprintOut {
	var modStatus *string
	if !bp.IsPublished {
		s := "审核中"
		modStatus = &s
	}
	// Match Python: prefer the freshly-computed like_count, fall back to stored.
	lc := bp.LikeCount
	if likeCount > 0 {
		lc = likeCount
	}
	tags := []string{}
	for _, bt := range bp.Tags {
		if bt.Tag != nil {
			tags = append(tags, bt.Tag.Name)
		}
	}
	images := make([]dto.ImageOut, 0, len(bp.Images))
	for _, im := range bp.Images {
		images = append(images, toImageOut(im))
	}
	var partList json.RawMessage
	if len(bp.PartList) > 0 {
		partList = bp.PartList
	}
	return &dto.BlueprintOut{
		ID:               bp.ID,
		AuthorID:         bp.AuthorID,
		Title:            bp.Title,
		Slug:             bp.Slug,
		Description:      bp.Description,
		Difficulty:       bp.Difficulty,
		PieceCount:       bp.PieceCount,
		Category:         bp.Category,
		Dimensions:       bp.Dimensions,
		PartList:         partList,
		ViewCount:        bp.ViewCount,
		LikeCount:        lc,
		FavoriteCount:    favCount,
		IsLiked:          isLiked,
		CoverURL:         computeCover(bp.Images),
		IsPublished:      bp.IsPublished,
		CreatedAt:        dto.ISO(bp.CreatedAt),
		UpdatedAt:        dto.ISO(bp.UpdatedAt),
		Author:           dto.FromUser(bp.Author),
		Images:           images,
		Tags:             tags,
		ModerationStatus: modStatus,
	}
}

func toBlueprintDetail(bp *db.Blueprint, isFav bool, favCount int) *dto.BlueprintDetail {
	out := toBlueprintOut(bp, favCount, 0, false, false)
	return &dto.BlueprintDetail{BlueprintOut: *out, IsFavorited: isFav}
}

func toImageOut(img db.BlueprintImage) dto.ImageOut {
	ft := img.FileType
	if ft == "" {
		ft = "image"
	}
	return dto.ImageOut{
		ID:          img.ID,
		BlueprintID: img.BlueprintID,
		URL:         img.URL,
		SortOrder:   img.SortOrder,
		IsCover:     img.IsCover,
		FileType:    ft,
	}
}

func toCommentOut(comment *db.Comment) *dto.CommentOut {
	return &dto.CommentOut{
		ID:          comment.ID,
		BlueprintID: comment.BlueprintID,
		UserID:      comment.UserID,
		ParentID:    comment.ParentID,
		Content:     comment.Content,
		CreatedAt:   dto.ISO(comment.CreatedAt),
		User:        dto.FromUser(comment.User),
	}
}

// computeCover mirrors the Python logic: prefer the marked cover (non-PDF),
// else the first non-PDF image, else the first image.
func computeCover(imgs []db.BlueprintImage) *string {
	var firstNonPDF *string
	for _, im := range imgs {
		ft := im.FileType
		if ft == "" {
			ft = "image"
		}
		if ft == "pdf" {
			continue
		}
		u := im.URL
		if firstNonPDF == nil {
			firstNonPDF = &u
		}
		if im.IsCover {
			return &u
		}
	}
	if firstNonPDF != nil {
		return firstNonPDF
	}
	if len(imgs) > 0 {
		u := imgs[0].URL
		return &u
	}
	return nil
}

// addNotification creates a notification unless the actor is the target user.
func addNotification(gdb *gorm.DB, userID, actorID, typ string, blueprintID, commentID *string, payload map[string]interface{}) {
	if userID == actorID {
		return
	}
	var raw json.RawMessage
	if payload != nil {
		b, _ := json.Marshal(payload)
		raw = b
	}
	actor := actorID
	n := db.Notification{
		UserID:      userID,
		ActorID:     &actor,
		Type:        typ,
		BlueprintID: blueprintID,
		CommentID:   commentID,
		Payload:     raw,
	}
	gdb.Create(&n)
}

func atoiOr(s string, def, min int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < min {
		return def
	}
	return n
}
