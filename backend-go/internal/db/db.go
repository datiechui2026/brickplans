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
