package handler

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"brickplans/internal/auth"
	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/dto"
	"brickplans/internal/mail"
	"brickplans/internal/middleware"
	"brickplans/internal/storage"
	"brickplans/internal/upload"
)

type AuthHandler struct {
	cfg *config.Config
	gdb *gorm.DB
}

func NewAuthHandler(cfg *config.Config, gdb *gorm.DB) *AuthHandler {
	return &AuthHandler{cfg: cfg, gdb: gdb}
}

func (h *AuthHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")
	g.POST("/register", middleware.RateLimit(5, 5), h.register)
	g.POST("/login", middleware.RateLimit(5, 5), h.login)
	g.POST("/refresh", middleware.RateLimit(10, 10), h.refresh)
	g.POST("/logout", h.logout)
	g.GET("/me", auth.AuthRequired(h.cfg, h.gdb), h.me)
	g.PUT("/me", auth.AuthRequired(h.cfg, h.gdb), h.updateMe)
	g.PUT("/password", auth.AuthRequired(h.cfg, h.gdb), middleware.RateLimit(5, 5), h.changePassword)
	g.POST("/avatar", auth.AuthRequired(h.cfg, h.gdb), middleware.RateLimit(10, 10), h.uploadAvatar)
	g.GET("/avatars", h.presetAvatars)
	g.GET("/verify-email", h.verifyEmail)
	g.POST("/verify-email/resend", auth.AuthRequired(h.cfg, h.gdb), middleware.RateLimit(3, 3), h.resendVerify)
}

type registerReq struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := auth.ValidateUsername(req.Username); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	// Uniqueness check (email + username together to map the right conflict).
	var existing db.User
	if err := h.gdb.Where("email = ? OR username = ?", req.Email, req.Username).First(&existing).Error; err == nil {
		if existing.Email == req.Email {
			c.JSON(http.StatusConflict, gin.H{"detail": "Email already registered"})
		} else {
			c.JSON(http.StatusConflict, gin.H{"detail": "Username already taken"})
		}
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	preset := fmt.Sprintf("/avatars/presets/%02d.png", rand.Intn(20)+1)
	user := db.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		AvatarURL:    &preset,
	}
	if err := h.gdb.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	go h.sendVerifyEmail(&user)
	h.respondTokens(c, http.StatusCreated, &user)
}

type loginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	var user db.User
	if err := h.gdb.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid email or password"})
		return
	}
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid email or password"})
		return
	}
	h.respondTokens(c, http.StatusOK, &user)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// refresh reads the refresh token from the bp_refresh httpOnly cookie (not the
// body), rotates it, and returns a new access token in the body. The cookie's
// Path=/api/auth means it's sent only to refresh/logout, limiting exposure.
func (h *AuthHandler) refresh(c *gin.Context) {
	token, err := c.Cookie("bp_refresh")
	if err != nil || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid or expired token"})
		return
	}
	claims, err := auth.Parse(h.cfg, token)
	if err != nil || claims.Type != "refresh" {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid or expired token"})
		return
	}
	var user db.User
	if err := h.gdb.First(&user, "id = ?", claims.Subject).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Invalid or expired token"})
		return
	}
	if user.TokenVersion != claims.Ver {
		c.JSON(http.StatusUnauthorized, gin.H{"detail": "Token revoked"})
		return
	}
	at, err := auth.CreateAccessToken(h.cfg, user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	rt, err := auth.CreateRefreshToken(h.cfg, user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	h.setRefreshCookie(c, rt)
	// Echo the user object so the frontend can restore session on page reload.
	c.JSON(http.StatusOK, dto.TokenResponse{AccessToken: at, TokenType: "bearer", User: dto.FromMe(&user)})
}

func (h *AuthHandler) me(c *gin.Context) {
	c.JSON(http.StatusOK, dto.FromMe(auth.CurrentUser(c)))
}

type updateMeReq struct {
	Username  *string `json:"username"`
	Bio       *string `json:"bio"`
	AvatarURL *string `json:"avatar_url"`
}

func (h *AuthHandler) updateMe(c *gin.Context) {
	user := auth.CurrentUser(c)
	var req updateMeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if req.Username != nil {
		*req.Username = strings.TrimSpace(*req.Username)
		if err := auth.ValidateUsername(*req.Username); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
			return
		}
		if *req.Username != user.Username {
			var existing db.User
			if err := h.gdb.Where("username = ? AND id <> ?", *req.Username, user.ID).First(&existing).Error; err == nil {
				c.JSON(http.StatusConflict, gin.H{"detail": "Username already taken"})
				return
			}
			user.Username = *req.Username
		}
	}
	if req.Bio != nil {
		user.Bio = req.Bio
	}
	if req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}
	if err := h.gdb.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.JSON(http.StatusOK, dto.FromMe(user))
}

type passwordReq struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

func (h *AuthHandler) changePassword(c *gin.Context) {
	user := auth.CurrentUser(c)
	var req passwordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	if !auth.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "Current password is incorrect"})
		return
	}
	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	user.PasswordHash = hash
	user.TokenVersion++ // revoke all previously issued tokens (stateless)
	if err := h.gdb.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password updated"})
}

func (h *AuthHandler) uploadAvatar(c *gin.Context) {
	user := auth.CurrentUser(c)
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "No file provided"})
		return
	}
	if file.Size > upload.MaxAvatarSize {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "File too large. Maximum size is 2MB."})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无法读取文件"})
		return
	}
	defer src.Close()
	data, err := io.ReadAll(io.LimitReader(src, upload.MaxAvatarSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "无法读取文件"})
		return
	}
	if len(data) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": upload.ErrEmpty.Error()})
		return
	}
	if len(data) > upload.MaxAvatarSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"detail": "File too large. Maximum size is 2MB."})
		return
	}
	// Trust magic bytes, NOT the client-supplied Content-Type. Then re-encode to
	// JPEG and store under a forced .jpg key — this blocks .html/.svg stored XSS.
	if _, ok := upload.DetectImageType(data); !ok {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": upload.ErrUnsupported.Error()})
		return
	}
	jpegBytes, err := upload.ReencodeImage(data, 512, 85)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": upload.ErrBadImage.Error()})
		return
	}
	st, err := storage.Get(h.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	// "avatar.jpg" forces the .jpg extension server-side; the UUID key makes the path random.
	obj, err := st.Upload(jpegBytes, "avatar.jpg", "image/jpeg", "avatars")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	// Best-effort delete of the previous avatar file.
	if user.AvatarURL != nil && strings.HasPrefix(*user.AvatarURL, "/uploads/") {
		_ = st.Delete(*user.AvatarURL)
	}
	user.AvatarURL = &obj.URL
	if err := h.gdb.Save(user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar_url": obj.URL, "user": dto.FromMe(user)})
}

func (h *AuthHandler) presetAvatars(c *gin.Context) {
	avatars := make([]gin.H, 0, 20)
	for i := 1; i <= 20; i++ {
		avatars = append(avatars, gin.H{
			"id":  fmt.Sprintf("preset-%02d", i),
			"url": fmt.Sprintf("/avatars/presets/%02d.png", i),
		})
	}
	c.JSON(http.StatusOK, gin.H{"avatars": avatars})
}

// respondTokens issues a fresh access+refresh pair. The access token goes in the
// response body (kept in memory by the frontend); the refresh token goes in an
// httpOnly cookie (Path=/api/auth) so client JS can never read it.
func (h *AuthHandler) respondTokens(c *gin.Context, code int, user *db.User) {
	at, err := auth.CreateAccessToken(h.cfg, user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	rt, err := auth.CreateRefreshToken(h.cfg, user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "internal error"})
		return
	}
	h.setRefreshCookie(c, rt)
	c.JSON(code, dto.TokenResponse{AccessToken: at, TokenType: "bearer", User: dto.FromMe(user)})
}

// setRefreshCookie sets the httpOnly refresh cookie. SameSite=Lax blocks
// cross-site POSTs (CSRF mitigation); Secure is on in production (HTTPS).
func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("bp_refresh", token, h.cfg.JWTRefreshDays*24*3600, "/api/auth", "", h.cfg.IsProd(), true)
}

// logout clears the refresh cookie.
func (h *AuthHandler) logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("bp_refresh", "", -1, "/api/auth", "", h.cfg.IsProd(), true)
	c.JSON(http.StatusOK, gin.H{"detail": "logged out"})
}

// sendVerifyEmail issues a 24h email-verification token and mails the link.
// Best-effort: failures are logged but never surface to the user flow.
func (h *AuthHandler) sendVerifyEmail(user *db.User) {
	token, err := auth.CreateEmailVerifyToken(h.cfg, user.ID)
	if err != nil {
		log.Printf("[mail] verify token for %s: %v", user.Email, err)
		return
	}
	url := h.cfg.AppBaseURL + "/api/auth/verify-email?token=" + token
	body := fmt.Sprintf(
		`<html><body style="font-family:sans-serif"><h2>验证你的 BrickPlans 邮箱</h2>`+
			`<p>点击下方链接完成验证（24 小时内有效）：</p>`+
			`<p><a href="%s" style="padding:10px 20px;background:#2563eb;color:#fff;text-decoration:none;border-radius:4px">验证邮箱</a></p>`+
			`<p style="color:#666;font-size:12px">若非本人操作请忽略此邮件。</p></body></html>`, url)
	if err := mail.SendMail(h.cfg.SMTPHost, h.cfg.SMTPPort, h.cfg.SMTPUser, h.cfg.SMTPPass,
		h.cfg.SMTPFrom, user.Email, "验证你的 BrickPlans 邮箱", body); err != nil {
		log.Printf("[mail] send verify to %s: %v", user.Email, err)
	}
}

// verifyEmail activates an account from an emailed link. Returns a small HTML
// page so the user clicking the link in their mailbox gets immediate feedback.
func (h *AuthHandler) verifyEmail(c *gin.Context) {
	token := c.Query("token")
	claims, err := auth.Parse(h.cfg, token)
	if err != nil || claims.Type != "email_verify" || claims.Subject == "" {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8",
			[]byte(`<html><body style="font-family:sans-serif;text-align:center;padding:40px"><h2>验证链接无效或已过期</h2><a href="/">返回首页</a></body></html>`))
		return
	}
	var user db.User
	if err := h.gdb.First(&user, "id = ?", claims.Subject).Error; err != nil {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8",
			[]byte(`<html><body style="font-family:sans-serif;text-align:center;padding:40px"><h2>用户不存在</h2></body></html>`))
		return
	}
	if !user.EmailVerified {
		user.EmailVerified = true
		h.gdb.Save(&user)
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8",
		[]byte(`<html><body style="font-family:sans-serif;text-align:center;padding:40px"><h2>✅ 邮箱验证成功</h2><p>你现在可以关闭此页面。</p><p><a href="/">返回 BrickPlans</a></p></body></html>`))
}

func (h *AuthHandler) resendVerify(c *gin.Context) {
	user := auth.CurrentUser(c)
	if user.EmailVerified {
		c.JSON(http.StatusBadRequest, gin.H{"detail": "邮箱已验证"})
		return
	}
	go h.sendVerifyEmail(user)
	c.JSON(http.StatusOK, gin.H{"message": "验证邮件已发送"})
}
