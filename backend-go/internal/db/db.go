package db

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to MySQL. SQL logging is silenced in production to avoid leaking
// bound parameter values into logs.
func Open(dsn string, env string) (*gorm.DB, error) {
	level := logger.Silent
	if env != "production" {
		level = logger.Warn
	}
	return gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(level),
	})
}

// TestLogger returns a silent logger for use in tests.
func TestLogger() logger.Interface { return logger.Default.LogMode(logger.Silent) }

// OrderImages is a GORM preload scope that orders blueprint images by sort_order
// only. sort_order is the single source of truth for page sequence: it defaults
// to upload order (incrementing) and is rewritten when the author reorders in the
// edit page. The DB id (a random UUID) must NEVER participate in ordering.
func OrderImages(db *gorm.DB) *gorm.DB {
	return db.Order("sort_order ASC")
}
