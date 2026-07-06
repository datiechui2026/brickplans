package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// defaultSecret mirrors the old Python default — it MUST be overridden in production.
const defaultSecret = "change-me-in-production-use-a-long-random-string"

type Config struct {
	AppEnv                  string
	HTTPAddr                string
	SecretKey               string
	JWTAccessMin            int
	JWTRefreshDays          int
	MySQLDSN                string
	CORSOrigins             []string
	StorageBackend          string
	UploadDir               string
	TencentCOSSecretID      string
	TencentCOSSecretKey     string
	TencentCOSBucket        string
	TencentCOSRegion        string
	TencentCOSPublicBaseURL string

	AppBaseURL string
	SMTPHost   string
	SMTPPort   string
	SMTPUser   string
	SMTPPass   string
	SMTPFrom   string

	// SEO/SSR
	FrontendDist string // path to frontend/dist (for vite manifest + static assets)
	PublicURL    string // canonical site URL, e.g. https://brickplans.com
}

func Load() *Config {
	// Load .env if present; ignore error when it doesn't exist (production reads real env).
	_ = godotenv.Load()

	c := &Config{
		AppEnv:                  getenv("APP_ENV", "development"),
		HTTPAddr:                getenv("HTTP_ADDR", "127.0.0.1:8100"),
		SecretKey:               getenv("SECRET_KEY", defaultSecret),
		JWTAccessMin:            getenvInt("JWT_ACCESS_MIN", 30),
		JWTRefreshDays:          getenvInt("JWT_REFRESH_DAYS", 7),
		MySQLDSN:                getenv("MYSQL_DSN", ""),
		CORSOrigins:             splitCSV(getenv("CORS_ORIGINS", "http://localhost:5173")),
		StorageBackend:          getenv("STORAGE_BACKEND", "local"),
		UploadDir:               getenv("UPLOAD_DIR", "./uploads"),
		TencentCOSSecretID:      getenv("TENCENT_COS_SECRET_ID", ""),
		TencentCOSSecretKey:     getenv("TENCENT_COS_SECRET_KEY", ""),
		TencentCOSBucket:        getenv("TENCENT_COS_BUCKET", ""),
		TencentCOSRegion:        getenv("TENCENT_COS_REGION", ""),
		TencentCOSPublicBaseURL: getenv("TENCENT_COS_PUBLIC_BASE_URL", ""),
		AppBaseURL:              getenv("APP_BASE_URL", "http://localhost:5173"),
		SMTPHost:                getenv("SMTP_HOST", ""),
		SMTPPort:                getenv("SMTP_PORT", "587"),
		SMTPUser:                getenv("SMTP_USER", ""),
		SMTPPass:                getenv("SMTP_PASS", ""),
		SMTPFrom:                getenv("SMTP_FROM", ""),
		FrontendDist:            getenv("FRONTEND_DIST", "../frontend/dist"),
		PublicURL:               getenv("PUBLIC_URL", "https://brickplans.com"),
	}
	c.validate()
	return c
}

// validate enforces security-critical config at startup so the server never
// boots with a known-default JWT secret or missing DB DSN.
func (c *Config) validate() {
	if c.SecretKey == defaultSecret {
		log.Fatal("config: SECRET_KEY is the built-in default — set a random >=32-char value in .env")
	}
	if len(c.SecretKey) < 32 {
		log.Fatal("config: SECRET_KEY must be at least 32 characters")
	}
	if c.MySQLDSN == "" {
		log.Fatal("config: MYSQL_DSN must be set")
	}
}

func (c *Config) IsProd() bool { return c.AppEnv == "production" }

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func splitCSV(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
