package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"brickplans/internal/blog"
	"brickplans/internal/config"
)

// langOf resolves the ?lang= selector. Only an explicit "en" switches trees:
// the SPA predates the English blog and never sends the parameter, so anything
// unrecognised must keep behaving exactly like before.
func langOf(c *gin.Context) string {
	if c.Query("lang") == blog.LangEN {
		return blog.LangEN
	}
	return blog.LangZH
}

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
	Slug        string          `json:"slug"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Date        string          `json:"date"`
	Author      string          `json:"author"`
	Category    string          `json:"category"`
	Tags        []string        `json:"tags"`
	Body        string          `json:"body,omitempty"`
	PrevSlug    string          `json:"prev_slug,omitempty"`
	PrevTitle   string          `json:"prev_title,omitempty"`
	NextSlug    string          `json:"next_slug,omitempty"`
	NextTitle   string          `json:"next_title,omitempty"`
	Related     []BlogPostBrief `json:"related,omitempty"`
}

// BlogPostBrief is a lightweight blog post summary for related posts.
type BlogPostBrief struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Date     string `json:"date"`
}

// List returns one page of blog posts (metadata only, no body).
//
// The blog grew to 1500+ posts; returning all of them cost ~720 KB raw / 215 KB
// gzip per request, which dominated first paint on slow links. Now paginated:
// ?page= (1-based, default 1) and ?page_size= (default blog.DefaultPageSize,
// capped at blog.MaxPageSize).
//
// The top-level "tags" array (every tag in the corpus, ~2500 entries / 46 KB)
// was never consumed by the frontend — only "categories" drives the filter bar —
// so it is now omitted unless explicitly requested with ?with_tags=1.
func (h *BlogAPIHandler) List(c *gin.Context) {
	cat := c.Query("category")
	tag := c.Query("tag")
	page := atoiOr(c.Query("page"), 1, 1)
	size := blog.NormPageSize(atoiOr(c.Query("page_size"), blog.DefaultPageSize, 1))

	// ?lang=en serves the English tree. Default stays zh so the existing SPA
	// client (which never sends the param) is unaffected.
	lang := langOf(c)

	items, total := blog.Paginate(h.store.FilterLang(lang, cat, tag), page, size)

	out := make([]BlogPostOut, 0, len(items))
	for _, p := range items {
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

	resp := gin.H{
		"items":       out,
		"total":       total,
		"page":        page,
		"page_size":   size,
		"total_pages": blog.PageCount(total, size),
		"lang":        lang,
		"categories":  h.store.CategoriesLang(lang),
	}
	if c.Query("with_tags") == "1" {
		resp["tags"] = h.store.TagsLang(lang)
	}

	c.JSON(http.StatusOK, resp)
}

// Detail returns a single blog post with body, prev/next info, and related posts.
func (h *BlogAPIHandler) Detail(c *gin.Context) {
	slug := c.Param("slug")
	post := h.store.GetByLang(slug, langOf(c))
	if post == nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Blog post not found"})
		return
	}

	// Stay inside the language tree: English readers must not be handed Chinese
	// prev/next/related links (and vice versa).
	prev, next := h.store.PrevNextLang(post.Slug, post.Lang)
	relatedPosts := h.store.RelatedLang(post.Slug, post.Lang, 3)

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
	if len(relatedPosts) > 0 {
		out.Related = make([]BlogPostBrief, 0, len(relatedPosts))
		for _, rp := range relatedPosts {
			out.Related = append(out.Related, BlogPostBrief{
				Slug:     rp.Slug,
				Title:    rp.Title,
				Category: rp.Category,
				Date:     rp.Date.Format("2006-01-02"),
			})
		}
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
