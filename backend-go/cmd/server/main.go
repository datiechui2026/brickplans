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

	// Periodically remove accounts that never verified their email within 24h.
	go cleanupUnverified(gdb)

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
