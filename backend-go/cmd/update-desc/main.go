package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"brickplans/internal/config"
	"brickplans/internal/db"
)

func main() {
	cfg := config.Load()
	gdb, err := db.Open(cfg.MySQLDSN, cfg.AppEnv)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}

	// Read all description files from /tmp/brickplan/descriptions/
	descDir := "/tmp/brickplan/descriptions"
	entries, err := os.ReadDir(descDir)
	if err != nil {
		log.Fatalf("read dir: %v", err)
	}

	updated := 0
	notFound := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		title := strings.TrimSuffix(entry.Name(), ".md")
		raw, err := os.ReadFile(filepath.Join(descDir, entry.Name()))
		if err != nil {
			log.Printf("[WARN] read %s: %v", entry.Name(), err)
			continue
		}
		desc := string(raw)

		// Find blueprint by title
		result := gdb.Model(&db.Blueprint{}).Where("title = ?", title).Update("description", desc)
		if result.Error != nil {
			log.Printf("[WARN] update %q: %v", title, result.Error)
			continue
		}
		if result.RowsAffected == 0 {
			log.Printf("[WARN] %q: no blueprint found with this title", title)
			notFound++
		} else {
			log.Printf("[OK] %q: description updated (%d bytes)", title, len(desc))
			updated++
		}
	}

	log.Printf("════ Complete ════ updated=%d notFound=%d", updated, notFound)
}