package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
	"brickplans/internal/config"
)

// BlogAPIHandler serves blog data as JSON for the SPA frontend.
type BlogAPIHandler struct {
	cfg   *config.Config
	store *blog.Store
}

func NewBlogAPIHandler(cfg *config.Config, store *blog.Store) *BlogAPIHandler {
	return &BlogAPIHandler{cfg: cfg, store: store}
}

func (h *BlogAPIHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/blog", h.List)
	rg.GET("/blog/:slug", h.Detail)
}

// BlogPostOut is the JSON response for a blog post.
type BlogPostOut struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Date        string   `json:"date"`
	Author      string   `json:"author"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Body        string   `json:"body,omitempty"`
	PrevSlug    string   `json:"prev_slug,omitempty"`
	PrevTitle   string   `json:"prev_title,omitempty"`
	NextSlug    string   `json:"next_slug,omitempty"`
	NextTitle   string   `json:"next_title,omitempty"`
}

// List returns all blog posts (metadata only, no body).
func (h *BlogAPIHandler) List(c *gin.Context) {
	cat := c.Query("category")
	tag := c.Query("tag")

	var posts []blog.Post
	if cat != "" {
		posts = h.store.FilterByCategory(cat)
	} else if tag != "" {
		posts = h.store.FilterByTag(tag)
	} else {
		posts = h.store.All()
	}

	out := make([]BlogPostOut, 0, len(posts))
	for _, p := range posts {
		out = append(out, BlogPostOut{
			Slug:        p.Slug,
			Title:       p.Title,
			Description: p.Description,
			Date:        p.Date.Format("2006-01-02"),
			Author:      p.Author,
			Category:    p.Category,
			Tags:        p.Tags,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":      out,
		"categories": h.store.Categories(),
		"tags":       h.store.Tags(),
	})
}

// Detail returns a single blog post with body and prev/next info.
func (h *BlogAPIHandler) Detail(c *gin.Context) {
	slug := c.Param("slug")
	post := h.store.Get(slug)
	if post == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blog post not found"})
		return
	}

	prev, next := h.store.PrevNext(slug)

	out := BlogPostOut{
		Slug:        post.Slug,
		Title:       post.Title,
		Description: post.Description,
		Date:        post.Date.Format("2006-01-02"),
		Author:      post.Author,
		Category:    post.Category,
		Tags:        post.Tags,
		Body:        post.Body,
	}
	if prev != nil {
		out.PrevSlug = prev.Slug
		out.PrevTitle = prev.Title
	}
	if next != nil {
		out.NextSlug = next.Slug
		out.NextTitle = next.Title
	}

	c.JSON(http.StatusOK, out)
}

// formatDate ensures consistent date formatting
func formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// ensure non-empty tags slice
func ensureTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

// used to avoid unused import warnings during development
var _ = strings.TrimSpace
