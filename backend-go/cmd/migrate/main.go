// Command migrate imports data from the legacy Python SQLite database
// (backend/brickplans.db) into the new MySQL database. Preserves UUIDs,
// timestamps, and bcrypt password hashes (passlib $2b$ is binary-compatible
// with Go's bcrypt). Existing users are marked email_verified=true so the
// 24h unverified-account cleanup doesn't delete them.
//
// Usage (from backend-go/):
//
//	go run ./cmd/migrate                         # import (idempotent via ON DUPLICATE KEY)
//	go run ./cmd/migrate --sqlite /path/to.db    # custom SQLite path
//	go run ./cmd/migrate --reset                 # truncate MySQL tables first
package main

import (
	"flag"
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"brickplans/internal/config"
	"brickplans/internal/db"
)

func main() {
	sqlitePath := flag.String("sqlite", "../backend/brickplans.db", "path to the legacy Python SQLite db")
	reset := flag.Bool("reset", false, "truncate MySQL tables before import")
	flag.Parse()

	cfg := config.Load()

	// Source: legacy SQLite (read-only).
	src, err := gorm.Open(sqlite.Open(*sqlitePath), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf("open sqlite %s: %v", *sqlitePath, err)
	}

	// Destination: MySQL.
	dst, err := db.Open(cfg.MySQLDSN, cfg.AppEnv)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if err := dst.AutoMigrate(db.AllModels()...); err != nil {
		log.Fatalf("auto-migrate: %v", err)
	}

	if *reset {
		log.Println("truncating MySQL tables...")
		dst.Exec("SET FOREIGN_KEY_CHECKS = 0")
		for _, t := range []string{"reports", "notifications", "comments", "likes", "favorites",
			"blueprint_tags", "blueprint_images", "blueprints", "tags", "users"} {
			dst.Exec("TRUNCATE TABLE " + t)
		}
		dst.Exec("SET FOREIGN_KEY_CHECKS = 1")
	}

	// OnConflict DoNothing makes re-runs idempotent (INSERT IGNORE semantics).
	conflict := clause.OnConflict{DoNothing: true}

	// ── users ── (Python User has no updated_at/token_version/email_verified)
	var users []db.User
	src.Select("id", "username", "email", "password_hash", "avatar_url", "bio", "is_admin", "created_at").
		Find(&users)
	for i := range users {
		users[i].TokenVersion = 0
		users[i].EmailVerified = true // don't let the 24h cleanup nuke legacy users
	}
	if err := dst.Clauses(conflict).CreateInBatches(&users, 200); err != nil {
		log.Fatalf("users: %v", err)
	}
	log.Printf("users: %d", len(users))

	// ── blueprints ──
	var bps []db.Blueprint
	src.Select("id", "author_id", "title", "slug", "description", "difficulty", "piece_count",
		"category", "dimensions", "part_list", "view_count", "like_count", "is_published",
		"created_at", "updated_at").Find(&bps)
	if err := dst.Clauses(conflict).CreateInBatches(&bps, 200); err != nil {
		log.Fatalf("blueprints: %v", err)
	}
	log.Printf("blueprints: %d", len(bps))

	// ── blueprint_images ──
	var imgs []db.BlueprintImage
	src.Select("id", "blueprint_id", "url", "object_key", "sort_order", "is_cover", "file_type", "created_at").
		Find(&imgs)
	if err := dst.Clauses(conflict).CreateInBatches(&imgs, 200); err != nil {
		log.Fatalf("images: %v", err)
	}
	log.Printf("blueprint_images: %d", len(imgs))

	// ── tags ──
	var tags []db.Tag
	src.Select("id", "name", "created_at").Find(&tags)
	if err := dst.Clauses(conflict).CreateInBatches(&tags, 200); err != nil {
		log.Fatalf("tags: %v", err)
	}
	log.Printf("tags: %d", len(tags))

	// ── blueprint_tags (composite PK) ──
	var bts []db.BlueprintTag
	src.Select("blueprint_id", "tag_id", "created_at").Find(&bts)
	if err := dst.Clauses(conflict).CreateInBatches(&bts, 200); err != nil {
		log.Fatalf("blueprint_tags: %v", err)
	}
	log.Printf("blueprint_tags: %d", len(bts))

	// ── favorites (composite PK) ──
	var favs []db.Favorite
	src.Select("user_id", "blueprint_id", "created_at").Find(&favs)
	if err := dst.Clauses(conflict).CreateInBatches(&favs, 200); err != nil {
		log.Fatalf("favorites: %v", err)
	}
	log.Printf("favorites: %d", len(favs))

	// ── likes ──
	var likes []db.Like
	src.Select("id", "user_id", "blueprint_id", "created_at").Find(&likes)
	if err := dst.Clauses(conflict).CreateInBatches(&likes, 200); err != nil {
		log.Fatalf("likes: %v", err)
	}
	log.Printf("likes: %d", len(likes))

	// ── comments ──
	var comments []db.Comment
	src.Select("id", "blueprint_id", "user_id", "parent_id", "content", "created_at").Find(&comments)
	if err := dst.Clauses(conflict).CreateInBatches(&comments, 200); err != nil {
		log.Fatalf("comments: %v", err)
	}
	log.Printf("comments: %d", len(comments))

	// ── notifications ──
	var notifs []db.Notification
	src.Select("id", "user_id", "actor_id", "type", "blueprint_id", "comment_id", "payload",
		"is_read", "created_at", "read_at").Find(&notifs)
	if err := dst.Clauses(conflict).CreateInBatches(&notifs, 200); err != nil {
		log.Fatalf("notifications: %v", err)
	}
	log.Printf("notifications: %d", len(notifs))

	// ── reports ──
	var reports []db.Report
	src.Select("id", "reporter_id", "blueprint_id", "reason", "detail", "status", "created_at").
		Find(&reports)
	if err := dst.Clauses(conflict).CreateInBatches(&reports, 200); err != nil {
		log.Fatalf("reports: %v", err)
	}
	log.Printf("reports: %d", len(reports))

	log.Println("════ migration complete ════")
}
