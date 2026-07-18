package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
)

// CtxUser is the context key holding the authenticated *db.User (may be nil for OptionalUser).
const CtxUser = "current_user"

func extractToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if h == "" || !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

// AuthRequired validates an access token and loads the current user. Aborts with
// 401 if the token is missing, malformed, expired, of the wrong type, or if the
// user's TokenVersion no longer matches (stateless revocation).
func AuthRequired(cfg *config.Config, gdb *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := extractToken(c)
		if tok == "" {
			abortAuth(c, http.StatusUnauthorized, "Not authenticated")
			return
		}
		claims, err := Parse(cfg, tok)
		if err != nil || claims.Type != "access" || claims.Subject == "" {
			abortAuth(c, http.StatusUnauthorized, "Invalid or expired token")
			return
		}
		var user db.User
		if err := gdb.First(&user, "id = ?", claims.Subject).Error; err != nil {
			abortAuth(c, http.StatusUnauthorized, "User not found")
			return
		}
		if user.TokenVersion != claims.Ver {
			abortAuth(c, http.StatusUnauthorized, "Token revoked")
			return
		}
		if user.Banned {
			abortAuth(c, http.StatusForbidden, "账号已被禁用")
			return
		}
		c.Set(CtxUser, &user)
		c.Next()
	}
}

// AdminRequired composes AuthRequired with an is_admin check (403 otherwise).
func AdminRequired(cfg *config.Config, gdb *gorm.DB) gin.HandlerFunc {
	auth := AuthRequired(cfg, gdb)
	return func(c *gin.Context) {
		auth(c)
		if c.IsAborted() {
			return
		}
		if u := CurrentUser(c); u == nil || !u.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"detail": "Admin access required"})
			return
		}
		c.Next()
	}
}

// OptionalUser decodes the token if present but never aborts; sets a nil *User otherwise.
// Used by public endpoints that behave differently when authenticated (e.g. is_favorited).
func OptionalUser(cfg *config.Config, gdb *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CtxUser, (*db.User)(nil))
		tok := extractToken(c)
		if tok == "" {
			c.Next()
			return
		}
		claims, err := Parse(cfg, tok)
		if err != nil || claims.Type != "access" || claims.Subject == "" {
			c.Next()
			return
		}
		var user db.User
		if err := gdb.First(&user, "id = ?", claims.Subject).Error; err != nil {
			c.Next()
			return
		}
		if user.TokenVersion == claims.Ver {
			c.Set(CtxUser, &user)
		}
		c.Next()
	}
}

// CurrentUser returns the *db.User set by an auth middleware, or nil.
func CurrentUser(c *gin.Context) *db.User {
	v, ok := c.Get(CtxUser)
	if !ok {
		return nil
	}
	u, _ := v.(*db.User)
	return u
}

func abortAuth(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"detail": msg})
}
