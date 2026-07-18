package main

import (
	"log"
	"time"

	"gorm.io/gorm"

	"brickplans/internal/config"
	"brickplans/internal/db"
	"brickplans/internal/router"
	"brickplans/internal/ssr"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Open(cfg.MySQLDSN, cfg.AppEnv)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := gdb.AutoMigrate(db.AllModels()...); err != nil {
		log.Fatalf("auto-migrate: %v", err)
	}

	// Disabled: the 24h unverified-account cleanup cascades to delete users'
	// blueprints/images DB rows while leaving the COS files orphaned. With SMTP
	// not configured, no one can verify email, so real users (and their
	// blueprints) were being wiped after 24h. Re-enable once email verification
	// is actually deliverable. See cleanupUnverified below.
	// go cleanupUnverified(gdb)

	renderer := ssr.NewRenderer(cfg.FrontendDist, cfg.PublicURL)
	r := router.New(cfg, gdb, renderer)
	log.Printf("BrickPlans backend-go listening on %s (env=%s)", cfg.HTTPAddr, cfg.AppEnv)
	if err := r.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// cleanupUnverified deletes unverified accounts older than 24h every hour.
// Cascades (OnDelete:CASCADE) remove their blueprints/images/etc. too.
func cleanupUnverified(gdb *gorm.DB) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-24 * time.Hour)
		if err := gdb.Where("email_verified = ? AND created_at < ?", false, cutoff).
			Delete(&db.User{}).Error; err != nil {
			log.Printf("[cleanup] unverified users: %v", err)
		}
	}
}
