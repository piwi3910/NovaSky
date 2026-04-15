package nightlysummary

import (
	"log"
	"time"

	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
)

// Generate creates a nightly summary for the given date
func Generate(date string) error {
	// Count frames from this date
	var frameCount int64
	db.GetDB().Model(&models.Frame{}).
		Where("DATE(created_at) = ?", date).
		Count(&frameCount)

	// Average cloud cover
	var avgCloud float64
	db.GetDB().Model(&models.AnalysisResult{}).
		Where("DATE(analyzed_at) = ?", date).
		Select("COALESCE(AVG(cloud_cover), 0)").Scan(&avgCloud)

	// Average SQM
	var avgSQM *float64
	db.GetDB().Model(&models.AnalysisResult{}).
		Where("DATE(analyzed_at) = ? AND sqm IS NOT NULL", date).
		Select("AVG(sqm)").Scan(&avgSQM)

	// Count clear hours (sky_quality != UNUSABLE)
	var clearCount int64
	db.GetDB().Model(&models.AnalysisResult{}).
		Where("DATE(analyzed_at) = ? AND sky_quality != 'UNUSABLE'", date).
		Count(&clearCount)
	// Assume ~10 seconds per frame
	clearHours := float64(clearCount) * 10.0 / 3600.0

	summary := models.NightlySummary{
		Date:          date,
		TotalFrames:   int(frameCount),
		ClearHours:    clearHours,
		CloudCoverAvg: avgCloud,
		SQMAvg:        avgSQM,
	}

	// Upsert
	result := db.GetDB().Where("date = ?", date).FirstOrCreate(&summary)
	if result.RowsAffected == 0 {
		db.GetDB().Model(&summary).Where("date = ?", date).Updates(summary)
	}

	log.Printf("[nightlysummary] Generated for %s: %d frames, %.1f clear hours, %.0f%% cloud avg",
		date, frameCount, clearHours, avgCloud*100)
	return nil
}

// GenerateYesterday generates summary for the previous night
func GenerateYesterday() error {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	return Generate(yesterday)
}
