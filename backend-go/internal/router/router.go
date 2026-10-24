package router

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/blog"
	"brickplans/internal/config"
	"brickplans/internal/handler"
	"brickplans/internal/middleware"
	"brickplans/internal/ssr"
	"brickplans/internal/storage"
)

// New assembles the gin engine with global middleware and all route groups, and
// returns the handler the HTTP server should serve.
//
// The return type is http.Handler rather than *gin.Engine on purpose: HEAD has
// to be rewritten to GET *before* gin routes the request (see
// middleware.HeadToGet), which is impossible from inside the gin chain. Tests get
// the same wrapper, so what they assert is what production serves.
// ssrRenderer renders server-side HTML for page routes (SEO/GEO).
func New(cfg *config.Config, gdb *gorm.DB, ssrRenderer *ssr.Renderer) http.Handler {
	return middleware.HeadToGet(newEngine(cfg, gdb, ssrRenderer))
}

func newEngine(cfg *config.Config, gdb *gorm.DB, ssrRenderer *ssr.Renderer) *gin.Engine {
	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.SecurityHeaders(cfg))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))
	r.Use(gin.Logger())

	// Multipart upload memory threshold; larger parts spill to temp files.
	r.MaxMultipartMemory = 32 << 20

	// Load blog content store (non-critical: log warning on failure).
	blogStore, blogErr := blog.LoadDirs(cfg.BlogContentDir, cfg.BlogContentDirEN)
	if blogErr != nil {
		fmt.Printf("[blog] warning: failed to load blog content from %s (+ %s): %v\n", cfg.BlogContentDir, cfg.BlogContentDirEN, blogErr)
	}
	// Intrinsic image sizes for the <img width/height> decoration done by the
	// SSR markdown renderer. Lives next to the content dirs; a missing manifest
	// only costs the CLS win, so it must never block startup.
	if err := ssr.LoadImageDims(filepath.Join(filepath.Dir(cfg.BlogContentDir), "image-dims.json")); err != nil {
		fmt.Printf("[ssr] warning: image dims manifest not loaded (%v); <img> will ship without width/height\n", err)
	}
	// English sitemap exposure ledger: releases the 1,537 translated articles
	// 100/day instead of all at once. Lives in content/en/, next to the English
	// content dir. Missing file = no gate (every article advertised), so this is
	// also the rollback switch: delete the file and the full corpus is back.
	exposurePath := filepath.Join(filepath.Dir(cfg.BlogContentDirEN), "sitemap-exposure.txt")
	exposure := blog.NewExposureLedger(exposurePath)
	if !exposure.Active() {
		fmt.Printf("[seo] exposure ledger not found at %s — advertising all English articles\n", exposurePath)
	}
	// The renderer needs the store to resolve a blog article's counterpart when
	// building the language switcher target (see ssr.Renderer.switchTargets).
	if ssrRenderer != nil {
		ssrRenderer.SetBlogStore(blogStore)
	}

	// Serve uploaded files when using local storage.
	if cfg.StorageBackend != "tencent_cos" {
		if st, err := storage.Get(cfg); err == nil {
			if ls, ok := st.(*storage.LocalStorage); ok {
				r.GET("/uploads/*filepath", func(c *gin.Context) {
					ls.ServeFile(c.Writer, c.Request, c.Param("filepath"))
				})
			}
		}
	}

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": "0.2.0"})
	})

	// Domain API routers
	handler.NewAuthHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewBlueprintsHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewImagesHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewTagsHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewNotificationsHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewUsersHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewReportsHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewStatsHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewAdminHandler(cfg, gdb).RegisterRoutes(api)
	handler.NewBlogAPIHandler(cfg, blogStore).RegisterRoutes(api)

	// SSR page routes (site root) + sitemap.
	ssr.NewHandler(cfg, gdb, ssrRenderer, blogStore).RegisterRoutes(r)
	handler.NewSEOHandler(cfg, gdb, blogStore).WithExposureLedger(exposure).RegisterRoutes(r)

	// Fallback: /api/* → JSON 404; /en/* → English 404 page; anything else →
	// Chinese 404 page. The /en/ tree must never leak its unknown-URL misses
	// into the Chinese 404 (P2-1), so the language is chosen by the URL prefix.
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Not found"})
			return
		}
		h := ssr.NewHandler(cfg, gdb, ssrRenderer, blogStore)
		if strings.HasPrefix(c.Request.URL.Path, "/en") {
			h.NotFoundEN(c)
			return
		}
		h.NotFound(c)
	})

	return r
}
