package db

import (
	"log"
	"os"

	"github.com/piwi3910/NovaSky/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate all models
	err = DB.AutoMigrate(
		&models.Frame{},
		&models.AnalysisResult{},
		&models.SensorReading{},
		&models.SafetyState{},
		&models.Alert{},
		&models.Config{},
		&models.Detection{},
		&models.OverlayLayout{},
		&models.DarkFrame{},
		&models.NightlySummary{},
		&models.ServiceHealth{},
	)
	if err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}
}

func GetDB() *gorm.DB {
	return DB
}
