package router

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/handler"
	"brickplans/internal/middleware"
	"brickplans/internal/ssr"
	"brickplans/internal/storage"
)

// New assembles the gin engine with global middleware and all route groups.
// ssrRenderer renders server-side HTML for page routes (SEO/GEO).
func New(cfg *config.Config, gdb *gorm.DB, ssrRenderer *ssr.Renderer) *gin.Engine {
	if cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.SecurityHeaders())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))
	r.Use(gin.Logger())

	// Multipart upload memory threshold; larger parts spill to temp files.
	r.MaxMultipartMemory = 32 << 20

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

	// SSR page routes (site root) + sitemap.
	ssr.NewHandler(cfg, gdb, ssrRenderer).RegisterRoutes(r)
	handler.NewSEOHandler(cfg, gdb).RegisterRoutes(r)

	// Fallback: /api/* → JSON 404; anything else → SSR 404 page.
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"detail": "Not found"})
			return
		}
		ssr.NewHandler(cfg, gdb, ssrRenderer).NotFound(c)
	})

	return r
}
